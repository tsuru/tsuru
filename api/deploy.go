// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/repository"
)

const eventIDHeader = "X-Tsuru-Eventid"

// title: app deploy
// path: /apps/{appname}/deploy
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: OK
//   400: Invalid data
//   403: Forbidden
//   404: Not found
func deploy(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	opts, err := prepareToBuild(r)
	if err != nil {
		return err
	}
	if opts.File != nil {
		defer opts.File.Close()
	}
	commit := InputValue(r, "commit")
	w.Header().Set("Content-Type", "text")
	appName := r.URL.Query().Get(":appname")
	origin := InputValue(r, "origin")
	if opts.Image != "" {
		origin = "image"
	}
	if origin != "" {
		if !app.ValidateOrigin(origin) {
			return &tsuruErrors.HTTP{
				Code:    http.StatusBadRequest,
				Message: "Invalid deployment origin",
			}
		}
	}
	var userName string
	if t.IsAppToken() {
		if t.GetAppName() != appName && t.GetAppName() != app.InternalAppName {
			return &tsuruErrors.HTTP{Code: http.StatusUnauthorized, Message: "invalid app token"}
		}
		userName = InputValue(r, "user")
	} else {
		commit = ""
		userName = t.GetUserName()
	}
	instance, err := app.GetByName(appName)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	message := InputValue(r, "message")
	if commit != "" && message == "" {
		var messages []string
		messages, err = repository.Manager().CommitMessages(instance.Name, commit, 1)
		if err != nil {
			return err
		}
		if len(messages) > 0 {
			message = messages[0]
		}
	}
	if origin == "" && commit != "" {
		origin = "git"
	}
	opts.App = instance
	opts.Commit = commit
	opts.User = userName
	opts.Origin = origin
	opts.Message = message
	opts.Token = t
	opts.GetKind()
	if t.GetAppName() != app.InternalAppName {
		canDeploy := permission.Check(t, permSchemeForDeploy(opts), contextsForApp(instance)...)
		if !canDeploy {
			return &tsuruErrors.HTTP{Code: http.StatusForbidden, Message: "User does not have permission to do this action in this app"}
		}
	}
	var imageID string
	evt, err := event.New(&event.Opts{
		Target:        appTarget(appName),
		Kind:          permission.PermAppDeploy,
		RawOwner:      event.Owner{Type: event.OwnerTypeUser, Name: userName},
		CustomData:    opts,
		Allowed:       event.Allowed(permission.PermAppReadEvents, contextsForApp(instance)...),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, contextsForApp(instance)...),
		Cancelable:    true,
	})
	if err != nil {
		return err
	}
	defer func() { evt.DoneCustomData(err, map[string]string{"image": imageID}) }()
	w.Header().Set(eventIDHeader, evt.UniqueID.Hex())
	opts.Event = evt
	writer := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "please wait...")
	defer writer.Stop()
	opts.OutputStream = writer
	imageID, err = app.Deploy(opts)
	if err == nil {
		fmt.Fprintln(w, "\nOK")
	}
	return err
}

func permSchemeForDeploy(opts app.DeployOptions) *permission.PermissionScheme {
	switch opts.GetKind() {
	case app.DeployGit:
		return permission.PermAppDeployGit
	case app.DeployImage:
		return permission.PermAppDeployImage
	case app.DeployUpload:
		return permission.PermAppDeployUpload
	case app.DeployUploadBuild:
		return permission.PermAppDeployBuild
	case app.DeployArchiveURL:
		return permission.PermAppDeployArchiveUrl
	case app.DeployRollback:
		return permission.PermAppDeployRollback
	default:
		return permission.PermAppDeploy
	}
}

// title: deploy diff
// path: /apps/{appname}/diff
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: OK
//   400: Invalid data
//   403: Forbidden
//   404: Not found
func diffDeploy(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	writer := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer writer.Stop()
	fmt.Fprint(w, "Saving the difference between the old and new code\n")
	appName := r.URL.Query().Get(":appname")
	diff := InputValue(r, "customdata")
	instance, err := app.GetByName(appName)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if t.GetAppName() != app.InternalAppName {
		canDiffDeploy := permission.Check(t, permission.PermAppReadDeploy, contextsForApp(instance)...)
		if !canDiffDeploy {
			return &tsuruErrors.HTTP{Code: http.StatusForbidden, Message: permission.ErrUnauthorized.Error()}
		}
	}
	evt, err := event.GetRunning(appTarget(appName), permission.PermAppDeploy.FullName())
	if err != nil {
		return err
	}
	return evt.SetOtherCustomData(map[string]string{
		"diff": diff,
	})
}

