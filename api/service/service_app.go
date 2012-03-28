package service

import (
	_ "github.com/mattn/go-sqlite3"
	. "github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/database"
	"github.com/timeredbull/tsuru/api/unit"
)

type ServiceApp struct {
	Id        int64
	ServiceId int64
	AppId     int64
}

func (sa *ServiceApp) Create() error {
	query := "INSERT INTO service_apps (service_id, app_id) VALUES (?, ?)"
	insertStmt, err := Db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := Db.Begin()
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
	query := "DELETE FROM service_apps WHERE service_id = ? AND app_id = ?"
	insertStmt, err := Db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := Db.Begin()
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
	query := "SELECT id, name, framework FROM apps WHERE id = ?"
	rows, err := Db.Query(query, sa.AppId)
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
