// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/api/bind"
	"github.com/globocom/tsuru/api/service"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/repository"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"net/http"
	"regexp"
	"strings"
)

func sendProjectChangeToGitosis(kind int, team *auth.Team, app *App) {
	ch := repository.Change{
		Kind: kind,
		Args: map[string]string{"group": team.Name, "project": app.Name},
	}
	repository.Ag.Process(ch)
}

func getAppOrError(name string, u *auth.User) (App, error) {
	app := App{Name: name}
	err := app.Get()
	if err != nil {
		return app, &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
	}
	if !auth.CheckUserAccess(app.Teams, u) {
		return app, &errors.Http{Code: http.StatusForbidden, Message: "User does not have access to this app"}
	}
	return app, nil
}

func CloneRepositoryHandler(w http.ResponseWriter, r *http.Request) error {
	var write = func(w http.ResponseWriter, out []byte) error {
		n, err := w.Write(out)
		if err != nil {
			return err
		}
		if n != len(out) {
			return io.ErrShortWrite
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return nil
	}
	w.Header().Set("Content-Type", "text")
	err := write(w, []byte("\n ---> Tsuru receiving push\n"))
	if err != nil {
		return err
	}
	app := App{Name: r.URL.Query().Get(":name")}
	err = app.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
	}
	err = write(w, []byte("\n ---> Clonning your code in your machines\n"))
	if err != nil {
		return err
	}
	out, err := repository.CloneOrPull(app.unit()) // should iterate over the machines
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: string(out)}
	}
	err = write(w, out)
	if err != nil {
		return err
	}
	err = write(w, []byte("\n ---> Parsing app.conf\n"))
	if err != nil {
		return err
	}
	c, err := app.conf()
	if err != nil {
		return err
	}
	err = write(w, []byte("\n ---> Installing dependencies\n"))
	if err != nil {
		return err
	}
	writer := FilteredWriter{w}
	_, err = installDeps(&app, &writer, &writer)
	if err != nil {
		return err
	}
	err = write(w, []byte("\n ---> Running pre-restart\n"))
	if err != nil {
		return err
	}
	out, err = app.preRestart(c)
	if err != nil {
		return err
	}
	err = write(w, out)
	if err != nil {
		return err
	}
	out, err = restart(&app, &writer)
	if err != nil {
		write(w, out)
		return err
	}
	err = write(w, out)
	if err != nil {
		return err
	}
	err = write(w, []byte("\n ---> Running pos-restart\n"))
	if err != nil {
		return err
	}
	out, err = app.posRestart(c)
	err = write(w, out)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	return write(w, []byte("\n ---> Deploy done!\n\n"))
}

// AppIsAvaliableHandler verify if the app.unit().State() is
// started. If is started it returns 200 else returns 500 for
// status code.
func AppIsAvaliableHandler(w http.ResponseWriter, r *http.Request) error {
	app := App{Name: r.URL.Query().Get(":name")}
	err := app.Get()
	if err != nil {
		return err
	}
	if state := app.unit().State(); state != "started" {
		return fmt.Errorf("App must be started to receive pushs, but it is %s.", state)
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func AppDelete(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	app, err := getAppOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	err = app.destroy()
	if err != nil {
		return err
	}
	for _, t := range app.teams() {
		sendProjectChangeToGitosis(repository.RemoveProject, &t, &app)
	}
	fmt.Fprint(w, "success")
	return nil
}

func getTeamNames(u *auth.User) (names []string, err error) {
	var teams []auth.Team
	err = db.Session.Teams().Find(bson.M{"users": u.Email}).All(&teams)
	if err != nil {
		return
	}
	names = make([]string, len(teams))
	for i, team := range teams {
		names[i] = team.Name
	}
	return
}

func AppList(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	teams, err := getTeamNames(u)
	if err != nil {
		return err
	}
	var apps []App
	err = db.Session.Apps().Find(bson.M{"teams": bson.M{"$in": teams}}).All(&apps)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	b, err := json.Marshal(apps)
	if err != nil {
		return err
	}
	fmt.Fprint(w, string(b))
	return nil
}

func AppInfo(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	app, err := getAppOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	b, err := json.Marshal(&app)
	if err != nil {
		return err
	}
	fmt.Fprint(w, string(b))
	return nil
}

func createAppHelper(app *App, u *auth.User) ([]byte, error) {
	var teams []auth.Team
	err := db.Session.Teams().Find(bson.M{"users": u.Email}).All(&teams)
	if err != nil {
		return nil, err
	}
	if len(teams) < 1 {
		msg := "In order to create an app, you should be member of at least one team"
		return nil, &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	app.setTeams(teams)
	err = createApp(app)
	if err != nil {
		if strings.Contains(err.Error(), "key error") {
			msg := fmt.Sprintf(`There is already an app named "%s".`, app.Name)
			return nil, &errors.Http{Code: http.StatusConflict, Message: msg}
		}
		return nil, err
	}
	for _, t := range teams {
		sendProjectChangeToGitosis(repository.AddProject, &t, app)
	}
	msg := map[string]string{
		"status":         "success",
		"repository_url": repository.GetUrl(app.Name),
	}
	return json.Marshal(msg)
}

func CreateAppHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	var app App
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, &app)
	if err != nil {
		return err
	}
	jsonMsg, err := createAppHelper(&app, u)
	if err != nil {
		return err
	}

	fmt.Fprint(w, string(jsonMsg))
	return nil
}

func grantAccessToTeam(appName, teamName string, u *auth.User) error {
	t := new(auth.Team)
	app := &App{Name: appName}
	err := app.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
	}
	if !auth.CheckUserAccess(app.Teams, u) {
		return &errors.Http{Code: http.StatusUnauthorized, Message: "User unauthorized"}
	}
	err = db.Session.Teams().Find(bson.M{"_id": teamName}).One(t)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Team not found"}
	}
	err = app.grant(t)
	if err != nil {
		return &errors.Http{Code: http.StatusConflict, Message: err.Error()}
	}
	err = db.Session.Apps().Update(bson.M{"name": app.Name}, app)
	if err != nil {
		return err
	}
	sendProjectChangeToGitosis(repository.AddProject, t, app)
	return nil
}

func GrantAccessToTeamHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	appName := r.URL.Query().Get(":app")
	teamName := r.URL.Query().Get(":team")
	return grantAccessToTeam(appName, teamName, u)
}

func revokeAccessFromTeam(appName, teamName string, u *auth.User) error {
	t := new(auth.Team)
	app := &App{Name: appName}
	err := app.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
	}
	if !auth.CheckUserAccess(app.Teams, u) {
		return &errors.Http{Code: http.StatusUnauthorized, Message: "User unauthorized"}
	}
	err = db.Session.Teams().Find(bson.M{"_id": teamName}).One(t)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Team not found"}
	}
	if len(app.Teams) == 1 {
		msg := "You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned"
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	err = app.revoke(t)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: err.Error()}
	}
	err = db.Session.Apps().Update(bson.M{"name": app.Name}, app)
	if err != nil {
		return err
	}
	sendProjectChangeToGitosis(repository.RemoveProject, t, app)
	return nil
}

func RevokeAccessFromTeamHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	appName := r.URL.Query().Get(":app")
	teamName := r.URL.Query().Get(":team")
	return revokeAccessFromTeam(appName, teamName, u)
}

func RunCommand(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	w.Header().Set("Content-Type", "text")
	msg := "You must provide the command to run"
	if r.Body == nil {
		return &errors.Http{Code: http.StatusBadRequest, Message: msg}
	}
	c, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(c) < 1 {
		return &errors.Http{Code: http.StatusBadRequest, Message: msg}
	}
	appName := r.URL.Query().Get(":name")
	app, err := getAppOrError(appName, u)
	if err != nil {
		return err
	}
	writer := FilteredWriter{w}
	return app.run(string(c), &writer)
}

func GetEnv(w http.ResponseWriter, r *http.Request, u *auth.User) (err error) {
	var variable []byte
	if r.Body != nil {
		variable, err = ioutil.ReadAll(r.Body)
		if err != nil {
			return
		}
	}
	appName := r.URL.Query().Get(":name")
	app, err := getAppOrError(appName, u)
	if err != nil {
		return err
	}
	var write = func(v bind.EnvVar) error {
		_, err := fmt.Fprintf(w, "%s\n", &v)
		return err
	}
	if variables := strings.Fields(string(variable)); len(variables) > 0 {
		for _, variable := range variables {
			if v, ok := app.Env[variable]; ok {
				err = write(v)
				if err != nil {
					return
				}
			}
		}
	} else {
		for _, v := range app.Env {
			err = write(v)
			if err != nil {
				return
			}
		}
	}
	return nil
}

