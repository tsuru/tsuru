// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
	permTypes "github.com/tsuru/tsuru/types/permission"
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

var reImageVersion = regexp.MustCompile(":v([0-9]+)$")

type DeployData struct {
	ID          bson.ObjectId `bson:"_id,omitempty"`
	App         string
	Timestamp   time.Time
	Duration    time.Duration
	Commit      string
	Error       string
	Image       string
	Version     int
	Log         string
	User        string
	Origin      string
	CanRollback bool
	Diff        string
	Message     string
}

func findValidImages(ctx context.Context, appNames []string) (set.Set, error) {
	validImages := set.Set{}

	allVersions, err := servicemanager.AppVersion.AllAppVersions(ctx, appNames...)
	if err != nil {
		return nil, err
	}

	for _, av := range allVersions {
		for _, version := range av.Versions {
			if version.DeploySuccessful && version.DeployImage != "" {
				validImages.Add(version.DeployImage)
			}
		}
	}
	return validImages, nil
}

// ListDeploys returns the list of deploy that match a given filter.
func ListDeploys(ctx context.Context, filter *Filter, skip, limit int) ([]DeployData, error) {
	var rawFilter bson.M
	if !filter.IsEmpty() {
		appsList, err := List(ctx, filter)
		if err != nil {
			return nil, err
		}
		apps := make([]string, len(appsList))
		for i, a := range appsList {
			apps[i] = a.GetName()
		}
		rawFilter = bson.M{"target.value": bson.M{"$in": apps}}
	}
	evts, err := event.List(&event.Filter{
		Target:    event.Target{Type: event.TargetTypeApp},
		Raw:       rawFilter,
		KindNames: []string{permission.PermAppDeploy.FullName()},
		KindType:  event.KindTypePermission,
		Limit:     limit,
		Skip:      skip,
	})
	if err != nil {
		return nil, err
	}
	if len(evts) == 0 {
		return []DeployData{}, nil
	}
	appsInEvents := set.Set{}
	for _, evt := range evts {
		appsInEvents.Add(evt.Target.Value)
	}
	validImages, err := findValidImages(ctx, appsInEvents.ToList())
	if err != nil {
		return nil, err
	}
	list := make([]DeployData, len(evts))
	for i := range evts {
		list[i] = *eventToDeployData(evts[i], validImages, false)
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
	var err error
	var deployOptions DeployOptions
	if err = evt.StartData(&deployOptions); err == nil {
		data.Commit = deployOptions.Commit
		data.Origin = deployOptions.GetOrigin()
		data.Message = deployOptions.Message
	} else {
		log.Errorf("cannot decode the event's start custom data value: event %s - %v", evt.UniqueID, err)
	}
	if full {
		data.Log = evt.Log()
		var otherData map[string]string
		if err = evt.OtherData(&otherData); err == nil {
			data.Diff = otherData["diff"]
		} else {
			log.Errorf("cannot decode the event's other custom data value: event %s - %v", evt.UniqueID, err)
		}
	}
	var endData map[string]string
	if err = evt.EndData(&endData); err == nil {
		data.Image = endData["image"]
		if reImageVersion.MatchString(data.Image) {
			parts := reImageVersion.FindStringSubmatch(data.Image)
			data.Version, _ = strconv.Atoi(parts[1])
		}
		if validImages != nil {
			data.CanRollback = validImages.Includes(data.Image)
		}
	} else {
		log.Errorf("cannot decode the event's end custom data value: event %s - %v", evt.UniqueID, err)
	}
	return data
}

type DeployOptions struct {
	App              *App
	Commit           string
	BuildTag         string
	ArchiveURL       string
	FileSize         int64
	File             io.ReadCloser `bson:"-"`
	OutputStream     io.Writer     `bson:"-"`
	User             string
	Image            string
	Origin           string
	Event            *event.Event `bson:"-"`
	Kind             DeployKind
	Message          string
	Rollback         bool
	Build            bool
	NewVersion       bool
	OverrideVersions bool
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

func Build(ctx context.Context, opts DeployOptions) (string, error) {
	if opts.Event == nil {
		return "", errors.Errorf("missing event in build opts")
	}
	logWriter := LogWriter{AppName: opts.App.Name}
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
	version, err := builderDeploy(ctx, builder, &opts, opts.Event)
	if err != nil {
		return "", err
	}
	vi := version.VersionInfo()
	if vi.DeployImage != "" {
		return vi.DeployImage, nil
	}
	return vi.BuildImage, nil
}

type errorWithLog struct {
	err    error
	action string
	logs   []appTypes.Applog
}

func newErrorWithLog(base error, app *App, action string) *errorWithLog {
	logErr := &errorWithLog{
		err:    base,
		action: action,
	}
	if startupErr, ok := provision.IsStartupError(base); ok {
		logErr.logs = startupErr.CrashedUnitsLogs
		if logErr.logs != nil {
			return logErr
		}

		tokenEnv, _ := servicemanager.AppEnvVar.Get(context.Background(), app, "TSURU_APP_TOKEN")
		token, _ := AuthScheme.Auth(app.ctx, tokenEnv.Value)

		logErr.logs, _ = app.LastLogs(app.ctx, servicemanager.AppLog, appTypes.ListLogArgs{
			Source:       "tsuru",
			InvertSource: true,
			Units:        startupErr.CrashedUnits,
			Token:        token,
			Limit:        10,
		})
	}
	return logErr
}

func (e *errorWithLog) Cause() error {
	return e.err
}

func (e *errorWithLog) formatLogLines() string {
	const timeFormat = "2006-01-02 15:04:05 -0700"
	linesStr := make([]string, len(e.logs))
	for i, l := range e.logs {
		linesStr[i] = fmt.Sprintf("    %s [%s][%s]: %s", l.Date.Local().Format(timeFormat), l.Source, l.Unit, l.Message)
	}
	return strings.Join(linesStr, "\n")
}

func (e *errorWithLog) Error() string {
	var logPart string
	if len(e.logs) > 0 {
		logPart = fmt.Sprintf("\n---- Last %d log messages: ----\n%s", len(e.logs), e.formatLogLines())
	}
	return fmt.Sprintf("\n---- ERROR during %s: ----\n%v\n%s", e.action, e.err, logPart)
}

func validateVersions(ctx context.Context, opts DeployOptions) error {
	if opts.NewVersion && opts.OverrideVersions {
		return errors.New("conflicting deploy flags, new-version and override-old-versions")
	}
	if opts.NewVersion || opts.OverrideVersions {
		return nil
	}
	multi, err := opts.App.hasMultipleVersions(ctx)
	if err != nil {
		return err
	}
	if multi {
		return errors.New("multiple versions currently deployed, either new-version or override-old-versions must be set")
	}
	return nil
}

// Deploy runs a deployment of an application. It will first try to run an
// archive based deploy (if opts.ArchiveURL is not empty), and then fallback to
// the Git based deployment.
func Deploy(ctx context.Context, opts DeployOptions) (string, error) {
	if opts.Event == nil {
		return "", errors.Errorf("missing event in deploy opts")
	}
	err := validateVersions(ctx, opts)
	if err != nil {
		return "", err
	}
	logWriter := LogWriter{AppName: opts.App.Name}
	logWriter.Async()
	defer logWriter.Close()
	opts.Event.SetLogWriter(io.MultiWriter(&tsuruIo.NoErrorWriter{Writer: opts.OutputStream}, &logWriter))
	imageID, err := deployToProvisioner(ctx, &opts, opts.Event)
	rebuild.RoutesRebuildOrEnqueueWithProgress(opts.App.Name, opts.Event)
	if err != nil {
		return "", newErrorWithLog(err, opts.App, "deploy")
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

func RollbackUpdate(ctx context.Context, app *App, imageID, reason string, disableRollback bool) error {
	version, err := servicemanager.AppVersion.VersionByImageOrVersion(ctx, app, imageID)
	if err != nil {
		return err
	}
	return version.ToggleEnabled(!disableRollback, reason)
}

func deployToProvisioner(ctx context.Context, opts *DeployOptions, evt *event.Event) (string, error) {
	prov, err := opts.App.getProvisioner()
	if err != nil {
		return "", err
	}
	if opts.Kind == "" {
		opts.GetKind()
	}
	if opts.App.GetPlatform() == "" && opts.Kind != DeployImage && opts.Kind != DeployRollback {
		return "", errors.Errorf("can't deploy app without platform, if it's not an image or rollback")
	}

	deployer, ok := prov.(provision.BuilderDeploy)
	if !ok {
		return "", provision.ProvisionerNotSupported{Prov: prov, Action: fmt.Sprintf("%s deploy", opts.Kind)}
	}

	var version appTypes.AppVersion
	if opts.Kind == DeployRollback {
		version, err = servicemanager.AppVersion.VersionByImageOrVersion(ctx, opts.App, opts.Image)
		if err != nil {
			return "", err
		}
		versionInfo := version.VersionInfo()
		if versionInfo.MarkedToRemoval {
			return "", appTypes.ErrVersionMarkedToRemoval
		} else if versionInfo.Disabled {
			return "", errors.Errorf("the selected version is disabled for rollback: %s", version.VersionInfo().DisabledReason)
		}
	} else {
		version, err = builderDeploy(ctx, deployer, opts, evt)
		if err != nil {
			return "", err
		}
	}
	return deployer.Deploy(ctx, provision.DeployArgs{
		App:              opts.App,
		Version:          version,
		Event:            evt,
		PreserveVersions: opts.NewVersion,
		OverrideVersions: opts.OverrideVersions,
	})
}

func builderDeploy(ctx context.Context, prov provision.BuilderDeploy, opts *DeployOptions, evt *event.Event) (appTypes.AppVersion, error) {
	isRebuild := opts.Kind == DeployRebuild
	buildOpts := builder.BuildOpts{
		BuildFromFile: opts.Build,
		ArchiveURL:    opts.ArchiveURL,
		ArchiveFile:   opts.File,
		ArchiveSize:   opts.FileSize,
		Rebuild:       isRebuild,
		ImageID:       opts.Image,
		Tag:           opts.BuildTag,
		Message:       opts.Message,
	}
	builder, err := opts.App.getBuilder()
	if err != nil {
		return nil, err
	}
	version, err := builder.Build(ctx, prov, opts.App, evt, &buildOpts)
	if buildOpts.IsTsuruBuilderImage {
		opts.Kind = DeployBuildedImage
	}
	return version, err
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
	evt.StructuredLog = []event.LogEntry{
		{Message: data.Log},
	}
	a, err := GetByName(context.TODO(), data.App)
	if err == nil {
		evt.Allowed = event.Allowed(permission.PermAppReadEvents, append(permission.Contexts(permTypes.CtxTeam, a.Teams),
			permission.Context(permTypes.CtxApp, a.Name),
			permission.Context(permTypes.CtxPool, a.Pool),
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
