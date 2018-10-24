// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ajg/form"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	apiTypes "github.com/tsuru/tsuru/types/api"
	appTypes "github.com/tsuru/tsuru/types/app"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
)

func appTarget(appName string) event.Target {
	return event.Target{Type: event.TargetTypeApp, Value: appName}
}

func getAppFromContext(name string, r *http.Request) (app.App, error) {
	var err error
	a := context.GetApp(r)
	if a == nil {
		a, err = getApp(name)
		if err != nil {
			return app.App{}, err
		}
		context.SetApp(r, a)
	}
	return *a, nil
}

func getApp(name string) (*app.App, error) {
	a, err := app.GetByName(name)
	if err != nil {
		return nil, &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", name)}
	}
	return a, nil
}

// title: remove app
// path: /apps/{name}
// method: DELETE
// produce: application/x-json-stream
// responses:
//   200: App removed
//   401: Unauthorized
//   404: Not found
func appDelete(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	canDelete := permission.Check(t, permission.PermAppDelete,
		contextsForApp(&a)...,
	)
	if !canDelete {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(a.Name),
		Kind:       permission.PermAppDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt.SetLogWriter(writer)
	w.Header().Set("Content-Type", "application/x-json-stream")
	return app.Delete(&a, evt, requestIDHeader(r))
}

// miniApp is a minimal representation of the app, created to make appList
// faster and transmit less data.
type miniApp struct {
	Name      string               `json:"name"`
	Pool      string               `json:"pool"`
	TeamOwner string               `json:"teamowner"`
	Plan      appTypes.Plan        `json:"plan"`
	Units     []provision.Unit     `json:"units"`
	CName     []string             `json:"cname"`
	IP        string               `json:"ip"`
	Routers   []appTypes.AppRouter `json:"routers"`
	Lock      provision.AppLock    `json:"lock"`
	Tags      []string             `json:"tags"`
	Error     string               `json:"error,omitempty"`
}

func minifyApp(app app.App, unitData app.AppUnitsResponse) (miniApp, error) {
	var errorStr string
	if unitData.Err != nil {
		errorStr = unitData.Err.Error()
	}
	if unitData.Units == nil {
		unitData.Units = []provision.Unit{}
	}
	ma := miniApp{
		Name:      app.Name,
		Pool:      app.Pool,
		Plan:      app.Plan,
		TeamOwner: app.TeamOwner,
		Units:     unitData.Units,
		CName:     app.CName,
		Routers:   app.Routers,
		Lock:      &app.Lock,
		Tags:      app.Tags,
		Error:     errorStr,
	}
	if len(ma.Routers) > 0 {
		ma.IP = ma.Routers[0].Address
	}
	return ma, nil
}

func appFilterByContext(contexts []permTypes.PermissionContext, filter *app.Filter) *app.Filter {
	if filter == nil {
		filter = &app.Filter{}
	}
contextsLoop:
	for _, c := range contexts {
		switch c.CtxType {
		case permTypes.CtxGlobal:
			filter.Extra = nil
			break contextsLoop
		case permTypes.CtxTeam:
			filter.ExtraIn("teams", c.Value)
		case permTypes.CtxApp:
			filter.ExtraIn("name", c.Value)
		case permTypes.CtxPool:
			filter.ExtraIn("pool", c.Value)
		}
	}
	return filter
}

// title: app list
// path: /apps
// method: GET
// produce: application/json
// responses:
//   200: List apps
//   204: No content
//   401: Unauthorized
func appList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	filter := &app.Filter{}
	if name := r.URL.Query().Get("name"); name != "" {
		filter.NameMatches = name
	}
	if platform := r.URL.Query().Get("platform"); platform != "" {
		filter.Platform = platform
	}
	if teamOwner := r.URL.Query().Get("teamOwner"); teamOwner != "" {
		filter.TeamOwner = teamOwner
	}
	if owner := r.URL.Query().Get("owner"); owner != "" {
		filter.UserOwner = owner
	}
	if pool := r.URL.Query().Get("pool"); pool != "" {
		filter.Pool = pool
	}
	locked, _ := strconv.ParseBool(r.URL.Query().Get("locked"))
	if locked {
		filter.Locked = true
	}
	if status, ok := r.URL.Query()["status"]; ok {
		filter.Statuses = status
	}
	if tags, ok := r.URL.Query()["tag"]; ok {
		filter.Tags = tags
	}
	contexts := permission.ContextsForPermission(t, permission.PermAppRead)
	if len(contexts) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	apps, err := app.List(appFilterByContext(contexts, filter))
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	simple, _ := strconv.ParseBool(r.URL.Query().Get("simplified"))
	w.Header().Set("Content-Type", "application/json")
	miniApps := make([]miniApp, len(apps))
	if simple {
		for i, ap := range apps {
			ur := app.AppUnitsResponse{Units: nil, Err: nil}
			miniApps[i], err = minifyApp(ap, ur)
			if err != nil {
				return err
			}
		}
		return json.NewEncoder(w).Encode(miniApps)
	}
	appUnits, err := app.Units(apps)
	if err != nil {
		return err
	}
	for i, app := range apps {
		miniApps[i], err = minifyApp(app, appUnits[app.Name])
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(w).Encode(miniApps)
}

// title: app info
// path: /apps/{name}
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: Not found
func appInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	canRead := permission.Check(t, permission.PermAppRead,
		contextsForApp(&a)...,
	)
	if !canRead {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(&a)
}

type inputApp struct {
	TeamOwner   string
	Platform    string
	Plan        string
	Name        string
	Description string
	Pool        string
	Router      string
	RouterOpts  map[string]string
	Tags        []string
}

func autoTeamOwner(t auth.Token, perm *permission.PermissionScheme) (string, error) {
	team, err := permission.TeamForPermission(t, perm)
	if err == nil {
		return team, nil
	}
	if err != permission.ErrTooManyTeams {
		return "", err
	}
	teams, listErr := servicemanager.Team.List()
	if listErr != nil {
		return "", listErr
	}
	if len(teams) != 1 {
		return "", err
	}
	return teams[0].Name, nil
}

// title: app create
// path: /apps
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   201: App created
//   400: Invalid data
//   401: Unauthorized
//   403: Quota exceeded
//   409: App already exists
func createApp(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var ia inputApp
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	dec.DecodeValues(&ia, r.Form)
	a := app.App{
		TeamOwner:   ia.TeamOwner,
		Platform:    ia.Platform,
		Plan:        appTypes.Plan{Name: ia.Plan},
		Name:        ia.Name,
		Description: ia.Description,
		Pool:        ia.Pool,
		RouterOpts:  ia.RouterOpts,
		Router:      ia.Router,
		Tags:        ia.Tags,
		Quota:       quota.UnlimitedQuota,
	}
	a.Tags = append(a.Tags, r.Form["tag"]...) // for compatibility
	if a.TeamOwner == "" {
		a.TeamOwner, err = autoTeamOwner(t, permission.PermAppCreate)
		if err != nil {
			return err
		}
	}
	canCreate := permission.Check(t, permission.PermAppCreate,
		permission.Context(permTypes.CtxTeam, a.TeamOwner),
	)
	if !canCreate {
		return permission.ErrUnauthorized
	}
	u, err := auth.ConvertNewUser(t.User())
	if err != nil {
		return err
	}
	if a.Platform != "" {
		repo, _ := image.SplitImageName(a.Platform)
		platform, errPlat := servicemanager.Platform.FindByName(repo)
		if errPlat != nil {
			return errPlat
		}
		if platform.Disabled {
			canUsePlat := permission.Check(t, permission.PermPlatformUpdate) ||
				permission.Check(t, permission.PermPlatformCreate)
			if !canUsePlat {
				return &errors.HTTP{Code: http.StatusBadRequest, Message: appTypes.ErrInvalidPlatform.Error()}
			}
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(a.Name),
		Kind:       permission.PermAppCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = app.CreateApp(&a, u)
	if err != nil {
		log.Errorf("Got error while creating app: %s", err)
		if _, ok := err.(appTypes.NoTeamsError); ok {
			return &errors.HTTP{
				Code:    http.StatusBadRequest,
				Message: "In order to create an app, you should be member of at least one team",
			}
		}
		if e, ok := err.(*appTypes.AppCreationError); ok {
			if e.Err == app.ErrAppAlreadyExists {
				return &errors.HTTP{Code: http.StatusConflict, Message: e.Error()}
			}
			if _, ok := e.Err.(*quota.QuotaExceededError); ok {
				return &errors.HTTP{
					Code:    http.StatusForbidden,
					Message: "Quota exceeded",
				}
			}
		}
		if err == appTypes.ErrInvalidPlatform {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
		return err
	}
	repo, err := repository.Manager().GetRepository(a.Name)
	if err != nil {
		return err
	}
	msg := map[string]interface{}{
		"status":         "success",
		"repository_url": repo.ReadWriteURL,
	}
	addrs, err := a.GetAddresses()
	if err != nil {
		return err
	}
	if len(addrs) > 0 {
		msg["ip"] = addrs[0]
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(jsonMsg)
	return nil
}

// title: app update
// path: /apps/{name}
// method: PUT
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: App updated
//   400: Invalid new pool
//   401: Unauthorized
//   404: Not found
func updateApp(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var ia inputApp
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	dec.DecodeValues(&ia, r.Form)
	imageReset, _ := strconv.ParseBool(r.FormValue("imageReset"))
	updateData := app.App{
		TeamOwner:      ia.TeamOwner,
		Plan:           appTypes.Plan{Name: ia.Plan},
		Pool:           ia.Pool,
		Description:    ia.Description,
		Router:         ia.Router,
		Tags:           ia.Tags,
		Platform:       r.FormValue("platform"),
		UpdatePlatform: imageReset,
		RouterOpts:     ia.RouterOpts,
	}
	updateData.Tags = append(updateData.Tags, r.Form["tag"]...) // for compatibility
	appName := r.URL.Query().Get(":appname")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	var wantedPerms []*permission.PermissionScheme
	if updateData.Router != "" || len(updateData.RouterOpts) > 0 {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "updating router was deprecated, please add the wanted router and remove the old one"}
	}
	if updateData.Description != "" {
		wantedPerms = append(wantedPerms, permission.PermAppUpdateDescription)
	}
	if len(updateData.Tags) > 0 {
		wantedPerms = append(wantedPerms, permission.PermAppUpdateTags)
	}
	if updateData.Plan.Name != "" {
		wantedPerms = append(wantedPerms, permission.PermAppUpdatePlan)
	}
	if updateData.Pool != "" {
		wantedPerms = append(wantedPerms, permission.PermAppUpdatePool)
	}
	if updateData.TeamOwner != "" {
		wantedPerms = append(wantedPerms, permission.PermAppUpdateTeamowner)
	}
	if updateData.Platform != "" {
		repo, _ := image.SplitImageName(updateData.Platform)
		platform, errPlat := servicemanager.Platform.FindByName(repo)
		if errPlat != nil {
			return errPlat
		}
		if platform.Disabled {
			canUsePlat := permission.Check(t, permission.PermPlatformUpdate) ||
				permission.Check(t, permission.PermPlatformCreate)
			if !canUsePlat {
				return &errors.HTTP{Code: http.StatusBadRequest, Message: appTypes.ErrInvalidPlatform.Error()}
			}
		}
		wantedPerms = append(wantedPerms, permission.PermAppUpdatePlatform)
		updateData.UpdatePlatform = true
	}
	if updateData.UpdatePlatform {
		wantedPerms = append(wantedPerms, permission.PermAppUpdateImageReset)
	}
	if len(wantedPerms) == 0 {
		msg := "Neither the description, tags, plan, pool, team owner or platform were set. You must define at least one."
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	for _, perm := range wantedPerms {
		allowed := permission.Check(t, perm,
			contextsForApp(&a)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	w.Header().Set("Content-Type", "application/x-json-stream")
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = a.Update(updateData, writer)
	if err == appTypes.ErrPlanNotFound {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if _, ok := err.(*router.ErrRouterNotFound); ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	return err
}

func numberOfUnits(r *http.Request) (uint, error) {
	unitsStr := r.FormValue("units")
	if unitsStr == "" {
		return 0, &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "You must provide the number of units.",
		}
	}
	n, err := strconv.ParseUint(unitsStr, 10, 32)
	if err != nil || n == 0 {
		return 0, &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid number of units: the number must be an integer greater than 0.",
		}
	}
	return uint(n), nil
}

// title: add units
// path: /apps/{name}/units
// method: PUT
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Units added
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func addUnits(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	n, err := numberOfUnits(r)
	if err != nil {
		return err
	}
	processName := r.FormValue("process")
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateUnitAdd,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateUnitAdd,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	return a.AddUnits(n, processName, writer)
}

// title: remove units
// path: /apps/{name}/units
// method: DELETE
// produce: application/x-json-stream
// responses:
//   200: Units removed
//   400: Invalid data
//   401: Unauthorized
//   403: Not enough reserved units
//   404: App not found
func removeUnits(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	n, err := numberOfUnits(r)
	if err != nil {
		return err
	}
	processName := r.FormValue("process")
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateUnitRemove,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateUnitRemove,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	return a.RemoveUnits(n, processName, writer)
}

// title: set unit status
// path: /apps/{app}/units/{unit}
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App or unit not found
func setUnitStatus(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	unitName := r.URL.Query().Get(":unit")
	if unitName == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "missing unit",
		}
	}
	postStatus := r.FormValue("status")
	status, err := provision.ParseStatus(postStatus)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	appName := r.URL.Query().Get(":app")
	a, err := app.GetByName(appName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	allowed := permission.Check(t, permission.PermAppUpdateUnitStatus,
		contextsForApp(a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	err = a.SetUnitStatus(unitName, status)
	if _, ok := err.(*provision.UnitNotFoundError); ok {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return err
}

// title: set node status
// path: /node/status
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App or unit not found
func setNodeStatus(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if t.GetAppName() != app.InternalAppName {
		return &errors.HTTP{Code: http.StatusForbidden, Message: "this token is not allowed to execute this action"}
	}
	err := r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var hostInput provision.NodeStatusData
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&hostInput, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	result, err := app.UpdateNodeStatus(hostInput)
	if err != nil {
		if err == provision.ErrNodeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(result)
}

// title: grant access to app
// path: /apps/{app}/teams/{team}
// method: PUT
// responses:
//   200: Access granted
//   401: Unauthorized
//   404: App or team not found
//   409: Grant already exists
func grantAppAccess(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	appName := r.URL.Query().Get(":app")
	teamName := r.URL.Query().Get(":team")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateGrant,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateGrant,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	team, err := servicemanager.Team.FindByName(teamName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "Team not found"}
	}
	err = a.Grant(team)
	if err == app.ErrAlreadyHaveAccess {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	return err
}

// title: revoke access to app
// path: /apps/{app}/teams/{team}
// method: DELETE
// responses:
//   200: Access revoked
//   401: Unauthorized
//   403: Forbidden
//   404: App or team not found
func revokeAppAccess(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	appName := r.URL.Query().Get(":app")
	teamName := r.URL.Query().Get(":team")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateRevoke,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateRevoke,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	team, err := servicemanager.Team.FindByName(teamName)
	if err != nil || team == nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "Team not found"}
	}
	if len(a.Teams) == 1 {
		msg := "You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned"
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	err = a.Revoke(team)
	switch err {
	case app.ErrNoAccess:
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	case app.ErrCannotOrphanApp:
		return &errors.HTTP{Code: http.StatusForbidden, Message: err.Error()}
	default:
		return err
	}
}

// title: run commands
// path: /apps/{app}/run
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// method: POST
// responses:
//   200: Ok
//   401: Unauthorized
//   404: App not found
func runCommand(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	msg := "You must provide the command to run"
	command := r.FormValue("command")
	if len(command) < 1 {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	appName := r.URL.Query().Get(":app")
	once := r.FormValue("once")
	isolated := r.FormValue("isolated")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppRun,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppRun,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	onceBool, _ := strconv.ParseBool(once)
	isolatedBool, _ := strconv.ParseBool(isolated)
	args := provision.RunArgs{Once: onceBool, Isolated: isolatedBool}
	return a.Run(command, writer, args)
}

// title: get envs
// path: /apps/{app}/env
// method: GET
// produce: application/x-json-stream
// responses:
//   200: OK
//   401: Unauthorized
//   404: App not found
func getEnv(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var variables []string
	if envs, ok := r.URL.Query()["env"]; ok {
		variables = envs
	}
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	if !t.IsAppToken() {
		allowed := permission.Check(t, permission.PermAppReadEnv,
			contextsForApp(&a)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	return writeEnvVars(w, &a, variables...)
}

func writeEnvVars(w http.ResponseWriter, a *app.App, variables ...string) error {
	var result []bind.EnvVar
	w.Header().Set("Content-Type", "application/json")
	if len(variables) > 0 {
		for _, variable := range variables {
			if v, ok := a.Env[variable]; ok {
				result = append(result, v)
			}
		}
	} else {
		for _, v := range a.Envs() {
			result = append(result, v)
		}
	}
	return json.NewEncoder(w).Encode(result)
}

// title: set envs
// path: /apps/{app}/env
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Envs updated
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func setEnv(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var e apiTypes.Envs
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&e, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if len(e.Envs) == 0 {
		msg := "You must provide the list of environment variables"
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateEnvSet,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	if e.Private {
		for i := 0; i < len(e.Envs); i++ {
			r.Form.Set(fmt.Sprintf("Envs.%d.Value", i), "*****")
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateEnvSet,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	envs := map[string]string{}
	variables := []bind.EnvVar{}
	for _, v := range e.Envs {
		envs[v.Name] = v.Value
		variables = append(variables, bind.EnvVar{Name: v.Name, Value: v.Value, Public: !e.Private})
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = a.SetEnvs(bind.SetEnvArgs{
		Envs:          variables,
		ShouldRestart: !e.NoRestart,
		Writer:        writer,
	})
	if v, ok := err.(*errors.ValidationError); ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: v.Message}
	}
	return err
}

// title: unset envs
// path: /apps/{app}/env
// method: DELETE
// produce: application/x-json-stream
// responses:
//   200: Envs removed
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func unsetEnv(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	msg := "You must provide the list of environment variables."
	if r.FormValue("env") == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	var variables []string
	if envs, ok := r.Form["env"]; ok {
		variables = envs
	} else {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateEnvUnset,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateEnvUnset,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	noRestart, _ := strconv.ParseBool(r.FormValue("noRestart"))
	return a.UnsetEnvs(bind.UnsetEnvArgs{
		VariableNames: variables,
		ShouldRestart: !noRestart,
		Writer:        writer,
	})
}

// title: set cname
// path: /apps/{app}/cname
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func setCName(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	cNameMsg := "You must provide the cname."
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: cNameMsg}
	}
	cnames := r.Form["cname"]
	if len(cnames) == 0 {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: cNameMsg}
	}
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateCnameAdd,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateCnameAdd,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	if err = a.AddCName(cnames...); err == nil {
		return nil
	}
	if err.Error() == "Invalid cname" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	return err
}

// title: unset cname
// path: /apps/{app}/cname
// method: DELETE
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func unsetCName(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	cnames := r.Form["cname"]
	if len(cnames) == 0 {
		msg := "You must provide the cname."
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateCnameRemove,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateCnameRemove,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	if err = a.RemoveCName(cnames...); err == nil {
		return nil
	}
	if err.Error() == "Invalid cname" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	return err
}

// title: app log
// path: /apps/{app}/log
// method: GET
// produce: application/x-json-stream
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func appLog(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var err error
	var lines int
	if l := r.URL.Query().Get("lines"); l != "" {
		lines, err = strconv.Atoi(l)
		if err != nil {
			msg := `Parameter "lines" must be an integer.`
			return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
		}
	} else {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: `Parameter "lines" is mandatory.`}
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	source := r.URL.Query().Get("source")
	unit := r.URL.Query().Get("unit")
	follow := r.URL.Query().Get("follow")
	appName := r.URL.Query().Get(":app")
	filterLog := app.Applog{Source: source, Unit: unit}
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppReadLog,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	logs, err := a.LastLogs(lines, filterLog)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	err = encoder.Encode(logs)
	if err != nil {
		return err
	}
	if follow != "1" {
		return nil
	}
	closeChan := r.Context().Done()
	l, err := app.NewLogListener(&a, filterLog)
	if err != nil {
		return err
	}
	logTracker.add(l)
	defer func() {
		logTracker.remove(l)
		l.Close()
	}()
	logChan := l.ListenChan()
	for {
		var logMsg app.Applog
		var chOpen bool
		select {
		case <-closeChan:
			return nil
		case logMsg, chOpen = <-logChan:
		}
		if !chOpen {
			return nil
		}
		err := encoder.Encode([]app.Applog{logMsg})
		if err != nil {
			break
		}
	}
	return nil
}

func getServiceInstance(serviceName, instanceName, appName string) (*service.ServiceInstance, *app.App, error) {
	var app app.App
	conn, err := db.Conn()
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()
	instance, err := getServiceInstanceOrError(serviceName, instanceName)
	if err != nil {
		return nil, nil, err
	}
	err = conn.Apps().Find(bson.M{"name": appName}).One(&app)
	if err != nil {
		err = &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", appName)}
		return nil, nil, err
	}
	return instance, &app, nil
}

// title: bind service instance
// path: /services/{service}/instances/{instance}/{app}
// method: PUT
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func bindServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	instanceName := r.URL.Query().Get(":instance")
	appName := r.URL.Query().Get(":app")
	serviceName := r.URL.Query().Get(":service")
	req := struct {
		NoRestart  bool
		Parameters service.BindAppParameters
	}{}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	r.ParseForm()
	dec.DecodeValues(&req, r.Form)
	instance, a, err := getServiceInstance(serviceName, instanceName, appName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateBind,
		append(permission.Contexts(permTypes.CtxTeam, instance.Teams),
			permission.Context(permTypes.CtxServiceInstance, instance.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	allowed = permission.Check(t, permission.PermAppUpdateBind,
		contextsForApp(a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	err = a.ValidateService(serviceName)
	if err != nil {
		if err == pool.ErrPoolHasNoService {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
		return err
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateBind,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = instance.BindApp(a, req.Parameters, !req.NoRestart, writer, evt, requestIDHeader(r))
	if err != nil {
		status, errStatus := instance.Status(requestIDHeader(r))
		if errStatus != nil {
			return fmt.Errorf("%v (failed to retrieve instance status: %v)", err, errStatus)
		}
		return fmt.Errorf("%v (%q is %v)", err, instanceName, status)
	}
	fmt.Fprintf(writer, "\nInstance %q is now bound to the app %q.\n", instanceName, appName)
	envs := a.InstanceEnvs(serviceName, instanceName)
	if len(envs) > 0 {
		fmt.Fprintf(writer, "The following environment variables are available for use in your app:\n\n")
		for k := range envs {
			fmt.Fprintf(writer, "- %s\n", k)
		}
		fmt.Fprintf(writer, "- %s\n", app.TsuruServicesEnvVar)
	}
	return nil
}

// title: unbind service instance
// path: /services/{service}/instances/{instance}/{app}
// method: DELETE
// produce: application/x-json-stream
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func unbindServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName, appName, serviceName := r.URL.Query().Get(":instance"), r.URL.Query().Get(":app"),
		r.URL.Query().Get(":service")
	noRestart, _ := strconv.ParseBool(r.FormValue("noRestart"))
	force, _ := strconv.ParseBool(r.FormValue("force"))
	instance, a, err := getServiceInstance(serviceName, instanceName, appName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateUnbind,
		append(permission.Contexts(permTypes.CtxTeam, instance.Teams),
			permission.Context(permTypes.CtxServiceInstance, instance.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	allowed = permission.Check(t, permission.PermAppUpdateUnbind,
		contextsForApp(a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	if force {
		s, errGet := service.Get(instance.ServiceName)
		if errGet != nil {
			return errGet
		}
		allowed = permission.Check(t, permission.PermServiceUpdate,
			contextsForServiceProvision(&s)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateUnbind,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt.SetLogWriter(writer)
	err = instance.UnbindApp(service.UnbindAppArgs{
		App:         a,
		Restart:     !noRestart,
		ForceRemove: force,
		Event:       evt,
		RequestID:   requestIDHeader(r),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(evt, "\nInstance %q is not bound to the app %q anymore.\n", instanceName, appName)
	return nil
}

// title: app restart
// path: /apps/{app}/restart
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Ok
//   401: Unauthorized
//   404: App not found
func restart(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	process := r.FormValue("process")
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateRestart,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateRestart,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	return a.Restart(process, writer)
}

// title: app sleep
// path: /apps/{app}/sleep
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func sleep(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	process := r.FormValue("process")
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	proxy := r.FormValue("proxy")
	if proxy == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Empty proxy URL"}
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		log.Errorf("Invalid url for proxy param: %v", proxy)
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateSleep,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateSleep,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	return a.Sleep(writer, process, proxyURL)
}

// title: app log
// path: /apps/{app}/log
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func addLog(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	a, err := app.GetByName(r.URL.Query().Get(":app"))
	if err != nil {
		return err
	}
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if t.GetAppName() != app.InternalAppName {
		allowed := permission.Check(t, permission.PermAppUpdateLog,
			contextsForApp(a)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	logs := r.Form["message"]
	source := r.FormValue("source")
	if source == "" {
		source = "app"
	}
	unit := r.FormValue("unit")
	for _, log := range logs {
		err := a.Log(log, source, unit)
		if err != nil {
			return err
		}
	}
	return nil
}

// title: app swap
// path: /swap
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
//   409: App locked
//   412: Number of units or platform don't match
func swap(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	app1Name := r.FormValue("app1")
	app2Name := r.FormValue("app2")
	forceSwap := r.FormValue("force")
	cnameOnly, _ := strconv.ParseBool(r.FormValue("cnameOnly"))
	if forceSwap == "" {
		forceSwap = "false"
	}
	locked1, err := app.AcquireApplicationLockWait(app1Name, t.GetUserName(), "/swap", lockWaitDuration)
	if err != nil {
		return err
	}
	defer app.ReleaseApplicationLock(app1Name)
	locked2, err := app.AcquireApplicationLockWait(app2Name, t.GetUserName(), "/swap", lockWaitDuration)
	if err != nil {
		return err
	}
	defer app.ReleaseApplicationLock(app2Name)
	app1, err := getApp(app1Name)
	if err != nil {
		return err
	}
	if !locked1 {
		return &errors.HTTP{Code: http.StatusConflict, Message: fmt.Sprintf("%s: %s", app1.Name, &app1.Lock)}
	}
	app2, err := getApp(app2Name)
	if err != nil {
		return err
	}
	if !locked2 {
		return &errors.HTTP{Code: http.StatusConflict, Message: fmt.Sprintf("%s: %s", app2.Name, &app2.Lock)}
	}
	allowed1 := permission.Check(t, permission.PermAppUpdateSwap,
		contextsForApp(app1)...,
	)
	allowed2 := permission.Check(t, permission.PermAppUpdateSwap,
		contextsForApp(app2)...,
	)
	if !allowed1 || !allowed2 {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target: appTarget(app1Name),
		ExtraTargets: []event.ExtraTarget{
			{Target: appTarget(app2Name), Lock: true},
		},
		Kind:       permission.PermAppUpdateSwap,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(app1)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	// compare apps by platform type and number of units
	if forceSwap == "false" {
		if app1.Platform != app2.Platform {
			return &errors.HTTP{
				Code:    http.StatusPreconditionFailed,
				Message: "platforms don't match",
			}
		}
		app1Units, err := app1.Units()
		if err != nil {
			return err
		}
		app2Units, err := app2.Units()
		if err != nil {
			return err
		}
		if len(app1Units) != len(app2Units) {
			return &errors.HTTP{
				Code:    http.StatusPreconditionFailed,
				Message: "number of units doesn't match",
			}
		}
	}
	return app.Swap(app1, app2, cnameOnly)
}

// title: app start
// path: /apps/{app}/start
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Ok
//   401: Unauthorized
//   404: App not found
func start(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	process := r.FormValue("process")
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateStart,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateStart,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	return a.Start(writer, process)
}

// title: app stop
// path: /apps/{app}/stop
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Ok
//   401: Unauthorized
//   404: App not found
func stop(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	process := r.FormValue("process")
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateStop,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateStop,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	return a.Stop(writer, process)
}

// title: app unlock
// path: /apps/{app}/lock
// method: DELETE
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
//   404: App not found
func forceDeleteLock(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppAdminUnlock,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppAdminUnlock,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	app.ReleaseApplicationLock(a.Name)
	return nil
}

func isDeployAgentUA(r *http.Request) bool {
	ua := strings.ToLower(r.UserAgent())
	return strings.HasPrefix(ua, "go-http-client") ||
		strings.HasPrefix(ua, "tsuru-deploy-agent")
}

// title: register unit
// path: /apps/{app}/units/register
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
//   404: App not found
func registerUnit(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get(":app")
	a, err := app.GetByName(appName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateUnitRegister,
		contextsForApp(a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	if isDeployAgentUA(r) && r.Header.Get("X-Agent-Version") == "" {
		// Filtering the user-agent is not pretty, but it's safer than doing
		// the header check for every request, otherwise calling directly the
		// API would always fail without this header that only makes sense to
		// the agent.
		msgError := fmt.Sprintf("Please contact admin. %s platform is using outdated deploy-agent version, minimum required version is 0.2.4", a.GetPlatform())
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msgError}
	}
	defer r.Body.Close()
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	val, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	hostname := val.Get("hostname")
	var customData map[string]interface{}
	rawCustomData := val.Get("customdata")
	if rawCustomData != "" {
		err = json.Unmarshal([]byte(rawCustomData), &customData)
		if err != nil {
			return err
		}
	}
	err = a.RegisterUnit(hostname, customData)
	if err != nil {
		if err, ok := err.(*provision.UnitNotFoundError); ok {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	return writeEnvVars(w, a)
}

// title: metric envs
// path: /apps/{app}/metric/envs
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
//   404: App not found
func appMetricEnvs(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppReadMetric,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	metricMap, err := a.MetricEnvs()
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(metricMap)
}

// title: rebuild routes
// path: /apps/{app}/routes
// method: POST
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
//   404: App not found
func appRebuildRoutes(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppAdminRoutes,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	dry, _ := strconv.ParseBool(r.FormValue("dry"))
	evt, err := event.New(&event.Opts{
		Target:     appTarget(a.Name),
		Kind:       permission.PermAppAdminRoutes,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	result := map[string]rebuild.RebuildRoutesResult{}
	defer func() { evt.DoneCustomData(err, result) }()
	w.Header().Set("Content-Type", "application/json")
	result, err = rebuild.RebuildRoutes(&a, dry)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(&result)
}

// title: set app certificate
// path: /apps/{app}/certificate
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func setCertificate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateCertificateSet,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	cname := r.FormValue("cname")
	certificate := r.FormValue("certificate")
	key := r.FormValue("key")
	if cname == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "You must provide a cname."}
	}
	r.Form.Del("key")
	evt, err := event.New(&event.Opts{
		Target:     appTarget(a.Name),
		Kind:       permission.PermAppUpdateCertificateSet,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = a.SetCertificate(cname, certificate, key)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	return nil
}

// title: unset app certificate
// path: /apps/{app}/certificate
// method: DELETE
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func unsetCertificate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateCertificateUnset,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	cname := r.FormValue("cname")
	if cname == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "You must provide a cname."}
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(a.Name),
		Kind:       permission.PermAppUpdateCertificateUnset,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = a.RemoveCertificate(cname)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	return nil
}

// title: list app certificates
// path: /apps/{app}/certificate
// method: GET
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   401: Unauthorized
//   404: App not found
func listCertificates(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppReadCertificate,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	result, err := a.GetCertificates()
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(&result)
}

func contextsForApp(a *app.App) []permTypes.PermissionContext {
	return append(permission.Contexts(permTypes.CtxTeam, a.Teams),
		permission.Context(permTypes.CtxApp, a.Name),
		permission.Context(permTypes.CtxPool, a.Pool),
	)
}
