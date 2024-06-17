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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
)

const eventIDHeader = "X-Tsuru-Eventid"

var (
	appDeploysTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tsuru",
		Subsystem: "app",
		Name:      "deploys_total",
		Help:      "Total number of app deploys",
	}, []string{"app", "status", "kind", "platform"})

	appDeployDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tsuru",
		Subsystem: "app",
		Name:      "deploy_duration_seconds",
		Buckets:   []float64{0, 30, 60, 120, 180, 240, 300, 600, 900, 1200}, // 0s, 30s, 1min, 2min, 3min, 4min, 5min, 10min, 15min, 30min
		Help:      "Duration in seconds of app deploy",
	}, []string{"app", "status", "kind", "platform"})
)

func init() {
	prometheus.MustRegister(appDeploysTotal)
	prometheus.MustRegister(appDeployDuration)
}

// title: app deploy
// path: /apps/{appname}/deploy
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//
//	200: OK
//	400: Invalid data
//	403: Forbidden
//	404: Not found
func deploy(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	startingDeployTime := time.Now()
	ctx := r.Context()
	opts, err := prepareToBuild(r)
	if err != nil {
		return err
	}
	if opts.File != nil {
		defer opts.File.Close()
	}
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

	userName := t.GetUserName()

	instance, err := app.GetByName(ctx, appName)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	message := InputValue(r, "message")
	opts.App = instance
	opts.User = userName
	opts.Origin = origin
	opts.Message = message
	opts.NewVersion, _ = strconv.ParseBool(InputValue(r, "new-version"))
	opts.OverrideVersions, _ = strconv.ParseBool(InputValue(r, "override-versions"))
	opts.GetKind()
	canDeploy := permission.Check(t, permSchemeForDeploy(opts), contextsForApp(instance)...)
	if !canDeploy {
		return &tsuruErrors.HTTP{Code: http.StatusForbidden, Message: "User does not have permission to do this action in this app"}
	}

	var imageID string
	evt, err := event.New(&event.Opts{
		Target:        appTarget(appName),
		Kind:          permission.PermAppDeploy,
		RawOwner:      event.Owner{Type: event.OwnerTypeUser, Name: userName},
		RemoteAddr:    r.RemoteAddr,
		CustomData:    opts,
		Allowed:       event.Allowed(permission.PermAppReadEvents, contextsForApp(instance)...),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, contextsForApp(instance)...),
		Cancelable:    true,
	})
	if err != nil {
		return err
	}
	defer func() {
		evt.DoneCustomData(err, map[string]string{"image": imageID})
		labels := prometheus.Labels{"app": appName, "status": deployStatus(evt), "kind": string(opts.GetKind()), "platform": opts.App.Platform}
		appDeployDuration.With(labels).Observe(time.Since(startingDeployTime).Seconds())
		appDeploysTotal.With(labels).Inc()
	}()
	ctx, cancel := evt.CancelableContext(ctx)
	defer cancel()
	w.Header().Set(eventIDHeader, evt.UniqueID.Hex())
	opts.Event = evt
	writer := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "please wait...")
	defer writer.Stop()
	opts.OutputStream = writer
	imageID, err = app.Deploy(ctx, opts)
	if err == nil {
		fmt.Fprintln(w, "\nOK")
	}
	return err
}

func deployStatus(evt *event.Event) string {
	if evt == nil {
		return "unknown"
	}
	if evt.CancelInfo.Canceled {
		return "canceled"
	}
	if evt.Error != "" {
		return "error"
	}
	return "success"
}

func permSchemeForDeploy(opts app.DeployOptions) *permission.PermissionScheme {
	switch opts.GetKind() {
	case provisionTypes.DeployGit:
		return permission.PermAppDeployGit
	case provisionTypes.DeployImage:
		return permission.PermAppDeployImage
	case provisionTypes.DeployUpload:
		return permission.PermAppDeployUpload
	case provisionTypes.DeployUploadBuild:
		return permission.PermAppDeployBuild
	case provisionTypes.DeployArchiveURL:
		return permission.PermAppDeployArchiveUrl
	case provisionTypes.DeployRollback:
		return permission.PermAppDeployRollback
	case provisionTypes.DeployDockerfile:
		return permission.PermAppDeployDockerfile
	default:
		return permission.PermAppDeploy
	}
}

