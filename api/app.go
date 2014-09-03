// Copyright 2014 tsuru authors. All rights reserved.
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

	"github.com/tsuru/config"
	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/rec"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/mgo.v2/bson"
)

func getApp(name string, u *auth.User) (app.App, error) {
	a, err := app.GetByName(name)
	if err != nil {
		return app.App{}, &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", name)}
	}
	if u == nil || u.IsAdmin() {
		return *a, nil
	}
	if !auth.CheckUserAccess(a.Teams, u) {
		return *a, &errors.HTTP{Code: http.StatusForbidden, Message: "User does not have access to this app"}
	}
	return *a, nil
}

func appIsAvailable(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	app, err := app.GetByName(r.URL.Query().Get(":appname"))
	if err != nil {
		return err
	}
	if !app.Available() {
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
	rec.Log(u.Email, "app-delete", r.URL.Query().Get(":app"))
	a, err := getApp(r.URL.Query().Get(":app"), u)
	if err != nil {
		return err
	}
	context.SetPreventUnlock(r)
	app.Delete(&a)
	fmt.Fprint(w, "success")
	return nil
}

func appList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "app-list")
	apps, err := app.List(u)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return json.NewEncoder(w).Encode(apps)
}

func appInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "app-info", r.URL.Query().Get(":app"))
	app, err := getApp(r.URL.Query().Get(":app"), u)
	if err != nil {
		return err
	}
	host, err := config.GetString("host")
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; profile="+host+"/schema/app")
	return json.NewEncoder(w).Encode(&app)
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
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "create-app", "name="+a.Name, "platform="+a.Platform, "memory="+strconv.Itoa(a.Memory), "swap="+strconv.Itoa(a.Swap))
	canSetMem, _ := config.GetBool("docker:allow-memory-set")
	if !canSetMem && (a.Memory > 0 || a.Swap > 0) {
		err := "Memory setting not allowed."
		log.Errorf("%s", err)
		return &errors.HTTP{Code: http.StatusForbidden, Message: err}
	}
	maxMem, _ := config.GetInt("docker:max-allowed-memory")
	if maxMem > 0 && a.Memory > maxMem {
		err := fmt.Sprintf("Invalid memory size. You cannot request more than %dMB.", maxMem)
		log.Errorf("%s", err)
		return &errors.HTTP{Code: http.StatusForbidden, Message: err}
	}
	maxSwap, _ := config.GetInt("docker:max-allowed-swap")
	if maxSwap > 0 && a.Swap > maxSwap {
		err := fmt.Sprintf("Invalid swap size. You cannot request more than %dMB.", maxSwap)
		log.Errorf("%s", err)
		return &errors.HTTP{Code: http.StatusForbidden, Message: err}
	}
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
		return err
	}
	msg := map[string]string{
		"status":         "success",
		"repository_url": repository.ReadWriteURL(a.Name),
		"ip":             a.Ip,
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s", jsonMsg)
	return nil
}

func numberOfUnits(r *http.Request) (uint, error) {
	missingMsg := "You must provide the number of units."
	if r.Body == nil {
		return 0, &errors.HTTP{Code: http.StatusBadRequest, Message: missingMsg}
	}
	defer r.Body.Close()
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return 0, err
	}
	value := string(b)
	if value == "" {
		return 0, &errors.HTTP{Code: http.StatusBadRequest, Message: missingMsg}
	}
	n, err := strconv.ParseUint(value, 10, 32)
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
	appName := r.URL.Query().Get(":app")
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "add-units", "app="+appName, fmt.Sprintf("units=%d", n))
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	err = app.AddUnits(n)
	if _, ok := err.(*quota.QuotaExceededError); ok {
		return &errors.HTTP{
			Code:    http.StatusForbidden,
			Message: err.Error(),
		}
	}
	return err
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
	appName := r.URL.Query().Get(":app")
	rec.Log(u.Email, "remove-units", "app="+appName, fmt.Sprintf("units=%d", n))
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	context.SetPreventUnlock(r)
	return app.RemoveUnits(uint(n))
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
	err = a.SetUnitStatus(unitName, status)
	if err == app.ErrUnitNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return err
}

