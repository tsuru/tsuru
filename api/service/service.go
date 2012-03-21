package service

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/unit"
	. "github.com/timeredbull/tsuru/api/app"
)

type Service struct {
	Id	          int64
	ServiceTypeId int64
	Name          string
}

func (s *Service) Create() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "INSERT INTO service (service_type_id, name) VALUES (?, ?)"
	insertStmt, err := db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	result, err := stmt.Exec(s.ServiceTypeId, s.Name)
	if err != nil {
		panic(err)
	}
	tx.Commit()

	s.Id, err = result.LastInsertId()

	return err
}

func (s *Service) Delete() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "DELETE FROM service WHERE name = ? AND service_type_id = ?"
	insertStmt, err := db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	stmt.Exec(s.Name, s.ServiceTypeId)
	tx.Commit()

	u := unit.Unit{Name: s.Name}
	err = u.Destroy()

	return nil
}

func (s *Service) Bind(app *App) error {
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
	stmt.Exec(s.Id, app.Id)
	tx.Commit()

	//appUnit := unit.Unit{Name: app.Name}
	//serviceUnit := unit.Unit{Name: s.Name}
	//serviceUnit.addRelation(appUnit)

	return nil
}

func (s *Service) ServiceType() (st *ServiceType) {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "SELECT id, name, charm FROM service_type WHERE id = ?"
	rows, err := db.Query(query, s.ServiceTypeId)
	if err != nil {
		panic(err)
	}

	var id    int64
	var name  string
	var charm string
	for rows.Next() {
		rows.Scan(&id, &name, &charm)
	}

	st = &ServiceType{
		Id:    id,
		Name:  name,
		Charm: charm,
	}

	return
}
