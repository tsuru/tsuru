package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"github.com/timeredbull/tsuru/repository"
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
	var write = func(w http.ResponseWriter, content []byte) error {
		out := filterOutput(content, nil)
		n, err := w.Write(out)
		if err != nil {
			return err
		}
		if n != len(out) {
			return io.ErrShortWrite
		}
		return nil
	}
	w.Header().Set("Content-Type", "text")
	var output string
	app := App{Name: r.URL.Query().Get(":name")}
	err := app.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
	}
	output, err = repository.CloneOrPull(app.unit()) // should iterate over the machines
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: output}
	}
	err = write(w, []byte(output))
	if err != nil {
		return err
	}
	c, err := app.conf()
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	out, err := app.preRestart(c)
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	err = write(w, out)
	if err != nil {
		return err
	}
	out, err = app.updateHooks()
	if err != nil {
		write(w, out)
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	err = write(w, out)
	if err != nil {
		return err
	}
	out, err = app.posRestart(c)
	err = write(w, out)
	if err != nil {
		return err
	}
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	return nil
}

func AppDelete(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	app, err := getAppOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	err = app.Destroy()
	if err != nil {
		return err
	}
	for _, t := range app.GetTeams() {
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
	fmt.Fprint(w, bytes.NewBuffer(b).String())
	return nil
}

func AppInfo(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	app, err := getAppOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	b, err := json.Marshal(app)
	if err != nil {
		return err
	}
	fmt.Fprint(w, bytes.NewBuffer(b).String())
	return nil
}

func createApp(app *App, u *auth.User) ([]byte, error) {
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
	_, err = NewApp(app.Name, app.Framework, app.Teams)
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
	jsonMsg, err := createApp(&app, u)
	if err != nil {
		return err
	}

	fmt.Fprint(w, bytes.NewBuffer(jsonMsg).String())
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
	err = app.GrantAccess(t)
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
	err = app.RevokeAccess(t)
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
	cmd := fmt.Sprintf("[ -f /home/application/apprc ] && source /home/application/apprc; [ -d /home/application/current ] && cd /home/application/current; %s", c)
	app.Log(fmt.Sprintf("running '%s'", c))
	out, err := app.unit().Command(cmd)
	if err != nil {
		return err
	}
	out = filterOutput(out, nil)
	app.Log(string(out))
	n, err := w.Write(out)
	if err != nil {
		return err
	}
	if n != len(out) {
		return &errors.Http{Code: http.StatusInternalServerError, Message: "Unexpected error writing the output"}
	}
	return nil
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
	var write = func(v EnvVar) error {
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

func SetEnvsToApp(app *App, envs []EnvVar, publicOnly bool) error {
	if len(envs) > 0 {
		for _, env := range envs {
			set := true
			if publicOnly {
				e, err := app.GetEnv(env.Name)
				if err == nil && !e.Public {
					set = false
				}
			}
			if set {
				app.SetEnv(env)
			}
		}
		if err := db.Session.Apps().Update(bson.M{"name": app.Name}, app); err != nil {
			return err
		}
		mess := Message{
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
	envs := make([]EnvVar, len(variables))
	for i, v := range variables {
		parts := strings.Split(v[1], "=")
		envs[i] = EnvVar{Name: parts[0], Value: parts[1], Public: true}
	}
	return SetEnvsToApp(&app, envs, true)
}

func UnsetEnvFromApp(app *App, variableNames []string, publicOnly bool) error {
	if len(variableNames) > 0 {
		for _, name := range variableNames {
			var unset bool
			e, err := app.GetEnv(name)
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
		mess := Message{
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
	return UnsetEnvFromApp(&app, strings.Fields(string(body)), true)
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
	fmt.Fprint(w, bytes.NewBuffer(b).String())
	return nil
}

func serviceInstanceAndAppOrError(instanceName, appName string, u *auth.User) (instance service.ServiceInstance, a App, err error) {
	err = db.Session.ServiceInstances().Find(bson.M{"_id": instanceName}).One(&instance)
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
	return instance.Bind(&a)
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
	out, err := app.unit().ExecuteHook("restart")
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
