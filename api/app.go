// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/go-gandalfclient"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/service"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func write(w io.Writer, content []byte) error {
	n, err := w.Write(content)
	if err != nil {
		return err
	}
	if n != len(content) {
		return io.ErrShortWrite
	}
	return nil
}

func getAppOrError(name string, u *auth.User) (app.App, error) {
	app := app.App{Name: name}
	err := app.Get()
	if err != nil {
		return app, &errors.Http{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", name)}
	}
	if !auth.CheckUserAccess(app.Teams, u) {
		return app, &errors.Http{Code: http.StatusForbidden, Message: "User does not have access to this app"}
	}
	return app, nil
}

func CloneRepositoryHandler(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text")
	instance := app.App{Name: r.URL.Query().Get(":name")}
	err := instance.Get()
	logWriter := LogWriter{&instance, w}
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", instance.Name)}
	}
	err = write(&logWriter, []byte("\n ---> Tsuru receiving push\n"))
	if err != nil {
		return err
	}
	err = write(&logWriter, []byte("\n ---> Cloning your code in your machines\n"))
	if err != nil {
		return err
	}
	out, err := repository.CloneOrPull(&instance) // should iterate over the machines
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: string(out)}
	}
	err = write(&logWriter, out)
	if err != nil {
		return err
	}
	err = write(&logWriter, []byte("\n ---> Installing dependencies\n"))
	if err != nil {
		return err
	}
	err = instance.InstallDeps(&logWriter)
	if err != nil {
		return err
	}
	err = instance.Restart(&logWriter)
	if err != nil {
		return err
	}
	return write(&logWriter, []byte("\n ---> Deploy done!\n\n"))
}

// AppIsAvaliableHandler verify if the app.Unit().State() is
// started. If is started it returns 200 else returns 500 for
// status code.
func AppIsAvaliableHandler(w http.ResponseWriter, r *http.Request) error {
	app := app.App{Name: r.URL.Query().Get(":name")}
	err := app.Get()
	if err != nil {
		return err
	}
	if app.State != "started" {
		return fmt.Errorf("App must be started to receive pushs, but it is %q.", app.State)
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func AppDelete(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	app, err := getAppOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	gUrl := repository.GitServerUri()
	if err := (&gandalf.Client{Endpoint: gUrl}).RemoveRepository(app.Name); err != nil {
		log.Printf("Got error while removing repository from gandalf: %s", err.Error())
		return &errors.Http{Code: http.StatusInternalServerError, Message: "Could not remove app's repository at git server. Aborting..."}
	}
	if err := app.Destroy(); err != nil {
		return err
	}
	fmt.Fprint(w, "success")
	return nil
}

func getTeamNames(u *auth.User) ([]string, error) {
	var teams []auth.Team
	if err := db.Session.Teams().Find(bson.M{"users": u.Email}).All(&teams); err != nil {
		return nil, err
	}
	return auth.GetTeamsNames(teams), nil
}

func AppList(w http.ResponseWriter, r *http.Request, u *auth.User) error {
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

func AppInfo(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	app, err := getAppOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(&app)
}

func createAppHelper(instance *app.App, u *auth.User, units uint) ([]byte, error) {
	teams, err := u.Teams()
	if err != nil {
		return nil, err
	}
	if len(teams) < 1 {
		msg := "In order to create an app, you should be member of at least one team"
		return nil, &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	instance.SetTeams(teams)
	err = app.CreateApp(instance, units)
	if err != nil {
		log.Printf("Got error while creating app: %s", err)
		if e, ok := err.(*app.ValidationError); ok {
			return nil, &errors.Http{Code: http.StatusPreconditionFailed, Message: e.Message}
		}
		if strings.Contains(err.Error(), "key error") {
			msg := fmt.Sprintf(`There is already an app named "%s".`, instance.Name)
			return nil, &errors.Http{Code: http.StatusConflict, Message: msg}
		}
		return nil, err
	}
	msg := map[string]string{
		"status":         "success",
		"repository_url": repository.GetUrl(instance.Name),
	}
	return json.Marshal(msg)
}

type jsonApp struct {
	Name      string
	Framework string
	Units     uint
}

func CreateAppHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	var app app.App
	var japp jsonApp
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(body, &japp); err != nil {
		return err
	}
	app.Name = japp.Name
	app.Framework = japp.Framework
	if japp.Units == 0 {
		japp.Units = 1
	}
	jsonMsg, err := createAppHelper(&app, u, japp.Units)
	if err != nil {
		return err
	}
	fmt.Fprint(w, string(jsonMsg))
	return nil
}

func numberOfUnitsOrError(r *http.Request) (uint, error) {
	missingMsg := "You must provide the number of units."
	if r.Body == nil {
		return 0, &errors.Http{Code: http.StatusBadRequest, Message: missingMsg}
	}
	defer r.Body.Close()
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return 0, err
	}
	value := string(b)
	if value == "" {
		return 0, &errors.Http{Code: http.StatusBadRequest, Message: missingMsg}
	}
	n, err := strconv.ParseUint(value, 10, 32)
	if err != nil || n == 0 {
		return 0, &errors.Http{
			Code:    http.StatusBadRequest,
			Message: "Invalid number of units: the number must be an integer greater than 0.",
		}
	}
	return uint(n), nil
}

func AddUnitsHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	n, err := numberOfUnitsOrError(r)
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":name")
	app, err := getAppOrError(appName, u)
	if err != nil {
		return err
	}
	return app.AddUnits(n)
}

func RemoveUnitsHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	n, err := numberOfUnitsOrError(r)
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":name")
	app, err := getAppOrError(appName, u)
	if err != nil {
		return err
	}
	return app.RemoveUnits(uint(n))
}

func grantAccessToTeam(appName, teamName string, u *auth.User) error {
	t := new(auth.Team)
	app, err := getAppOrError(appName, u)
	if err != nil {
		return err
	}
	err = db.Session.Teams().Find(bson.M{"_id": teamName}).One(t)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Team not found"}
	}
	err = app.Grant(t)
	if err != nil {
		return &errors.Http{Code: http.StatusConflict, Message: err.Error()}
	}
	err = db.Session.Apps().Update(bson.M{"name": app.Name}, app)
	if err != nil {
		return err
	}
	gUrl := repository.GitServerUri()
	return (&gandalf.Client{Endpoint: gUrl}).GrantAccess([]string{app.Name}, t.Users)
}

func GrantAccessToTeamHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	appName := r.URL.Query().Get(":app")
	teamName := r.URL.Query().Get(":team")
	return grantAccessToTeam(appName, teamName, u)
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

func revokeAccessFromTeam(appName, teamName string, u *auth.User) error {
	t := new(auth.Team)
	app, err := getAppOrError(appName, u)
	if err != nil {
		return err
	}
	err = db.Session.Teams().Find(bson.M{"_id": teamName}).One(t)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Team not found"}
	}
	if len(app.Teams) == 1 {
		msg := "You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned"
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	err = app.Revoke(t)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: err.Error()}
	}
	err = db.Session.Apps().Update(bson.M{"name": app.Name}, app)
	if err != nil {
		return err
	}
	users := getEmailsForRevoking(&app, t)
	if len(users) > 0 {
		gUrl := repository.GitServerUri()
		if err := (&gandalf.Client{Endpoint: gUrl}).RevokeAccess([]string{app.Name}, users); err != nil {
			return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
		}
	}
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
	return app.Run(string(c), w)
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
	return app.SetEnvsToApp(envs, true, false)
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
	return app.UnsetEnvsFromApp(strings.Fields(string(body)), true, false)
}

func AppLog(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	w.Header().Set("Content-Type", "application/json")
	appName := r.URL.Query().Get(":name")
	pipe := []bson.M{}
	pipe = append(pipe, bson.M{"$unwind": "$logs"})
	match := bson.M{}
	match["name"] = appName
	if source := r.URL.Query().Get("source"); source != "" {
		match["logs.source"] = source
	}
	_, err := getAppOrError(appName, u)
	if err != nil {
		return err
	}
	pipe = append(pipe, bson.M{"$match": match})
	pipe = append(pipe, bson.M{"$project": bson.M{"_id": 0, "logs": 1}})
	pipe = append(pipe, bson.M{"$sort": bson.M{"logs.date": -1}})
	if l := r.URL.Query().Get("lines"); l != "" {
		lines, err := strconv.Atoi(l)
		if err != nil {
			return err
		}
		pipe = append(pipe, bson.M{"$limit": lines})
	}
	var result []map[string]map[string]interface{}
	err = db.Session.Apps().Pipe(pipe).All(&result)
	if err != nil {
		return err
	}
	n := len(result)
	logs := make([]app.Applog, n)
	for i, row := range result {
		log := app.Applog{
			Message: row["logs"]["message"].(string),
			Source:  row["logs"]["source"].(string),
			Date:    row["logs"]["date"].(time.Time),
		}
		logs[n-i-1] = log
	}
	b, err := json.Marshal(logs)
	if err != nil {
		return err
	}
	return write(w, b)
}

func serviceInstanceAndAppOrError(instanceName, appName string, u *auth.User) (instance service.ServiceInstance, a app.App, err error) {
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
		err = &errors.Http{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", appName)}
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
	w.Header().Set("Content-Type", "text")
	instance, err := getAppOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	return instance.Restart(w)
}

func AddLogHandler(w http.ResponseWriter, r *http.Request) error {
	app := app.App{Name: r.URL.Query().Get(":name")}
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
