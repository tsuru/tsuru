// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/set"
)

type DeployKind string

const (
	DeployArchiveURL   DeployKind = "archive-url"
	DeployGit          DeployKind = "git"
	DeployImage        DeployKind = "image"
	DeployBuildedImage DeployKind = "imagebuild"
	DeployRollback     DeployKind = "rollback"
	DeployUpload       DeployKind = "upload"
	DeployUploadBuild  DeployKind = "uploadbuild"
	DeployRebuild      DeployKind = "rebuild"
)

var reImageVersion = regexp.MustCompile("v[0-9]+$")

type DeployData struct {
	ID          bson.ObjectId `bson:"_id,omitempty"`
	App         string
	Timestamp   time.Time
	Duration    time.Duration
	Commit      string
	Error       string
	Image       string
	Log         string
	User        string
	Origin      string
	CanRollback bool
	RemoveDate  time.Time `bson:",omitempty"`
	Diff        string
}

func findValidImages(apps ...App) (set.Set, error) {
	validImages := set.Set{}
	for _, a := range apps {
		imgs, err := image.ListAppImages(a.Name)
		if err != nil && err != mgo.ErrNotFound {
			return nil, err
		}
		validImages.Add(imgs...)
	}
	return validImages, nil
}

// ListDeploys returns the list of deploy that match a given filter.
func ListDeploys(filter *Filter, skip, limit int) ([]DeployData, error) {
	appsList, err := List(filter)
	if err != nil {
		return nil, err
	}
	apps := make([]string, len(appsList))
	for i, a := range appsList {
		apps[i] = a.GetName()
	}
	evts, err := event.List(&event.Filter{
		Target:    event.Target{Type: event.TargetTypeApp},
		Raw:       bson.M{"target.value": bson.M{"$in": apps}},
		KindNames: []string{permission.PermAppDeploy.FullName()},
		KindType:  event.KindTypePermission,
		Limit:     limit,
		Skip:      skip,
	})
	if err != nil {
		return nil, err
	}
	validImages, err := findValidImages(appsList...)
	if err != nil {
		return nil, err
	}
	list := make([]DeployData, len(evts))
	for i := range evts {
		list[i] = *eventToDeployData(&evts[i], validImages, false)
	}
	return list, nil
}

func GetDeploy(id string) (*DeployData, error) {
	if !bson.IsObjectIdHex(id) {
		return nil, errors.Errorf("id parameter is not ObjectId: %s", id)
	}
	objID := bson.ObjectIdHex(id)
	evt, err := event.GetByID(objID)
	if err != nil {
		return nil, err
	}
	return eventToDeployData(evt, nil, true), nil
}

func eventToDeployData(evt *event.Event, validImages set.Set, full bool) *DeployData {
	data := &DeployData{
		ID:        evt.UniqueID,
		App:       evt.Target.Value,
		Timestamp: evt.StartTime,
		Duration:  evt.EndTime.Sub(evt.StartTime),
		Error:     evt.Error,
		User:      evt.Owner.Name,
	}
	var startOpts DeployOptions
	err := evt.StartData(&startOpts)
	if err == nil {
		data.Commit = startOpts.Commit
		data.Origin = startOpts.GetOrigin()
	}
	if full {
		data.Log = evt.Log
		var otherData map[string]string
		err = evt.OtherData(&otherData)
		if err == nil {
			data.Diff = otherData["diff"]
		}
	}
	var endData map[string]string
	err = evt.EndData(&endData)
	if err == nil {
		data.Image = endData["image"]
		if validImages != nil {
			data.CanRollback = validImages.Includes(data.Image)
			if reImageVersion.MatchString(data.Image) {
				parts := reImageVersion.FindAllStringSubmatch(data.Image, -1)
				data.Image = parts[0][0]
			}
		}
	}
	return data
}

type DeployOptions struct {
	App          *App
	Commit       string
	BuildTag     string
	ArchiveURL   string
	FileSize     int64
	File         io.ReadCloser `bson:"-"`
	OutputStream io.Writer     `bson:"-"`
	User         string
	Image        string
	Origin       string
	Rollback     bool
	Build        bool
	Event        *event.Event `bson:"-"`
	Kind         DeployKind
	Message      string
}

