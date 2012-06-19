package app

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"github.com/timeredbull/tsuru/repository"
	"io/ioutil"
	"launchpad.net/mgo/bson"
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
	if !app.CheckUserAccess(u) {
		return app, &errors.Http{Code: http.StatusForbidden, Message: "User does not have access to this app"}
	}
	return app, nil
}

func CloneRepositoryHandler(w http.ResponseWriter, r *http.Request) error {
	var output string
	app := App{Name: r.URL.Query().Get(":name")}
	err := app.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
	}
	output, err = repository.CloneOrPull(app.Name, app.Machine)
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: output}
	}
	c, err := app.conf()
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	err = app.preRestart(c)
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	err = app.updateHooks()
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	err = app.posRestart(c)
	if err != nil {
		return &errors.Http{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	fmt.Fprint(w, output)
	return nil
}

func AppDelete(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	app, err := getAppOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	app.Destroy()
	for _, t := range app.Teams {
		sendProjectChangeToGitosis(repository.RemoveProject, &t, &app)
	}
	fmt.Fprint(w, "success")
	return nil
}

func AppList(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	var apps []App
	err := db.Session.Apps().Find(bson.M{"teams.users.email": u.Email}).All(&apps)
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
	err := db.Session.Teams().Find(bson.M{"users.email": u.Email}).All(&app.Teams)
	if err != nil {
		return nil, err
	}
	if len(app.Teams) < 1 {
		msg := "In order to create an app, you should be member of at least one team"
		return nil, &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	err = app.Create()
	if err != nil {
		return nil, err
	}
	for _, t := range app.Teams {
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
	if !app.CheckUserAccess(u) {
		return &errors.Http{Code: http.StatusUnauthorized, Message: "User unauthorized"}
	}
	err = db.Session.Teams().Find(bson.M{"name": teamName}).One(t)
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
	if !app.CheckUserAccess(u) {
		return &errors.Http{Code: http.StatusUnauthorized, Message: "User unauthorized"}
	}
	err = db.Session.Teams().Find(bson.M{"name": teamName}).One(t)
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
	unit := app.unit()
	cmd := fmt.Sprintf("cd /home/application/current; %s", c)
	out, err := unit.Command(cmd)
	if err != nil {
		return err
	}
	out = filterOutput(out, nil)
	n, err := w.Write(out)
	if err != nil {
		return err
	}
	if n != len(out) {
		return stderrors.New("Unexpected error writing the output")
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
	if variables := strings.Fields(string(variable)); len(variables) > 0 {
		for _, variable := range variables {
			if value, ok := app.Env[variable]; ok {
				_, err = fmt.Fprintf(w, "%s=%s\n", variable, value)
				if err != nil {
					return
				}
			}
		}
	} else {
		for k, v := range app.Env {
			_, err = fmt.Fprintf(w, "%s=%s\n", k, v)
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
	regex, err := regexp.Compile(`([A-Z_]+=[^=.]+)(\s|$)`)
	if err != nil {
		return err
	}
	e := map[string]string{}
	variables := regex.FindAllStringSubmatch(string(body), -1)
	for _, v := range variables {
		parts := strings.Split(v[1], "=")
		app.SetEnv(parts[0], parts[1])
		e[parts[0]] = parts[1]
	}
	if err = db.Session.Apps().Update(bson.M{"name": app.Name}, app); err != nil {
		return err
	}
	mess := Message{
		app:  &app,
	}
	env <- mess
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
	e := map[string]string{}
	variables := strings.Fields(string(body))
	for _, variable := range variables {
		delete(app.Env, variable)
		e[variable] = ""
	}
	if err = db.Session.Apps().Update(bson.M{"name": app.Name}, app); err != nil {
		return err
	}
	mess := Message{
		app:  &app,
	}
	env <- mess
	return nil
}
