package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"github.com/timeredbull/tsuru/repository"
	"io/ioutil"
	"launchpad.net/mgo/bson"
	"net/http"
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
	var output []byte
	app := App{Name: r.URL.Query().Get(":name")}
	err := app.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
	}
	output, err = repository.Clone(app.Name, app.Machine)
	if err != nil {
		output, err = repository.Pull(app.Name, app.Machine)
		if err != nil {
			return &errors.Http{Code: http.StatusInternalServerError, Message: string(output)}
		}
	}
	u := unit.Unit{Name: app.Name, Machine: app.Machine}
	u.ExecuteHook("dependencies")
	fmt.Fprint(w, string(output))
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
