package app

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

type App struct {
	Name      string
	Framework string
	State     string
}

func (app *App) Create() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	app.State = "Pending"

	insertApp, err := db.Prepare("INSERT INTO apps (name, framework, state) VALUES (?, ?, ?)")
	if err != nil {
		panic(err)
	}
	tx, err := db.Begin()

	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertApp)
	stmt.Exec(app.Name, app.Framework, app.State)
	tx.Commit()

	return nil
}

func (app *App) Destroy() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	deleteApp, err := db.Prepare("DELETE FROM apps WHERE name = ?")
	if err != nil {
		panic(err)
	}
	tx, err := db.Begin()

	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(deleteApp)
	stmt.Exec(app.Name)
	tx.Commit()

	return nil
}
