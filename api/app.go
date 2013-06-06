// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/go-gandalfclient"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/quota"
	"github.com/globocom/tsuru/rec"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/service"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"net/http"
	"strconv"
)

func getApp(name string, u *auth.User) (app.App, error) {
	app := app.App{Name: name}
	err := app.Get()
	if err != nil {
		return app, &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", name)}
	}
	if u.IsAdmin() {
		return app, nil
	}
	if !auth.CheckUserAccess(app.Teams, u) {
		return app, &errors.HTTP{Code: http.StatusForbidden, Message: "User does not have access to this app"}
	}
	return app, nil
}

func cloneRepository(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	version := r.PostFormValue("version")
	if version == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Missing parameter version"}
	}
	w.Header().Set("Content-Type", "text")
	instance := &app.App{Name: r.URL.Query().Get(":appname")}
	err := instance.Get()
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", instance.Name)}
	}
	logger := app.LogWriter{App: instance, Writer: w}
	return app.Provisioner.Deploy(instance, version, &logger)
}

func appIsAvailable(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	app := app.App{Name: r.URL.Query().Get(":appname")}
	err := app.Get()
	if err != nil {
		return err
	}
	if !app.Available() {
		return fmt.Errorf("App must be available to receive pushs.")
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func appDelete(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "app-delete", r.URL.Query().Get(":app"))
	a, err := getApp(r.URL.Query().Get(":app"), u)
	if err != nil {
		return err
	}
	app.ForceDestroy(&a)
	fmt.Fprint(w, "success")
	return nil
}

func appList(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func appInfo(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func createApp(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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
	rec.Log(u.Email, "create-app", "name="+a.Name, "platform="+a.Platform)
	err = app.CreateApp(&a, u)
	if err != nil {
		log.Printf("Got error while creating app: %s", err)
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

func addUnits(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func removeUnits(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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
	return app.RemoveUnits(uint(n))
}

func grantAppAccess(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func revokeAppAccess(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func runCommand(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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
	rec.Log(u.Email, "run-command", "app="+appName, "command="+string(c))
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	return app.Run(string(c), w)
}

func getEnv(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	var variables []string
	if r.Body != nil {
		defer r.Body.Close()
		err := json.NewDecoder(r.Body).Decode(&variables)
		if err != nil && err != io.EOF {
			return err
		}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	rec.Log(u.Email, "get-env", "app="+appName, fmt.Sprintf("envs=%s", variables))
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	l := len(variables)
	if l == 0 {
		l = len(app.Env)
	}
	result := make(map[string]string, l)
	w.Header().Set("Content-Type", "application/json")
	if len(variables) > 0 {
		for _, variable := range variables {
			if v, ok := app.Env[variable]; ok {
				result[variable] = v.String()
			}
		}
	} else {
		for k, v := range app.Env {
			result[k] = v.String()
		}
	}
	return json.NewEncoder(w).Encode(result)
}

func setEnv(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func unsetEnv(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func setCName(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	msg := "You must provide the cname."
	if r.Body == nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	var v map[string]string
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
	rec.Log(u.Email, "set-cname", "app="+appName, "cname="+v["cname"])
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	if err = app.SetCName(v["cname"]); err == nil {
		return nil
	}
	if err.Error() == "Invalid cname" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	return err
}

func unsetCName(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func appLog(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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
	if r.URL.Query().Get("follow") == "1" {
		extra = append(extra, "follow=1")
	}
	rec.Log(u.Email, "app-log", extra...)
	a, err := getApp(appName, u)
	if err != nil {
		return err
	}
	logs, err := a.LastLogs(lines, source)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	err = encoder.Encode(logs)
	if err != nil {
		return err
	}
	// TODO(fss): write an automated test for this code.
	if r.URL.Query().Get("follow") == "1" {
		l := app.NewLogListener(&a)
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

func bindServiceInstance(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func unbindServiceInstance(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func restart(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func addLog(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	app := app.App{Name: r.URL.Query().Get(":app")}
	err := app.Get()
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
	for _, log := range logs {
		err := app.Log(log, "app")
		if err != nil {
			return err
		}
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func platformList(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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