func (o *DeployOptions) GetOrigin() string {
	if o.Origin != "" {
		return o.Origin
	}
	if o.Commit != "" {
		return "git"
	}
	return ""
}

func (o *DeployOptions) GetKind() (kind DeployKind) {
	defer func() {
		o.Kind = kind
	}()
	if o.Rollback {
		return DeployRollback
	}
	if o.Image != "" {
		return DeployImage
	}
	if o.File != nil {
		if o.Build {
			return DeployUploadBuild
		}
		return DeployUpload
	}
	if o.Commit != "" {
		return DeployGit
	}
	return DeployArchiveURL
}

func Build(opts DeployOptions) (string, error) {
	if opts.Event == nil {
		return "", errors.Errorf("missing event in build opts")
	}
	logWriter := LogWriter{App: opts.App}
	logWriter.Async()
	defer logWriter.Close()
	opts.Event.SetLogWriter(io.MultiWriter(&tsuruIo.NoErrorWriter{Writer: opts.OutputStream}, &logWriter))
	prov, err := opts.App.getProvisioner()
	if err != nil {
		return "", err
	}
	if opts.App.GetPlatform() == "" {
		return "", errors.Errorf("can't build app without platform")
	}
	builder, ok := prov.(provision.BuilderDeploy)
	if !ok {
		return "", errors.Errorf("provisioner don't implement builder interface")
	}
	imageID, err := builderDeploy(builder, &opts, opts.Event)
	if err != nil {
		return "", err
	}
	return imageID, nil
}

type errorWithLog struct {
	err  error
	logs []Applog
}

func (e *errorWithLog) Cause() error {
	return e.err
}

func (e *errorWithLog) formatLogLines() string {
	linesStr := make([]string, len(e.logs))
	for i, l := range e.logs {
		linesStr[i] = fmt.Sprintf("    %s[%s][%s]: %s", l.Date.Format(time.RFC3339), l.Source, l.Unit, l.Message)
	}
	return strings.Join(linesStr, "\n")
}

func (e *errorWithLog) Error() string {
	var logPart string
	if len(e.logs) > 0 {
		logPart = fmt.Sprintf("\n---- Last %d log messages: ----\n%s", len(e.logs), e.formatLogLines())
	}
	return fmt.Sprintf("---- ERROR during deploy: ----\n%v%s", e.err, logPart)
}

// Deploy runs a deployment of an application. It will first try to run an
// archive based deploy (if opts.ArchiveURL is not empty), and then fallback to
// the Git based deployment.
func Deploy(opts DeployOptions) (string, error) {
	if opts.Event == nil {
		return "", errors.Errorf("missing event in deploy opts")
	}
	if opts.Rollback && !regexp.MustCompile(":v[0-9]+$").MatchString(opts.Image) {
		imageName, err := image.GetAppImageBySuffix(opts.App.Name, opts.Image)
		if err != nil {
			return "", err
		}
		opts.Image = imageName
	}
	logWriter := LogWriter{App: opts.App}
	logWriter.Async()
	defer logWriter.Close()
	opts.Event.SetLogWriter(io.MultiWriter(&tsuruIo.NoErrorWriter{Writer: opts.OutputStream}, &logWriter))
	imageID, err := deployToProvisioner(&opts, opts.Event)
	rebuild.RoutesRebuildOrEnqueue(opts.App.Name)
	if err != nil {
		var logLines []Applog
		if provision.IsStartupError(err) {
			logLines, _ = opts.App.lastLogs(10, Applog{
				Source: "tsuru",
			}, true)
		}
		err = &errorWithLog{err: err, logs: logLines}
		return "", err
	}
	err = incrementDeploy(opts.App)
	if err != nil {
		log.Errorf("WARNING: couldn't increment deploy count, deploy opts: %#v", opts)
	}
	if opts.Kind == DeployImage || opts.Kind == DeployRollback {
		if !opts.App.UpdatePlatform {
			opts.App.SetUpdatePlatform(true)
		}
	} else if opts.App.UpdatePlatform {
		opts.App.SetUpdatePlatform(false)
	}
	return imageID, nil
}

