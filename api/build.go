// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
)

// title: app build
// path: /apps/{appname}/build
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: OK
//   400: Invalid data
//   403: Forbidden
//   404: Not found
func build(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	tag := r.FormValue("tag")
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
	var userName string
	if t.IsAppToken() {
		if t.GetAppName() != appName && t.GetAppName() != app.InternalAppName {
			return &tsuruErrors.HTTP{Code: http.StatusUnauthorized, Message: "invalid app token"}
		}
		userName = r.FormValue("user")
	} else {
		userName = t.GetUserName()
	}
	instance, err := app.GetByName(appName)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	opts.App = instance
	opts.BuildTag = tag
	opts.User = userName
	opts.GetKind()
	if t.GetAppName() != app.InternalAppName {
		canBuild := permission.Check(t, permission.PermAppBuild, contextsForApp(instance)...)
		if !canBuild {
			return &tsuruErrors.HTTP{Code: http.StatusForbidden, Message: "User does not have permission to do this action in this app"}
		}
	}
	evt, err := event.New(&event.Opts{
		Target:        appTarget(appName),
		Kind:          permission.PermAppBuild,
		RawOwner:      event.Owner{Type: event.OwnerTypeUser, Name: userName},
		CustomData:    opts,
		Allowed:       event.Allowed(permission.PermAppReadEvents, contextsForApp(instance)...),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, contextsForApp(instance)...),
		Cancelable:    true,
	})
	if err != nil {
		return err
	}
	var imageID string
	defer func() { evt.DoneCustomData(err, map[string]string{"image": imageID}) }()
	opts.Event = evt
	writer := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "please wait...")
	defer writer.Stop()
	opts.OutputStream = writer
	imageID, err = app.Build(opts)
	if err == nil {
		fmt.Fprintln(w, imageID)
		fmt.Fprintln(w, "OK")
	}
	return err
}

func prepareToBuild(r *http.Request) (opts app.DeployOptions, err error) {
	var file multipart.File
	var fileSize int64
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		file, _, err = r.FormFile("file")
		if err != nil {
			return opts, &tsuruErrors.HTTP{
				Code:    http.StatusBadRequest,
				Message: err.Error(),
			}
		}
		fileSize, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			return opts, errors.Wrap(err, "unable to find uploaded file size")
		}
		file.Seek(0, io.SeekStart)
	}
	archiveURL := r.FormValue("archive-url")
	image := r.FormValue("image")
	if image == "" && archiveURL == "" && file == nil {
		return opts, &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "you must specify either the archive-url, a image url or upload a file.",
		}
	}
	var build bool
	buildString := r.FormValue("build")
	if buildString != "" {
		build, err = strconv.ParseBool(buildString)
		if err != nil {
			return opts, &tsuruErrors.HTTP{
				Code:    http.StatusBadRequest,
				Message: err.Error(),
			}
		}
	}
	opts.FileSize = fileSize
	opts.File = file
	opts.ArchiveURL = archiveURL
	opts.Image = image
	opts.Build = build
	return
}