// title: deploy diff
// path: /apps/{appname}/diff
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//
//	410: Gone
func diffDeploy(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return &tsuruErrors.HTTP{Code: http.StatusGone, Message: "diff deploy is deprecated, this call does nothing"}
}

// title: rollback
// path: /apps/{app}/deploy/rollback
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//
//	200: OK
//	400: Invalid data
//	403: Forbidden
//	404: Not found
func deployRollback(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	appName := r.URL.Query().Get(":app")
	instance, err := app.GetByName(ctx, appName)
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
	opts.NewVersion, _ = strconv.ParseBool(InputValue(r, "new-version"))
	opts.OverrideVersions, _ = strconv.ParseBool(InputValue(r, "override-versions"))
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
		RemoteAddr:    r.RemoteAddr,
		CustomData:    opts,
		Allowed:       event.Allowed(permission.PermAppReadEvents, contextsForApp(instance)...),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, contextsForApp(instance)...),
		Cancelable:    true,
	})
	if err != nil {
		return err
	}
	defer func() { evt.DoneCustomData(err, map[string]string{"image": imageID}) }()
	ctx, cancel := evt.CancelableContext(ctx)
	defer cancel()
	opts.Event = evt
	imageID, err = app.Deploy(ctx, opts)
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
//
//	200: OK
//	204: No content
func deploysList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
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
	deploys, err := app.ListDeploys(ctx, filter, skipInt, limitInt)
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
//
//	200: OK
//	401: Unauthorized
//	404: Not found
func deployInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	depID := r.URL.Query().Get(":deploy")
	deploy, err := app.GetDeploy(depID)
	if err != nil {
		if err == event.ErrEventNotFound {
			return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: "Deploy not found."}
		}
		return err
	}
	dbApp, err := app.GetByName(ctx, deploy.App)
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
// path: /apps/{app}/deploy/rebuild
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//
//	200: OK
//	400: Invalid data
//	403: Forbidden
//	404: Not found
func deployRebuild(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	appName := r.URL.Query().Get(":app")
	instance, err := app.GetByName(ctx, appName)
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
		Kind:         provisionTypes.DeployRebuild,
	}
	opts.NewVersion, _ = strconv.ParseBool(InputValue(r, "new-version"))
	opts.OverrideVersions, _ = strconv.ParseBool(InputValue(r, "override-versions"))
	canDeploy := permission.Check(t, permSchemeForDeploy(opts), contextsForApp(instance)...)
	if !canDeploy {
		return &tsuruErrors.HTTP{Code: http.StatusForbidden, Message: permission.ErrUnauthorized.Error()}
	}
	var imageID string
	evt, err := event.New(&event.Opts{
		Target:        appTarget(appName),
		Kind:          permission.PermAppDeploy,
		Owner:         t,
		RemoteAddr:    r.RemoteAddr,
		CustomData:    opts,
		Allowed:       event.Allowed(permission.PermAppReadEvents, contextsForApp(instance)...),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, contextsForApp(instance)...),
		Cancelable:    true,
	})
	if err != nil {
		return err
	}
	defer func() { evt.DoneCustomData(err, map[string]string{"image": imageID}) }()
	ctx, cancel := evt.CancelableContext(ctx)
	defer cancel()
	opts.Event = evt
	imageID, err = app.Deploy(ctx, opts)
	if err != nil {
		return err
	}
	return nil
}

// title: rollback update
// path: /apps/{app}/deploy/rollback/update
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//
//	200: Rollback updated
//	400: Invalid data
//	403: Forbidden
func deployRollbackUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	appName := r.URL.Query().Get(":app")
	instance, err := app.GetByName(ctx, appName)
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
		RemoteAddr:    r.RemoteAddr,
		CustomData:    event.FormToCustomData(InputFields(r)),
		Allowed:       event.Allowed(permission.PermAppReadEvents, contextsForApp(instance)...),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents, contextsForApp(instance)...),
		Cancelable:    false,
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = app.RollbackUpdate(ctx, instance, img, reason, disableRollback)
	if err != nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	return err
}