func grantAppAccess(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	teamName := r.URL.Query().Get(":team")
	rec.Log(u.Email, "grant-app-access", "app="+appName, "team="+teamName)
	team := new(auth.Team)
	app, err := getApp(appName, u)
	if err != nil {
		return err
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
	err = app.Grant(team)
	if err != nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	err = conn.Apps().Update(bson.M{"name": app.Name}, app)
	if err != nil {
		return err
	}
	gURL := repository.ServerURL()
	gClient := gandalf.Client{Endpoint: gURL}
	if err := gClient.GrantAccess([]string{app.Name}, team.Users); err != nil {
		return fmt.Errorf("Failed to grant access in the git server: %s.", err)
	}
	return nil
}

func getEmailsForRevoking(app *app.App, t *auth.Team) []string {
	var i int
	teams := app.GetTeams()
	users := make([]string, len(t.Users))
	for _, email := range t.Users {
		found := false
		for _, team := range teams {
			for _, user := range team.Users {
				if user == email {
					found = true
					break
				}
			}
		}
		if !found {
			users[i] = email
			i++
		}
	}
	return users[:i]
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
	app, err := getApp(appName, u)
	if err != nil {
		return err
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
	if len(app.Teams) == 1 {
		msg := "You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned"
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	err = app.Revoke(team)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	err = conn.Apps().Update(bson.M{"name": app.Name}, app)
	if err != nil {
		return err
	}
	users := getEmailsForRevoking(&app, team)
	if len(users) > 0 {
		gURL := repository.ServerURL()
		if err := (&gandalf.Client{Endpoint: gURL}).RevokeAccess([]string{app.Name}, users); err != nil {
			return fmt.Errorf("Failed to revoke access in the git server: %s", err)
		}
	}
	return nil
}

func runCommand(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	w.Header().Set("Content-Type", "text")
	msg := "You must provide the command to run"
	if r.Body == nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
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
	rec.Log(u.Email, "run-command", "app="+appName, "command="+string(c))
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	err = app.Run(string(c), w, once == "true")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "\nOK!")
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
	if !t.IsAppToken() {
		u, err = t.User()
		if err != nil {
			return err
		}
		rec.Log(u.Email, "get-env", "app="+appName, fmt.Sprintf("envs=%s", variables))
	}
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	return writeEnvVars(w, &app, variables...)
}

func writeEnvVars(w http.ResponseWriter, a *app.App, variables ...string) error {
	var result []map[string]interface{}
	w.Header().Set("Content-Type", "application/json")
	if len(variables) > 0 {
		for _, variable := range variables {
			if v, ok := a.Env[variable]; ok {
				item := map[string]interface{}{
					"name":   v.Name,
					"value":  v.Value,
					"public": v.Public,
				}
				result = append(result, item)
			}
		}
	} else {
		for _, v := range a.Env {
			item := map[string]interface{}{
				"name":   v.Name,
				"value":  v.Value,
				"public": v.Public,
			}
			result = append(result, item)
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
	appName := r.URL.Query().Get(":app")
	rec.Log(u.Email, "set-env", "app="+appName, variables)
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	envs := make([]bind.EnvVar, 0, len(variables))
	for k, v := range variables {
		envs = append(envs, bind.EnvVar{Name: k, Value: v, Public: true})
	}
	return app.SetEnvs(envs, true)
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
	rec.Log(u.Email, "unset-env", "app="+appName, fmt.Sprintf("envs=%s", variables))
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	return app.UnsetEnvs(variables, true)
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
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	if err = app.AddCName(v["cname"]...); err == nil {
		return nil
	}
	if err.Error() == "Invalid cname" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	return err
}

func unsetCName(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	rec.Log(u.Email, "unset-cname", "app="+appName)
	return app.UnsetCName()
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
	a, err := getApp(appName, u)
	if err != nil {
		return err
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
	if follow == "1" {
		l, err := app.NewLogListener(&a, filterLog)
		if err != nil {
			return err
		}
		defer l.Close()
		for log := range l.C {
			err := encoder.Encode([]app.Applog{log})
			if err != nil {
				break
			}
		}
	}
	return nil
}

func getServiceInstance(instanceName, appName string, u *auth.User) (*service.ServiceInstance, *app.App, error) {
	var app app.App
	conn, err := db.Conn()
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()
	instance, err := getServiceInstanceOrError(instanceName, u)
	if err != nil {
		return nil, nil, err
	}
	err = conn.Apps().Find(bson.M{"name": appName}).One(&app)
	if err != nil {
		err = &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", appName)}
		return nil, nil, err
	}
	if !auth.CheckUserAccess(app.Teams, u) {
		err = &errors.HTTP{Code: http.StatusForbidden, Message: "This user does not have access to this app"}
		return nil, nil, err
	}
	return instance, &app, nil
}

func bindServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName, appName := r.URL.Query().Get(":instance"), r.URL.Query().Get(":app")
	u, err := t.User()
	if err != nil {
		return err
	}
	instance, a, err := getServiceInstance(instanceName, appName, u)
	if err != nil {
		return err
	}
	rec.Log(u.Email, "bind-app", "instance="+instanceName, "app="+appName)
	err = instance.BindApp(a)
	if err != nil {
		return err
	}
	var envs []string
	for k := range a.InstanceEnv(instanceName) {
		envs = append(envs, k)
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	return enc.Encode(envs)
}

func unbindServiceInstance(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	instanceName, appName := r.URL.Query().Get(":instance"), r.URL.Query().Get(":app")
	u, err := t.User()
	if err != nil {
		return err
	}
	instance, a, err := getServiceInstance(instanceName, appName, u)
	if err != nil {
		return err
	}
	rec.Log(u.Email, "unbind-app", "instance="+instanceName, "app="+appName)
	return instance.UnbindApp(a)
}

func restart(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	w.Header().Set("Content-Type", "text")
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	rec.Log(u.Email, "restart", appName)
	instance, err := getApp(appName, u)
	if err != nil {
		return err
	}
	return instance.Restart(w)
}

func addLog(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	queryValues := r.URL.Query()
	app, err := app.GetByName(queryValues.Get(":app"))
	if err != nil {
		return err
	}
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var logs []string
	err = json.Unmarshal(body, &logs)
	source := queryValues.Get("source")
	if len(source) == 0 {
		source = "app"
	}
	unit := queryValues.Get("unit")
	for _, log := range logs {
		err := app.Log(log, source, unit)
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
	platforms, err := app.Platforms()
	if err != nil {
		return err
	}
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
	locked1, err := app.AcquireApplicationLock(app1Name, t.GetUserName(), "/swap")
	if err != nil {
		return err
	}
	defer app.ReleaseApplicationLock(app1Name)
	locked2, err := app.AcquireApplicationLock(app2Name, t.GetUserName(), "/swap")
	if err != nil {
		return err
	}
	defer app.ReleaseApplicationLock(app2Name)
	app1, err := getApp(app1Name, u)
	if err != nil {
		return err
	}
	if !locked1 {
		return &errors.HTTP{Code: http.StatusConflict, Message: fmt.Sprintf("%s: %s", app1.Name, &app1.Lock)}
	}
	app2, err := getApp(app2Name, u)
	if err != nil {
		return err
	}
	if !locked2 {
		return &errors.HTTP{Code: http.StatusConflict, Message: fmt.Sprintf("%s: %s", app2.Name, &app2.Lock)}
	}
	// compare apps by platform type and number of units
	if forceSwap == "false" && ((len(app1.Units()) != len(app2.Units())) || (app1.Platform != app2.Platform)) {
		return app.ErrAppNotEqual
	}
	rec.Log(u.Email, "swap", app1Name, app2Name)
	return app.Swap(&app1, &app2)
}

func start(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	w.Header().Set("Content-Type", "text")
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	rec.Log(u.Email, "start", appName)
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	return app.Start(w)
}

func stop(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	w.Header().Set("Content-Type", "text")
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	rec.Log(u.Email, "stop", appName)
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	return app.Stop(w)
}

func forceDeleteLock(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get(":app")
	app.ReleaseApplicationLock(appName)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func registerUnit(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get(":app")
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	val, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	hostname := val.Get("hostname")
	a, err := app.GetByName(appName)
	if err != nil {
		return err
	}
	err = a.RegisterUnit(hostname)
	if err != nil {
		if err == app.ErrUnitNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	return writeEnvVars(w, a)
}
