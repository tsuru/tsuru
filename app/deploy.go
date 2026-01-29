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

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	"github.com/tsuru/tsuru/streamfmt"
	appTypes "github.com/tsuru/tsuru/types/app"
	eventTypes "github.com/tsuru/tsuru/types/event"
	provisionTypes "github.com/tsuru/tsuru/types/provision"

	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var reImageVersion = regexp.MustCompile(":v([0-9]+)$")

type DeployData struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
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
			if version.DeploySuccessful && version.DeployImage != "" && !version.Disabled {
				validImages.Add(version.DeployImage)
			}
		}
	}
	return validImages, nil
}

// ListDeploys returns the list of deploy that match a given filter.
func ListDeploys(ctx context.Context, filter *Filter, skip, limit int) ([]DeployData, error) {
	var rawFilter mongoBSON.M
	if !filter.IsEmpty() {
		appsList, err := List(ctx, filter)
		if err != nil {
			return nil, err
		}
		apps := make([]string, len(appsList))
		for i, a := range appsList {
			apps[i] = a.Name
		}
		rawFilter = mongoBSON.M{"target.value": mongoBSON.M{"$in": apps}}
	}
	evts, err := event.List(ctx, &event.Filter{
		Target:    eventTypes.Target{Type: eventTypes.TargetTypeApp},
		Raw:       rawFilter,
		KindNames: []string{permission.PermAppDeploy.FullName()},
		KindType:  eventTypes.KindTypePermission,
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

func GetDeploy(ctx context.Context, id string) (*DeployData, error) {
	if _, err := primitive.ObjectIDFromHex(id); err != nil {
		return nil, errors.Errorf("id parameter is not ObjectId: %s", id)
	}
	evt, err := event.GetByHexID(ctx, id)
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
	App              *appTypes.App
	Commit           string
	BuildTag         string
	ArchiveURL       string
	Dockerfile       string
	FileSize         int64
	File             io.ReadCloser `bson:"-"`
	OutputStream     io.Writer     `bson:"-"`
	User             string
	Image            string
	Origin           string
	Event            *event.Event `bson:"-"`
	Kind             provisionTypes.DeployKind
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

func (o *DeployOptions) GetKind() (kind provisionTypes.DeployKind) {
	if o.Kind != "" {
		return o.Kind
	}

	defer func() { o.Kind = kind }()

	if o.Dockerfile != "" {
		return provisionTypes.DeployDockerfile
	}

	if o.Rollback {
		return provisionTypes.DeployRollback
	}

	if o.Image != "" {
		return provisionTypes.DeployImage
	}

	if o.File != nil {
		if o.Build {
			return provisionTypes.DeployUploadBuild
		}
		return provisionTypes.DeployUpload
	}

	if o.Commit != "" {
		return provisionTypes.DeployGit
	}

	if o.ArchiveURL != "" {
		return provisionTypes.DeployArchiveURL
	}

	return provisionTypes.DeployKind("")
}

func Build(ctx context.Context, opts DeployOptions) (string, error) {
	if opts.Event == nil {
		return "", errors.Errorf("missing event in build opts")
	}
	logWriter := LogWriter{AppName: opts.App.Name}
	logWriter.Async()
	defer logWriter.Close()
	opts.Event.SetLogWriter(io.MultiWriter(&tsuruIo.NoErrorWriter{Writer: opts.OutputStream}, &logWriter))
	if opts.App.Platform == "" {
		return "", errors.Errorf("can't build app without platform")
	}
	version, err := builderDeploy(ctx, &opts, opts.Event)
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

func newErrorWithLog(ctx context.Context, base error, app *appTypes.App, action string) *errorWithLog {
	logErr := &errorWithLog{
		err:    base,
		action: action,
	}
	if startupErr, ok := provision.IsStartupError(base); ok {
		logErr.logs = startupErr.CrashedUnitsLogs
		if logErr.logs != nil {
			return logErr
		}

		logErr.logs, _ = LastLogs(ctx, app, servicemanager.LogService, appTypes.ListLogArgs{
			Source:       "tsuru",
			InvertSource: true,
			Units:        startupErr.CrashedUnits,
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
		logPart = "\n" + streamfmt.Sectionf("Last %d log messages:", len(e.logs)) + "\n" + e.formatLogLines()
	}
	return "\n" + streamfmt.Sectionf("ERROR during %s:", e.action) + "\n" + fmt.Sprintf("%v", e.err) + "\n" + logPart
}

func validateVersions(ctx context.Context, opts DeployOptions) error {
	if opts.NewVersion && opts.OverrideVersions {
		return errors.New("conflicting deploy flags, new-version and override-old-versions")
	}
	if opts.NewVersion || opts.OverrideVersions {
		return nil
	}
	multi, err := hasMultipleVersions(ctx, opts.App)
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
	if err != nil {
		return "", newErrorWithLog(ctx, err, opts.App, "deploy")
	}
	err = rebuild.RebuildRoutesWithAppName(opts.App.Name, opts.Event)
	if err != nil {
		return "", err
	}
	err = incrementDeploy(ctx, opts.App)
	if err != nil {
		log.Errorf("WARNING: couldn't increment deploy count, deploy opts: %#v", opts)
	}
	if opts.Kind == provisionTypes.DeployImage || opts.Kind == provisionTypes.DeployRollback {
		if !opts.App.UpdatePlatform {
			SetUpdatePlatform(ctx, opts.App, true)
		}
	} else if opts.App.UpdatePlatform {
		SetUpdatePlatform(ctx, opts.App, false)
	}
	return imageID, nil
}

func RollbackUpdate(ctx context.Context, app *appTypes.App, imageID, reason string, disableRollback bool) error {
	version, err := servicemanager.AppVersion.VersionByImageOrVersion(ctx, app, imageID)
	if err != nil {
		return err
	}
	return version.ToggleEnabled(!disableRollback, reason)
}

func deployToProvisioner(ctx context.Context, opts *DeployOptions, evt *event.Event) (string, error) {
	prov, err := getProvisioner(ctx, opts.App)
	if err != nil {
		return "", err
	}
	if opts.Kind == "" {
		opts.GetKind()
	}

	if opts.App.Platform == "" && opts.Kind != provisionTypes.DeployImage && opts.Kind != provisionTypes.DeployRollback && opts.Kind != provisionTypes.DeployDockerfile {
		return "", errors.Errorf("can't deploy app without platform, if it's not an image, dockerfile or rollback")
	}

	deployer, ok := prov.(provision.BuilderDeploy)
	if !ok {
		return "", provision.ProvisionerNotSupported{Prov: prov, Action: fmt.Sprintf("%s deploy", opts.Kind)}
	}

	var version appTypes.AppVersion
	if opts.Kind == provisionTypes.DeployRollback {
		version, err = servicemanager.AppVersion.VersionByImageOrVersion(ctx, (*appTypes.App)(opts.App), opts.Image)
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
		version, err = builderDeploy(ctx, opts, evt)
		if err != nil {
			return "", err
		}
	}

	err = evt.SetCancelable(ctx, false)
	if err != nil {
		return "", errors.Wrap(err, "failed to set event as non-cancelable")
	}

	return deployer.Deploy(ctx, provision.DeployArgs{
		App:              opts.App,
		Version:          version,
		Event:            evt,
		PreserveVersions: opts.NewVersion,
		OverrideVersions: opts.OverrideVersions,
	})
}

func builderDeploy(ctx context.Context, opts *DeployOptions, evt *event.Event) (appTypes.AppVersion, error) {
	buildOpts := builder.BuildOpts{
		Rebuild:     opts.GetKind() == provisionTypes.DeployRebuild,
		ArchiveURL:  opts.ArchiveURL,
		ArchiveFile: opts.File,
		ArchiveSize: opts.FileSize,
		ImageID:     opts.Image,
		Tag:         opts.BuildTag,
		Message:     opts.Message,
		Output:      evt,
		Dockerfile:  opts.Dockerfile,
	}

	b, err := getBuilder(ctx, opts.App)
	if err != nil {
		return nil, err
	}

	var version appTypes.AppVersion
	version, err = b.Build(ctx, opts.App, evt, buildOpts)
	if err != nil {
		return nil, err
	}

	return version, nil
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

func incrementDeploy(ctx context.Context, app *appTypes.App) error {
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	_, err = collection.UpdateOne(
		ctx,
		mongoBSON.M{"name": app.Name},
		mongoBSON.M{"$inc": mongoBSON.M{"deploys": 1}},
	)
	if err == nil {
		app.Deploys += 1
	}
	return err
}