func setEnvsToApp(app *App, envs []bind.EnvVar, publicOnly bool) error {
	if len(envs) > 0 {
		for _, env := range envs {
			set := true
			if publicOnly {
				e, err := app.getEnv(env.Name)
				if err == nil && !e.Public {
					set = false
				}
			}
			if set {
				app.setEnv(env)
			}
		}
		if err := db.Session.Apps().Update(bson.M{"name": app.Name}, app); err != nil {
			return err
		}
		mess := message{
			app: app,
		}
		env <- mess
	}
	return nil
}

func SetEnv(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	msg := "You must provide the environment variables"
	if r.Body == nil {
		return &errors.Http{Code: http.StatusBadRequest, Message: msg}
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return &errors.Http{Code: http.StatusBadRequest, Message: msg}
	}
	appName := r.URL.Query().Get(":name")
	app, err := getAppOrError(appName, u)
	if err != nil {
		return err
	}
	regex, err := regexp.Compile(`(\w+=[^=]+)(\s|$)`)
	if err != nil {
		return err
	}
	variables := regex.FindAllStringSubmatch(string(body), -1)
	envs := make([]bind.EnvVar, len(variables))
	for i, v := range variables {
		parts := strings.Split(v[1], "=")
		envs[i] = bind.EnvVar{Name: parts[0], Value: parts[1], Public: true}
	}
	return setEnvsToApp(&app, envs, true)
}

func unsetEnvFromApp(app *App, variableNames []string, publicOnly bool) error {
	if len(variableNames) > 0 {
		for _, name := range variableNames {
			var unset bool
			e, err := app.getEnv(name)
			if !publicOnly || (err == nil && e.Public) {
				unset = true
			}
			if unset {
				delete(app.Env, name)
			}
		}
		if err := db.Session.Apps().Update(bson.M{"name": app.Name}, app); err != nil {
			return err
		}
		mess := message{
			app: app,
		}
		env <- mess
	}
	return nil
}

func UnsetEnv(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	msg := "You must provide the environment variables"
	if r.Body == nil {
		return &errors.Http{Code: http.StatusBadRequest, Message: msg}
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return &errors.Http{Code: http.StatusBadRequest, Message: msg}
	}
	appName := r.URL.Query().Get(":name")
	app, err := getAppOrError(appName, u)
	if err != nil {
		return err
	}
	return unsetEnvFromApp(&app, strings.Fields(string(body)), true)
}

func AppLog(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	appName := r.URL.Query().Get(":name")
	app, err := getAppOrError(appName, u)
	if err != nil {
		return err
	}
	b, err := json.Marshal(app.Logs)
	if err != nil {
		return err
	}
	fmt.Fprint(w, string(b))
	return nil
}

func serviceInstanceAndAppOrError(instanceName, appName string, u *auth.User) (instance service.ServiceInstance, a App, err error) {
	err = db.Session.ServiceInstances().Find(bson.M{"name": instanceName}).One(&instance)
	if err != nil {
		err = &errors.Http{Code: http.StatusNotFound, Message: "Instance not found"}
		return
	}
	if !auth.CheckUserAccess(instance.Teams, u) {
		err = &errors.Http{Code: http.StatusForbidden, Message: "This user does not have access to this instance"}
		return
	}
	err = db.Session.Apps().Find(bson.M{"name": appName}).One(&a)
	if err != nil {
		err = &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
		return
	}
	if !auth.CheckUserAccess(a.Teams, u) {
		err = &errors.Http{Code: http.StatusForbidden, Message: "This user does not have access to this app"}
		return
	}
	return
}

func BindHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	instanceName, appName := r.URL.Query().Get(":instance"), r.URL.Query().Get(":app")
	instance, a, err := serviceInstanceAndAppOrError(instanceName, appName, u)
	if err != nil {
		return err
	}
	err = instance.Bind(&a)
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

func UnbindHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	instanceName, appName := r.URL.Query().Get(":instance"), r.URL.Query().Get(":app")
	instance, a, err := serviceInstanceAndAppOrError(instanceName, appName, u)
	if err != nil {
		return err
	}
	return instance.Unbind(&a)
}

func RestartHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	app, err := getAppOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	if app.unit().Ip == "" {
		msg := "You can't restart this app because it doesn't have an IP yet."
		return &errors.Http{Code: http.StatusPreconditionFailed, Message: msg}
	}
	out, err := restart(&app, nil)
	if err != nil {
		return err
	}
	n, err := w.Write(out)
	if err != nil {
		return err
	}
	if n != len(out) {
		return &errors.Http{Code: http.StatusInternalServerError, Message: "Failed to write response body."}
	}
	return nil
}
