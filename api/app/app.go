package app

import (
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"launchpad.net/mgo/bson"
	"os"
	"os/exec"
	"path"
)

const gitServer = "tsuru.plataformas.glb.com"

type App struct {
	Ip        string
	Name      string
	Framework string
	State     string
}

type Repository struct {
	Name   string
	Server string
}

func NewRepository(name string) (r *Repository, err error) {
	r = &Repository{
		Name: name,
		Server: gitServer,
	}

	home := os.Getenv("HOME")
	repoPath := path.Join(home, "../git", name)
	err = os.Mkdir(repoPath, 0700)
	if err != nil {
		return nil, err
	}

	oldPwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	err = os.Chdir(repoPath)
	if err != nil {
		return nil, err
	}

	err = exec.Command("git", "init", "--bare").Run()
	if err != nil {
		return nil, err
	}

	err = os.Chdir(oldPwd)

	return
}

func AllApps() ([]App, error) {
	var apps []App
	err := db.Session.Apps().Find(nil).All(&apps)
	return apps, err
}

func (app *App) Get() error {
	return db.Session.Apps().Find(bson.M{"name": app.Name}).One(&app)
}

func (app *App) Create() error {
	app.State = "Pending"
	err := db.Session.Apps().Insert(app)
	if err != nil {
		return err
	}

	u := unit.Unit{Name: app.Name, Type: app.Framework}
	err = u.Create()
	if err != nil {
		return err
	}

	return nil
}

func (app *App) Destroy() error {
	err := db.Session.Apps().Remove(app)

	if err != nil {
		return err
	}
	u := unit.Unit{Name: app.Name, Type: app.Framework}
	u.Destroy()

	return nil
}
