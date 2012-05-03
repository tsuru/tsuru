package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"github.com/timeredbull/tsuru/log"
	"io/ioutil"
	"launchpad.net/mgo/bson"
	"net/http"
	"os"
	"os/exec"
	"time"
)

func Upload(w http.ResponseWriter, r *http.Request) error {
	app := App{Name: r.URL.Query().Get(":name")}
	err := app.Get()

	if err != nil {
		http.NotFound(w, r)
	} else {
		f, _, err := r.FormFile("application")
		if err != nil {
			return err
		}

		releaseName := time.Now().Format("20060102150405")
		zipFile := fmt.Sprintf("/tmp/%s.zip", releaseName)
		zipDir := fmt.Sprintf("/tmp/%s", releaseName)

		newFile, err := os.Create(zipFile)
		if err != nil {
			return err
		}
		out, _ := ioutil.ReadAll(f)
		newFile.Write(out)

		cmd := exec.Command("unzip", zipFile, "-d", zipDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return err
		}
		log.Printf(string(output))

		appDir := "/home/application"
		currentDir := appDir + "/releases/current"
		gunicorn := appDir + "/env/bin/gunicorn_django"
		releasesDir := appDir + "/releases"
		releaseDir := releasesDir + "/" + releaseName

		u := unit.Unit{Name: app.Name}
		err = u.SendFile(zipDir, releaseDir)
		if err != nil {
			return err
		}

		output, err = u.Command(fmt.Sprintf("cd %s && ln -nfs %s current", releasesDir, releaseName))
		log.Printf(string(output))
		if err != nil {
			return err
		}

		err = u.ExecuteHook("dependencies")
		if err != nil {
			return err
		}

		output, err = u.Command("sudo killall gunicorn_django")
		log.Printf(string(output))
		if err != nil {
			return err
		}

		output, err = u.Command(fmt.Sprintf("cd %s && sudo %s --daemon --workers=3 --bind=127.0.0.1:8888", currentDir, gunicorn))
		log.Printf(string(output))
		if err != nil {
			return err
		}

		fmt.Fprint(w, "success")
	}
	return nil
}

func CloneRepositoryHandler(w http.ResponseWriter, r *http.Request) error {
	app := App{Name: r.URL.Query().Get(":name")}
	CloneRepository(&app)
	fmt.Fprint(w, "success")
	return nil
}

func AppDelete(w http.ResponseWriter, r *http.Request) error {
	app := App{Name: r.URL.Query().Get(":name")}
	app.Destroy()
	fmt.Fprint(w, "success")
	return nil
}

func AppList(w http.ResponseWriter, r *http.Request) error {
	apps, err := AllApps()
	if err != nil {
		return err
	}

	b, err := json.Marshal(apps)
	if err != nil {
		return err
	}
	fmt.Fprint(w, bytes.NewBuffer(b).String())
	return nil
}

func AppInfo(w http.ResponseWriter, r *http.Request) error {
	app := App{Name: r.URL.Query().Get(":name")}
	err := app.Get()

	if err != nil {
		http.NotFound(w, r)
	} else {
		b, err := json.Marshal(app)
		if err != nil {
			return err
		}
		fmt.Fprint(w, bytes.NewBuffer(b).String())
	}
	return nil
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

	err = db.Session.Teams().Find(bson.M{"users.email": u.Email}).All(&app.Teams)
	if err != nil {
		return err
	}
	err = app.Create()
	if err != nil {
		return err
	}

	msg := map[string]string{
		"status":         "success",
		"repository_url": GetRepositoryUrl(&app),
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	fmt.Fprint(w, bytes.NewBuffer(jsonMsg).String())
	return nil
}

func GrantAccessToTeamHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	t := new(auth.Team)
	app := &App{Name: r.URL.Query().Get(":app")}
	err := app.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
	}
	if !app.CheckUserAccess(u) {
		return &errors.Http{Code: http.StatusUnauthorized, Message: "User unauthorized"}
	}
	err = db.Session.Teams().Find(bson.M{"name": r.URL.Query().Get(":team")}).One(t)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Team not found"}
	}
	err = app.GrantAccess(t)
	if err != nil {
		return &errors.Http{Code: http.StatusConflict, Message: err.Error()}
	}
	return db.Session.Apps().Update(bson.M{"name": app.Name}, app)
}

func RevokeAccessFromTeamHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	t := new(auth.Team)
	app := &App{Name: r.URL.Query().Get(":app")}
	err := app.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "App not found"}
	}
	if !app.CheckUserAccess(u) {
		return &errors.Http{Code: http.StatusUnauthorized, Message: "User unauthorized"}
	}
	err = db.Session.Teams().Find(bson.M{"name": r.URL.Query().Get(":team")}).One(t)
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
	return db.Session.Apps().Update(bson.M{"name": app.Name}, app)
}
