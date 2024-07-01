// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	eventTypes "github.com/tsuru/tsuru/types/event"
)

var (
	appBuildsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tsuru",
		Subsystem: "app",
		Name:      "builds_total",
		Help:      "Total number of app builds",
	}, []string{"app", "status", "kind", "platform"})

	appBuildDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tsuru",
		Subsystem: "app",
		Name:      "build_duration_seconds",
		Buckets:   []float64{0, 30, 60, 120, 180, 240, 300, 600, 900, 1200}, // 0s, 30s, 1min, 2min, 3min, 4min, 5min, 10min, 15min, 30min
		Help:      "Duration in seconds of app build",
	}, []string{"app", "status", "kind", "platform"})
)

func init() {
	prometheus.MustRegister(appBuildsTotal)
	prometheus.MustRegister(appBuildDuration)
}

// title: app build
// path: /apps/{appname}/build
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//
//	200: OK
//	400: Invalid data
//	403: Forbidden
//	404: Not found
func build(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	startingBuildTime := time.Now()
	ctx := r.Context()
	tag := InputValue(r, "tag")
	if tag == "" {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "you must specify the image tag.",
		}
	}
	opts, err := prepareToBuild(r)
	if err != nil {
		return err
	}
	if opts.File != nil {
		defer opts.File.Close()
	}
	w.Header().Set("Content-Type", "text")
	appName := r.URL.Query().Get(":appname")
	userName := t.GetUserName()
	instance, err := app.GetByName(ctx, appName)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	opts.App = instance
	opts.Build = true
	opts.BuildTag = tag
	opts.User = userName
	opts.GetKind()
	canBuild := permission.Check(t, permission.PermAppBuild, contextsForApp(instance)...)
	if !canBuild {
		return &tsuruErrors.HTTP{Code: http.StatusForbidden, Message: "User does not have permission to do this action in this app"}
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:        appTarget(appName),
		Kind:          permission.PermAppBuild,
		RawOwner:      eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: userName},
		RemoteAddr:    r.RemoteAddr,
		CustomData:    opts,
		Allowed:       event.Allowed(permission.PermAppReadEvents, contextsForApp(instance)...),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, contextsForApp(instance)...),
		Cancelable:    true,
	})
	if err != nil {
		return err
	}
	var imageID string
	defer func() {
		evt.DoneCustomData(ctx, err, map[string]string{"image": imageID})
		labels := prometheus.Labels{"app": appName, "status": deployStatus(evt), "kind": string(opts.GetKind()), "platform": opts.App.Platform}
		appBuildDuration.With(labels).Observe(time.Since(startingBuildTime).Seconds())
		appBuildsTotal.With(labels).Inc()
	}()
	ctx, cancel := evt.CancelableContext(ctx)
	defer cancel()
	opts.Event = evt
	writer := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "please wait...")
	defer writer.Stop()
	opts.OutputStream = writer
	imageID, err = app.Build(ctx, opts)
	if err == nil {
		fmt.Fprintln(w, imageID)
		fmt.Fprintln(w, "OK")
	}
	return err
}

func prepareToBuild(r *http.Request) (opts app.DeployOptions, err error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		var fh *multipart.FileHeader

		opts.File, fh, err = r.FormFile("file")
		if err != nil && !errors.Is(err, http.ErrMissingFile) {
			return opts, &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}

		opts.FileSize = fh.Size
	}

	opts.ArchiveURL = InputValue(r, "archive-url")
	opts.Image = InputValue(r, "image")
	opts.Dockerfile = InputValue(r, "dockerfile")

	if opts.ArchiveURL != "" && (opts.FileSize > 0 || opts.Image != "" || opts.Dockerfile != "") {
		return opts, &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: `Cannot set "archive-url" mutually with "dockerfile", "file" or "image" fields`,
		}
	}

	if opts.Image != "" && (opts.FileSize > 0 || opts.ArchiveURL != "" || opts.Dockerfile != "") {
		return opts, &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: `Cannot set "image" mutually with "archive-url", "dockerfile" or "file" fields`,
		}
	}

	if opts.Image == "" && opts.ArchiveURL == "" && opts.Dockerfile == "" && opts.FileSize == 0 {
		return opts, &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: `You must provide at least one of: "archive-url", "dockerfile", "image" or "file"`,
		}
	}

	return
}