// title: rollback
// path: /apps/{appname}/deploy/rollback
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: OK
//   400: Invalid data
//   403: Forbidden
//   404: Not found
func deployRollback(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get(":appname")
	instance, err := app.GetByName(appName)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", appName)}
	}
	image := InputValue(r, "image")
	if image == "" {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "you cannot rollback without an image name",
		}
	}
	origin := InputValue(r, "origin")
	if origin != "" {
		if !app.ValidateOrigin(origin) {
			return &tsuruErrors.HTTP{
				Code:    http.StatusBadRequest,
				Message: "Invalid deployment origin",
			}
		}
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	opts := app.DeployOptions{
		App:          instance,
		OutputStream: writer,
		Image:        image,
		User:         t.GetUserName(),
		Origin:       origin,
		Rollback:     true,
	}
	opts.GetKind()
	canRollback := permission.Check(t, permSchemeForDeploy(opts), contextsForApp(instance)...)
	if !canRollback {
		return &tsuruErrors.HTTP{Code: http.StatusForbidden, Message: permission.ErrUnauthorized.Error()}
	}
	var imageID string
	evt, err := event.New(&event.Opts{
		Target:        appTarget(appName),
		Kind:          permission.PermAppDeploy,
		Owner:         t,
		CustomData:    opts,
		Allowed:       event.Allowed(permission.PermAppReadEvents, contextsForApp(instance)...),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, contextsForApp(instance)...),
		Cancelable:    true,
	})
	if err != nil {
		return err
	}
	defer func() { evt.DoneCustomData(err, map[string]string{"image": imageID}) }()
	opts.Event = evt
	imageID, err = app.Deploy(opts)
	if err != nil {
		return err
	}
	return nil
}

// title: deploy list
// path: /deploys
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
func deploysList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	contexts := permission.ContextsForPermission(t, permission.PermAppReadDeploy)
	if len(contexts) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	filter := appFilterByContext(contexts, nil)
	filter.Name = r.URL.Query().Get("app")
	skip := r.URL.Query().Get("skip")
	limit := r.URL.Query().Get("limit")
	skipInt, _ := strconv.Atoi(skip)
	limitInt, _ := strconv.Atoi(limit)
	deploys, err := app.ListDeploys(filter, skipInt, limitInt)
	if err != nil {
		return err
	}
	if len(deploys) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(deploys)
}

// title: deploy info
// path: /deploys/{deploy}
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: Not found
func deployInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	depID := r.URL.Query().Get(":deploy")
	deploy, err := app.GetDeploy(depID)
	if err != nil {
		if err == event.ErrEventNotFound {
			return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: "Deploy not found."}
		}
		return err
	}
	dbApp, err := app.GetByName(deploy.App)
	if err != nil {
		return err
	}
	canGet := permission.Check(t, permission.PermAppReadDeploy, contextsForApp(dbApp)...)
	if !canGet {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: "Deploy not found."}
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(deploy)
}

// title: rebuild
// path: /apps/{appname}/deploy/rebuild
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: OK
//   400: Invalid data
//   403: Forbidden
//   404: Not found
func deployRebuild(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get(":appname")
	instance, err := app.GetByName(appName)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", appName)}
	}
	origin := InputValue(r, "origin")
	if !app.ValidateOrigin(origin) {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid deployment origin",
		}
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	opts := app.DeployOptions{
		App:          instance,
		OutputStream: writer,
		User:         t.GetUserName(),
		Origin:       origin,
		Kind:         app.DeployRebuild,
	}
	canDeploy := permission.Check(t, permSchemeForDeploy(opts), contextsForApp(instance)...)
	if !canDeploy {
		return &tsuruErrors.HTTP{Code: http.StatusForbidden, Message: permission.ErrUnauthorized.Error()}
	}
	var imageID string
	evt, err := event.New(&event.Opts{
		Target:        appTarget(appName),
		Kind:          permission.PermAppDeploy,
		Owner:         t,
		CustomData:    opts,
		Allowed:       event.Allowed(permission.PermAppReadEvents, contextsForApp(instance)...),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, contextsForApp(instance)...),
		Cancelable:    true,
	})
	if err != nil {
		return err
	}
	defer func() { evt.DoneCustomData(err, map[string]string{"image": imageID}) }()
	opts.Event = evt
	imageID, err = app.Deploy(opts)
	if err != nil {
		return err
	}
	return nil
}

// title: rollback update
// path: /apps/{appname}/deploy/rollback/update
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: Rollback updated
//   400: Invalid data
//   403: Forbidden
func deployRollbackUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get(":appname")
	instance, err := app.GetByName(appName)
	if err != nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("App %s was not found", appName),
		}
	}
	canUpdateRollback := permission.Check(t, permission.PermAppUpdateDeployRollback, contextsForApp(instance)...)
	if !canUpdateRollback {
		return &tsuruErrors.HTTP{
			Code:    http.StatusForbidden,
			Message: "User does not have permission to do this action in this app",
		}
	}
	img := InputValue(r, "image")
	if img == "" {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "you must specify an image",
		}
	}
	disable := InputValue(r, "disable")
	disableRollback, err := strconv.ParseBool(disable)
	if err != nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Cannot set 'disable' status to: '%s', instead of 'true' or 'false'", disable),
		}
	}
	reason := InputValue(r, "reason")
	if (reason == "") && (disableRollback) {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Reason cannot be empty while disabling a image rollback",
		}
	}
	evt, err := event.New(&event.Opts{
		Target:        appTarget(appName),
		Kind:          permission.PermAppUpdateDeployRollback,
		Owner:         t,
		CustomData:    event.FormToCustomData(InputFields(r)),
		Allowed:       event.Allowed(permission.PermAppReadEvents, contextsForApp(instance)...),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, contextsForApp(instance)...),
		Cancelable:    false,
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = app.RollbackUpdate(instance.Name, img, reason, disableRollback)
	if err != nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	return err
}
