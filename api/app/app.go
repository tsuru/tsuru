package app

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/unit"
)

type App struct {
	Id        int64
	Name      string
	Framework string
	State     string
}

func (app *App) Get() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "SELECT id, framework, state FROM apps WHERE name = ?"
	rows, err := db.Query(query, app.Name)
	if err != nil {
		return err
	}

	for rows.Next() {
		rows.Scan(&app.Id, &app.Framework, &app.State)
	}

	return nil
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
	result, err := stmt.Exec(app.Name, app.Framework, app.State)
	if err != nil {
		panic(err)
	}

	tx.Commit()

	app.Id, err = result.LastInsertId()
	if err != nil {
		panic(err)
	}

	u := unit.Unit{Name: app.Name, Type: app.Framework}
	err = u.Create()

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
