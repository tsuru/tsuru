package service

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
//	"github.com/timeredbull/tsuru/api/unit"
	. "github.com/timeredbull/tsuru/api/app"
)

type ServiceApp struct {
	Id        int
	ServiceId int
	AppId     int
}

func (sa *ServiceApp) Create() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "INSERT INTO service_app (service_id, app_id) VALUES (?, ?)"
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


	// u := unit.Unit{Name: s.AppId, Type: st.Type}
	// err = u.Create()

	return err
}

func (sa *ServiceApp) Delete() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "DELETE FROM service_app WHERE service_id = ? AND app_id = ?"
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

	// u := unit.Unit{Name: s.Name}
	// err = u.Destroy()

	return nil
}

func (sa *ServiceApp) Service() (s *Service) {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "SELECT id, name, service_type_id FROM service WHERE id = ?"
	rows, err := db.Query(query, sa.ServiceId)
	if err != nil {
		panic(err)
	}

	var id            int
	var name          string
	var serviceTypeId int
	for rows.Next() {
		rows.Scan(&id, &name, &serviceTypeId)
	}

	s = &Service{
		Id:            id,
		Name:          name,
		ServiceTypeId: serviceTypeId,
	}

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

	var id        int
	var name      string
	var framework string
	for rows.Next() {
		rows.Scan(&id, &name, &framework)
	}

	a = &App{
		Id:            id,
		Name:          name,
		Framework: framework,
	}

	return
}
