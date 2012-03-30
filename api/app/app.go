package app

import (
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/unit"
	. "github.com/timeredbull/tsuru/database"
)

type App struct {
	Id        int
	Ip        string
	Name      string
	Framework string
	State     string
}

func AllApps() ([]App, error) {
	query := "SELECT id, name, framework, ip, state FROM apps"
	rows, err := Db.Query(query)
	if err != nil {
		return []App{}, err
	}

	apps := make([]App, 0)
	var app App
	for rows.Next() {
		app = App{}
		rows.Scan(&app.Id, &app.Name, &app.Framework, &app.Ip, &app.State)
		apps = append(apps, app)
	}
	return apps, err
}

func (app *App) Get() error {
	query := "SELECT id, framework, state, ip FROM apps WHERE name = ?"
	rows, err := Db.Query(query, app.Name)
	if err != nil {
		return err
	}

	for rows.Next() {
		rows.Scan(&app.Id, &app.Framework, &app.State, &app.Ip)
	}

	return nil
}

func (app *App) Create() error {
	app.State = "Pending"

	insertApp, err := Db.Prepare("INSERT INTO apps (name, framework, state, ip) VALUES (?, ?, ?, ?)")
	if err != nil {
		panic(err)
	}
	tx, err := Db.Begin()

	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertApp)
	_, err = stmt.Exec(app.Name, app.Framework, app.State, app.Ip)
	if err != nil {
		panic(err)
	}

	tx.Commit()

	//app.Id, err = result.LastInsertId()
	if err != nil {
		panic(err)
	}

	u := unit.Unit{Name: app.Name, Type: app.Framework}
	err = u.Create()

	return nil
}

func (app *App) Destroy() error {
	deleteApp, err := Db.Prepare("DELETE FROM apps WHERE name = ?")
	if err != nil {
		panic(err)
	}
	tx, err := Db.Begin()

	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(deleteApp)
	stmt.Exec(app.Name)
	tx.Commit()

	u := unit.Unit{Name: app.Name, Type: app.Framework}
	u.Destroy()

	return nil
}
