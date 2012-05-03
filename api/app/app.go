package app

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
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
	Teams     []auth.Team
}

func NewRepository(app *App) (err error) {
	repoPath := GetRepositoryPath(app)

	err = os.Mkdir(repoPath, 0700)
	if err != nil {
		return
	}

	oldPwd, err := os.Getwd()
	if err != nil {
		return
	}

	err = os.Chdir(repoPath)
	if err != nil {
		return
	}

	err = exec.Command("git", "init", "--bare").Run()
	if err != nil {
		return
	}

	err = os.Chdir(oldPwd)
	return
}

func DeleteRepository(app *App) error {
	return os.RemoveAll(GetRepositoryPath(app))
}

func CloneRepository(app *App) (err error) {
	u := unit.Unit{Name: app.Name}
	cmd := fmt.Sprintf("git clone %s /home/application/%s", GetRepositoryUrl(app), app.Name)
	output, err := u.Command(cmd)
	if err != nil {
		log.Print(err)
		return
	}

	log.Print(output)
	return
}

func GetRepositoryPath(app *App) string {
	home := os.Getenv("HOME")
	return path.Join(home, "../git", GetRepositoryName(app))
}

func GetRepositoryUrl(app *App) string {
	return fmt.Sprintf("git@%s:%s", gitServer, GetRepositoryName(app))
}

func GetRepositoryName(app *App) string {
	return fmt.Sprintf("%s.git", app.Name)
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

	err = NewRepository(app)
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

	err = DeleteRepository(app)
	if err != nil {
		return err
	}

	u := unit.Unit{Name: app.Name, Type: app.Framework}
	u.Destroy()

	return nil
}

func (app *App) findTeam(team *auth.Team) int {
	for i, t := range app.Teams {
		if t.Name == team.Name {
			return i
		}
	}
	return -1
}

func (app *App) hasTeam(team *auth.Team) bool {
	return app.findTeam(team) > -1
}

func (app *App) GrantAccess(team *auth.Team) error {
	if app.hasTeam(team) {
		return errors.New("This team has already access to this app")
	}
	app.Teams = append(app.Teams, *team)
	return nil
}

func (app *App) RevokeAccess(team *auth.Team) error {
	index := app.findTeam(team)
	if index < 0 {
		return errors.New("This team does not have access to this app")
	}
	last := len(app.Teams) - 1
	app.Teams[index] = app.Teams[last]
	app.Teams = app.Teams[:last]
	return nil
}

func (app *App) CheckUserAccess(user *auth.User) bool {
	for _, team := range app.Teams {
		if team.ContainsUser(user) {
			return true
		}
	}
	return false
}
