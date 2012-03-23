package service

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/unit"
)

type ServiceApp struct {
	Id        int64
	ServiceId int64
	AppId     int64
}

func (sa *ServiceApp) Create() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "INSERT INTO service_apps (service_id, app_id) VALUES (?, ?)"
	insertStmt, err := db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	stmt.Exec(sa.ServiceId, sa.AppId)
	tx.Commit()

	s := sa.Service()
	a := sa.App()

	appUnit := unit.Unit{Name: a.Name}
	serviceUnit := unit.Unit{Name: s.Name}
	appUnit.AddRelation(&serviceUnit)


	return err
}

func (sa *ServiceApp) Delete() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "DELETE FROM service_apps WHERE service_id = ? AND app_id = ?"
	insertStmt, err := db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	stmt.Exec(sa.ServiceId, sa.AppId)
	tx.Commit()

	s := sa.Service()
	a := sa.App()

	appUnit := unit.Unit{Name: a.Name}
	serviceUnit := unit.Unit{Name: s.Name}
	appUnit.RemoveRelation(&serviceUnit)

	return nil
}

func (sa *ServiceApp) Service() (s *Service) {
	s = &Service{Id: sa.ServiceId}
	s.Get()

	return
}

func (sa *ServiceApp) App() (a *App) {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "SELECT id, name, framework FROM apps WHERE id = ?"
	rows, err := db.Query(query, sa.AppId)
	if err != nil {
		panic(err)
	}

	var id int64
	var name string
	var framework string
	for rows.Next() {
		rows.Scan(&id, &name, &framework)
	}

	a = &App{
		Id:        id,
		Name:      name,
		Framework: framework,
	}

	return
}