func RollbackUpdate(appName, imageID, reason string, disableRollback bool) error {
	imgName, err := image.GetAppImageBySuffix(appName, imageID)
	if err != nil {
		return err
	}
	return image.UpdateAppImageRollback(imgName, reason, disableRollback)
}

func deployToProvisioner(opts *DeployOptions, evt *event.Event) (string, error) {
	prov, err := opts.App.getProvisioner()
	if err != nil {
		return "", err
	}
	if opts.Kind == "" {
		opts.GetKind()
	}
	if (opts.App.GetPlatform() == "") && ((opts.Kind != DeployImage) && (opts.Kind != DeployRollback)) {
		return "", errors.Errorf("can't deploy app without platform, if it's not an image or rollback")
	}

	if opts.Kind != DeployRollback {
		if deployer, ok := prov.(provision.BuilderDeploy); ok {
			imageID, err := builderDeploy(deployer, opts, evt)
			if err != nil {
				return "", err
			}
			return deployer.Deploy(opts.App, imageID, evt)
		}
	} else {
		if deployer, ok := prov.(provision.RollbackableDeployer); ok {
			return deployer.Rollback(opts.App, opts.Image, evt)
		}
	}
	return "", provision.ProvisionerNotSupported{Prov: prov, Action: fmt.Sprintf("%s deploy", opts.Kind)}
}

func builderDeploy(prov provision.BuilderDeploy, opts *DeployOptions, evt *event.Event) (string, error) {
	isRebuild := opts.Kind == DeployRebuild
	buildOpts := builder.BuildOpts{
		BuildFromFile: opts.Build,
		ArchiveURL:    opts.ArchiveURL,
		ArchiveFile:   opts.File,
		ArchiveSize:   opts.FileSize,
		Rebuild:       isRebuild,
		ImageID:       opts.Image,
		Tag:           opts.BuildTag,
	}
	builder, err := opts.App.getBuilder()
	if err != nil {
		return "", err
	}
	img, err := builder.Build(prov, opts.App, evt, &buildOpts)
	if buildOpts.IsTsuruBuilderImage {
		opts.Kind = DeployBuildedImage
	}
	return img, err
}

func ValidateOrigin(origin string) bool {
	originList := []string{"app-deploy", "git", "rollback", "drag-and-drop", "image", "rebuild"}
	for _, ol := range originList {
		if ol == origin {
			return true
		}
	}
	return false
}

func incrementDeploy(app *App) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$inc": bson.M{"deploys": 1}},
	)
	if err == nil {
		app.Deploys += 1
	}
	return err
}

func deployDataToEvent(data *DeployData) error {
	var evt event.Event
	evt.UniqueID = data.ID
	evt.Target = event.Target{Type: event.TargetTypeApp, Value: data.App}
	evt.Owner = event.Owner{Type: event.OwnerTypeUser, Name: data.User}
	evt.Kind = event.Kind{Type: event.KindTypePermission, Name: permission.PermAppDeploy.FullName()}
	evt.StartTime = data.Timestamp
	evt.EndTime = data.Timestamp.Add(data.Duration)
	evt.Error = data.Error
	evt.Log = data.Log
	evt.RemoveDate = data.RemoveDate
	a, err := GetByName(data.App)
	if err == nil {
		evt.Allowed = event.Allowed(permission.PermAppReadEvents, append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...)
	} else {
		evt.Allowed = event.Allowed(permission.PermAppReadEvents)
	}
	startOpts := DeployOptions{
		Commit: data.Commit,
		Origin: data.Origin,
	}
	var otherData map[string]string
	if data.Diff != "" {
		otherData = map[string]string{"diff": data.Diff}
	}
	endData := map[string]string{"image": data.Image}
	err = evt.RawInsert(startOpts, otherData, endData)
	if mgo.IsDup(err) {
		return nil
	}
	return err
}

func MigrateDeploysToEvents() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	oldDeploysColl := conn.Collection("deploys")
	iter := oldDeploysColl.Find(nil).Iter()
	var data DeployData
	for iter.Next(&data) {
		err = deployDataToEvent(&data)
		if err != nil {
			return err
		}
	}
	return iter.Close()
}
