// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/rec"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/mgo.v2/bson"
)

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

func appIsAvailable(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	a, err := app.GetByName(r.URL.Query().Get(":appname"))
	if err != nil {
		return err
	}
	if t.GetAppName() != app.InternalAppName {
		allowed := permission.Check(t, permission.PermAppUpdateUnitAdd,
			append(permission.Contexts(permission.CtxTeam, a.Teams),
				permission.Context(permission.CtxApp, a.Name),
				permission.Context(permission.CtxPool, a.Pool),
			)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	if !a.Available() {
		return fmt.Errorf("App must be available to receive pushs.")
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func appDelete(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	canDelete := permission.Check(t, permission.PermAppDelete,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !canDelete {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "app-delete", "app="+a.Name)
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = app.Delete(&a, writer)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
	}
	return nil
}

// miniApp is a minimal representation of the app, created to make appList
// faster and transmit less data.
type miniApp struct {
	Name  string            `json:"name"`
	Units []provision.Unit  `json:"units"`
	CName []string          `json:"cname"`
	Ip    string            `json:"ip"`
	Lock  provision.AppLock `json:"lock"`
}

func minifyApp(app app.App) (miniApp, error) {
	units, err := app.Units()
	if err != nil {
		return miniApp{}, err
	}
	return miniApp{
		Name:  app.GetName(),
		Units: units,
		CName: app.GetCname(),
		Ip:    app.GetIp(),
		Lock:  app.GetLock(),
	}, nil
}

func appFilterByContext(contexts []permission.PermissionContext, filter *app.Filter) *app.Filter {
	if filter == nil {
		filter = &app.Filter{}
	}
contextsLoop:
	for _, c := range contexts {
		switch c.CtxType {
		case permission.CtxGlobal:
			filter.Extra = nil
			break contextsLoop
		case permission.CtxTeam:
			filter.ExtraIn("teams", c.Value)
		case permission.CtxApp:
			filter.ExtraIn("name", c.Value)
		case permission.CtxPool:
			filter.ExtraIn("pool", c.Value)
		}
	}
	return filter
}

func appList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	name := r.URL.Query().Get("name")
	platform := r.URL.Query().Get("platform")
	teamOwner := r.URL.Query().Get("teamowner")
	owner := r.URL.Query().Get("owner")
	pool := r.URL.Query().Get("pool")
	description := r.URL.Query().Get("description")
	locked, _ := strconv.ParseBool(r.URL.Query().Get("locked"))
	status := r.URL.Query().Get("status")
	extra := make([]interface{}, 0, 1)
	filter := &app.Filter{}
	if name != "" {
		extra = append(extra, fmt.Sprintf("name=%s", name))
		filter.Name = name
	}
	if platform != "" {
		extra = append(extra, fmt.Sprintf("platform=%s", platform))
		filter.Platform = platform
	}
	if teamOwner != "" {
		extra = append(extra, fmt.Sprintf("teamowner=%s", teamOwner))
		filter.TeamOwner = teamOwner
	}
	if owner != "" {
		extra = append(extra, fmt.Sprintf("owner=%s", owner))
		filter.UserOwner = owner
	}
	if pool != "" {
		extra = append(extra, fmt.Sprintf("pool=%s", pool))
		filter.Pool = pool
	}
	if description != "" {
		extra = append(extra, fmt.Sprintf("description=%s", description))
	}
	if locked {
		extra = append(extra, fmt.Sprintf("locked=%v", locked))
		filter.Locked = true
	}
	if status != "" {
		extra = append(extra, fmt.Sprintf("status=%s", status))
		filter.Statuses = strings.Split(status, ",")
	}
	rec.Log(u.Email, "app-list", extra...)
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
	w.Header().Set("Content-Type", "application/json")
	miniApps := make([]miniApp, len(apps))
	for i, app := range apps {
		miniApps[i], err = minifyApp(app)
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(w).Encode(miniApps)
}

func appInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	canRead := permission.Check(t, permission.PermAppRead,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !canRead {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "app-info", "app="+a.Name)
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(&a)
}

func createApp(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var a app.App
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(body, &a); err != nil {
		return err
	}
	if a.TeamOwner == "" {
		a.TeamOwner, err = permission.TeamForPermission(t, permission.PermAppCreate)
		if err != nil {
			return err
		}
	}
	canCreate := permission.Check(t, permission.PermAppCreate,
		permission.Context(permission.CtxTeam, a.TeamOwner),
	)
	if !canCreate {
		return permission.ErrUnauthorized
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	platform, err := app.GetPlatform(a.Platform)
	if err != nil {
		return err
	}
	if platform.Disabled {
		canUsePlat := permission.Check(t, permission.PermPlatformUpdate) ||
			permission.Check(t, permission.PermPlatformCreate)
		if !canUsePlat {
			return app.InvalidPlatformError
		}
	}
	rec.Log(u.Email, "create-app", "app="+a.Name, "platform="+a.Platform, "plan="+a.Plan.Name, "description="+a.Description)
	err = app.CreateApp(&a, u)
	if err != nil {
		log.Errorf("Got error while creating app: %s", err)
		if e, ok := err.(*errors.ValidationError); ok {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: e.Message}
		}
		if _, ok := err.(app.NoTeamsError); ok {
			return &errors.HTTP{
				Code:    http.StatusBadRequest,
				Message: "In order to create an app, you should be member of at least one team",
			}
		}
		if e, ok := err.(*app.AppCreationError); ok {
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
		if err == app.InvalidPlatformError {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	repo, _ := repository.Manager().GetRepository(a.Name)
	msg := map[string]string{
		"status":         "success",
		"repository_url": repo.ReadWriteURL,
		"ip":             a.Ip,
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s", jsonMsg)
	return nil
}

func updateApp(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var updateData app.App
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(body, &updateData); err != nil {
		return err
	}
	appName := r.URL.Query().Get(":appname")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	if updateData.Description == "" && updateData.Plan.Name == "" && updateData.Pool == "" && updateData.TeamOwner == "" {
		msg := "Neither the description, plan, pool or team owner were set. You must define at least one."
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	if updateData.Description != "" {
		allowed := permission.Check(t, permission.PermAppUpdateDescription,
			append(permission.Contexts(permission.CtxTeam, a.Teams),
				permission.Context(permission.CtxApp, a.Name),
				permission.Context(permission.CtxPool, a.Pool),
			)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}

	}
	if updateData.Plan.Name != "" {
		allowed := permission.Check(t, permission.PermAppUpdatePlan,
			append(permission.Contexts(permission.CtxTeam, a.Teams),
				permission.Context(permission.CtxApp, a.Name),
				permission.Context(permission.CtxPool, a.Pool),
			)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	if updateData.Pool != "" {
		allowed := permission.Check(t, permission.PermAppUpdatePool,
			append(permission.Contexts(permission.CtxTeam, a.Teams),
				permission.Context(permission.CtxApp, a.Name),
				permission.Context(permission.CtxPool, a.Pool),
			)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	if updateData.TeamOwner != "" {
		allowed := permission.Check(t, permission.PermAppUpdateTeamowner,
			append(permission.Contexts(permission.CtxTeam, a.Teams),
				permission.Context(permission.CtxApp, a.Name),
				permission.Context(permission.CtxPool, a.Pool),
			)...,
		)
		if !allowed {
			return &errors.HTTP{
				Code:    http.StatusForbidden,
				Message: permission.ErrUnauthorized.Error(),
			}
		}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = a.Update(updateData, writer)
	if err == app.ErrPlanNotFound {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
		return err
	}
	rec.Log(u.Email, "update-app", "app="+appName, "description="+updateData.Description, "pool="+updateData.Pool)
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

func addUnits(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	n, err := numberOfUnits(r)
	if err != nil {
		return err
	}
	processName := r.FormValue("process")
	appName := r.URL.Query().Get(":app")
	u, err := t.User()
	if err != nil {
		return err
	}
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateUnitAdd,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "add-units", "app="+appName, fmt.Sprintf("units=%d", n))
	w.Header().Set("Content-Type", "application/json")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = a.AddUnits(n, processName, writer)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
		return nil
	}
	return nil
}

func removeUnits(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	n, err := numberOfUnits(r)
	if err != nil {
		return err
	}
	u, err := t.User()
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
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "remove-units", "app="+appName, fmt.Sprintf("units=%d", n))
	w.Header().Set("Content-Type", "application/json")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = a.RemoveUnits(uint(n), processName, writer)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
		return nil
	}
	return nil
}

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
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	err = a.SetUnitStatus(unitName, status)
	if _, ok := err.(*provision.UnitNotFoundError); ok {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if err == nil {
		w.WriteHeader(http.StatusOK)
	}
	return err
}

func setUnitsStatus(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if t.GetAppName() != app.InternalAppName {
		return &errors.HTTP{Code: http.StatusForbidden, Message: "this token is not allowed to execute this action"}
	}
	defer r.Body.Close()
	var input []app.UpdateUnitsData
	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	result, err := app.UpdateUnitsStatus(input)
	if err != nil {
		return err
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(result)
}

func grantAppAccess(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	teamName := r.URL.Query().Get(":team")
	team := new(auth.Team)
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateGrant,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "grant-app-access", "app="+appName, "team="+teamName)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Teams().Find(bson.M{"_id": teamName}).One(team)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "Team not found"}
	}
	err = a.Grant(team)
	if err == app.ErrAlreadyHaveAccess {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	return err
}

func revokeAppAccess(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	teamName := r.URL.Query().Get(":team")
	rec.Log(u.Email, "revoke-app-access", "app="+appName, "team="+teamName)
	team := new(auth.Team)
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateRevoke,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Teams().Find(bson.M{"_id": teamName}).One(team)
	if err != nil {
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

func runCommand(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	w.Header().Set("Content-Type", "text")
	msg := "You must provide the command to run"
	if r.Body == nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	defer r.Body.Close()
	c, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(c) < 1 {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	once := r.URL.Query().Get("once")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppRun,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "run-command", "app="+appName, "command="+string(c))
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = a.Run(string(c), writer, once == "true")
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
		return err
	}
	return nil
}

func getEnv(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var variables []string
	if r.Body != nil {
		defer r.Body.Close()
		err := json.NewDecoder(r.Body).Decode(&variables)
		if err != nil && err != io.EOF {
			return err
		}
	}
	appName := r.URL.Query().Get(":app")
	var u *auth.User
	var err error
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	if !t.IsAppToken() {
		u, err = t.User()
		if err != nil {
			return err
		}
		rec.Log(u.Email, "get-env", "app="+appName, fmt.Sprintf("envs=%s", variables))
		allowed := permission.Check(t, permission.PermAppReadEnv,
			append(permission.Contexts(permission.CtxTeam, a.Teams),
				permission.Context(permission.CtxApp, a.Name),
				permission.Context(permission.CtxPool, a.Pool),
			)...,
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
		for _, v := range a.Env {
			result = append(result, v)
		}
	}
	return json.NewEncoder(w).Encode(result)
}

func setEnv(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	msg := "You must provide the environment variables in a JSON object"
	if r.Body == nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	var variables map[string]string
	err := json.NewDecoder(r.Body).Decode(&variables)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	noRestart, _ := strconv.ParseBool(r.URL.Query().Get("noRestart"))
	isPrivateEnv, _ := strconv.ParseBool(r.URL.Query().Get("private"))
	extra := fmt.Sprintf("private=%t", isPrivateEnv)
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateEnvSet,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "set-env", "app="+appName, variables, extra)
	envs := make([]bind.EnvVar, 0, len(variables))
	for k, v := range variables {
		envs = append(envs, bind.EnvVar{Name: k, Value: v, Public: !isPrivateEnv})
	}
	w.Header().Set("Content-Type", "application/json")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = a.SetEnvs(
		bind.SetEnvApp{
			Envs:          envs,
			PublicOnly:    true,
			ShouldRestart: !noRestart,
		}, writer)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
		return nil
	}
	return nil
}

func unsetEnv(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	msg := "You must provide the list of environment variables, in JSON format"
	if r.Body == nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	var variables []string
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(&variables)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	if len(variables) == 0 {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	appName := r.URL.Query().Get(":app")
	u, err := t.User()
	if err != nil {
		return err
	}
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateEnvUnset,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "unset-env", "app="+appName, fmt.Sprintf("envs=%s", variables))
	w.Header().Set("Content-Type", "application/json")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	noRestart, _ := strconv.ParseBool(r.URL.Query().Get("noRestart"))
	err = a.UnsetEnvs(
		bind.UnsetEnvApp{
			VariableNames: variables,
			PublicOnly:    true,
			ShouldRestart: !noRestart,
		}, writer)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
		return nil
	}
	return nil
}

func setCName(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	msg := "You must provide the cname."
	if r.Body == nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	var v map[string][]string
	err := json.NewDecoder(r.Body).Decode(&v)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Invalid JSON in request body."}
	}
	if _, ok := v["cname"]; !ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	rawCName := strings.Join(v["cname"], ", ")
	rec.Log(u.Email, "add-cname", "app="+appName, "cname="+rawCName)
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateCnameAdd,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	if err = a.AddCName(v["cname"]...); err == nil {
		return nil
	}
	if err.Error() == "Invalid cname" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	return err
}

func unsetCName(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	msg := "You must provide the cname."
	if r.Body == nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	var v map[string][]string
	err := json.NewDecoder(r.Body).Decode(&v)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Invalid JSON in request body."}
	}
	if _, ok := v["cname"]; !ok {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	rawCName := strings.Join(v["cname"], ", ")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateCnameRemove,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "remove-cname", "app="+appName, "cnames="+rawCName)
	if err = a.RemoveCName(v["cname"]...); err == nil {
		return nil
	}
	if err.Error() == "Invalid cname" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	return err
}

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
	w.Header().Set("Content-Type", "application/json")
	source := r.URL.Query().Get("source")
	unit := r.URL.Query().Get("unit")
	follow := r.URL.Query().Get("follow")
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	extra := []interface{}{
		"app=" + appName,
		fmt.Sprintf("lines=%d", lines),
	}
	if source != "" {
		extra = append(extra, "source="+source)
	}
	if follow == "1" {
		extra = append(extra, "follow=1")
	}
	if unit != "" {
		extra = append(extra, "unit="+unit)
	}
	rec.Log(u.Email, "app-log", extra...)
	filterLog := app.Applog{Source: source, Unit: unit}
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppReadLog,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
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
	var closeChan <-chan bool
	if notifier, ok := w.(http.CloseNotifier); ok {
		closeChan = notifier.CloseNotify()
	} else {
		closeChan = make(chan bool)
	}
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
		select {
		case <-closeChan:
			return nil
		case logMsg = <-logChan:
		}
		if logMsg == (app.Applog{}) {
			break
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

func bindServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName, appName, serviceName := r.URL.Query().Get(":instance"), r.URL.Query().Get(":app"),
		r.URL.Query().Get(":service")
	noRestart, _ := strconv.ParseBool(r.URL.Query().Get("noRestart"))
	instance, a, err := getServiceInstance(serviceName, instanceName, appName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateBind,
		append(permission.Contexts(permission.CtxTeam, instance.Teams),
			permission.Context(permission.CtxServiceInstance, instance.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	allowed = permission.Check(t, permission.PermAppUpdateBind,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(t.GetUserName(), "bind-app", "instance="+instanceName, "app="+appName)
	w.Header().Set("Content-Type", "application/json")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = instance.BindApp(a, !noRestart, writer)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
		return nil
	}
	fmt.Fprintf(writer, "\nInstance %q is now bound to the app %q.\n", instanceName, appName)
	envs := a.InstanceEnv(instanceName)
	if len(envs) > 0 {
		fmt.Fprintf(writer, "The following environment variables are available for use in your app:\n\n")
		for k := range envs {
			fmt.Fprintf(writer, "- %s\n", k)
		}
		fmt.Fprintf(writer, "- %s\n", app.TsuruServicesEnvVar)
	}
	return nil
}

func unbindServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName, appName, serviceName := r.URL.Query().Get(":instance"), r.URL.Query().Get(":app"),
		r.URL.Query().Get(":service")
	noRestart, _ := strconv.ParseBool(r.URL.Query().Get("noRestart"))
	u, err := t.User()
	if err != nil {
		return err
	}
	instance, a, err := getServiceInstance(serviceName, instanceName, appName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermServiceInstanceUpdateUnbind,
		append(permission.Contexts(permission.CtxTeam, instance.Teams),
			permission.Context(permission.CtxServiceInstance, instance.Name),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	allowed = permission.Check(t, permission.PermAppUpdateUnbind,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "unbind-app", "instance="+instanceName, "app="+appName)
	w.Header().Set("Content-Type", "application/json")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = instance.UnbindApp(a, !noRestart, writer)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
		return nil
	}
	fmt.Fprintf(writer, "\nInstance %q is not bound to the app %q anymore.\n", instanceName, appName)
	return nil
}

func restart(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	process := r.URL.Query().Get("process")
	w.Header().Set("Content-Type", "text")
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateRestart,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "restart", "app="+appName)
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = a.Restart(process, writer)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
		return err
	}
	return nil
}

func sleep(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	process := r.URL.Query().Get("process")
	w.Header().Set("Content-Type", "text")
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	proxy := r.URL.Query().Get("proxy")
	if proxy == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Empty proxy URL"}
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		log.Errorf("Invalid url for proxy param: %v", proxy)
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateSleep,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "sleep", "app="+appName)
	return a.Sleep(w, process, proxyURL)
}

func addLog(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	queryValues := r.URL.Query()
	a, err := app.GetByName(queryValues.Get(":app"))
	if err != nil {
		return err
	}
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if t.GetAppName() != app.InternalAppName {
		allowed := permission.Check(t, permission.PermAppUpdateLog,
			append(permission.Contexts(permission.CtxTeam, a.Teams),
				permission.Context(permission.CtxApp, a.Name),
				permission.Context(permission.CtxPool, a.Pool),
			)...,
		)
		if !allowed {
			return permission.ErrUnauthorized
		}
	}
	var logs []string
	err = json.Unmarshal(body, &logs)
	source := queryValues.Get("source")
	if len(source) == 0 {
		source = "app"
	}
	unit := queryValues.Get("unit")
	for _, log := range logs {
		err := a.Log(log, source, unit)
		if err != nil {
			return err
		}
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func platformList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "platform-list")
	canUsePlat := permission.Check(t, permission.PermPlatformUpdate) ||
		permission.Check(t, permission.PermPlatformCreate)
	platforms, err := app.Platforms(!canUsePlat)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(platforms)
}

func swap(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	app1Name := r.URL.Query().Get("app1")
	app2Name := r.URL.Query().Get("app2")
	forceSwap := r.URL.Query().Get("force")
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
		append(permission.Contexts(permission.CtxTeam, app1.Teams),
			permission.Context(permission.CtxApp, app1.Name),
			permission.Context(permission.CtxPool, app1.Pool),
		)...,
	)
	allowed2 := permission.Check(t, permission.PermAppUpdateSwap,
		append(permission.Contexts(permission.CtxTeam, app2.Teams),
			permission.Context(permission.CtxApp, app2.Name),
			permission.Context(permission.CtxPool, app2.Pool),
		)...,
	)
	if !allowed1 || !allowed2 {
		return permission.ErrUnauthorized
	}
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
	rec.Log(u.Email, "swap", "app1="+app1Name, "app2="+app2Name)
	return app.Swap(app1, app2)
}

func start(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	w.Header().Set("Content-Type", "text")
	process := r.URL.Query().Get("process")
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	rec.Log(u.Email, "start", "app="+appName)
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateStart,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	return a.Start(w, process)
}

func stop(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	w.Header().Set("Content-Type", "text")
	process := r.URL.Query().Get("process")
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	rec.Log(u.Email, "stop", "app="+appName)
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateStop,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	return a.Stop(w, process)
}

func forceDeleteLock(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppAdminUnlock,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	app.ReleaseApplicationLock(a.Name)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func registerUnit(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get(":app")
	a, err := app.GetByName(appName)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateUnitRegister,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
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
		if _, ok := err.(*provision.UnitNotFoundError); ok {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	return writeEnvVars(w, a)
}

func appMetricEnvs(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppReadMetric,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "app-metric-envs", "app="+r.URL.Query().Get(":app"))
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(a.MetricEnvs())
}

func appRebuildRoutes(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppAdminRoutes,
		append(permission.Contexts(permission.CtxTeam, a.Teams),
			permission.Context(permission.CtxApp, a.Name),
			permission.Context(permission.CtxPool, a.Pool),
		)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	rec.Log(u.Email, "app-rebuild-routes", "app="+r.URL.Query().Get(":app"))
	w.Header().Set("Content-Type", "application/json")
	result, err := a.RebuildRoutes()
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(&result)
}
