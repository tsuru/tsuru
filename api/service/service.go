package service

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/unit"
)

type Service struct {
	Id            int64
	ServiceTypeId int64
	Name          string
}

func (s *Service) Get() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	var query string
	var rows *sql.Rows
	var err error
	switch {
	case s.Id != 0:
		query = "SELECT id, service_type_id, name FROM service WHERE id = ?"
		rows, err = db.Query(query, s.Id)
	case s.Name != "":
		query = "SELECT id, service_type_id, name FROM service WHERE name = ?"
		rows, err = db.Query(query, s.Name)
	}

	if err != nil {
		panic(err)
	}

	for rows.Next() {
		rows.Scan(&s.Id, &s.ServiceTypeId, &s.Name)
	}
	return nil
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
	sa := ServiceApp{ServiceId: s.Id, AppId: app.Id}
	sa.Create()

	appUnit := unit.Unit{Name: app.Name}
	serviceUnit := unit.Unit{Name: s.Name}
	appUnit.AddRelation(&serviceUnit)

	return nil
}

func (s *Service) Unbind(app *App) error {
	sa := ServiceApp{ServiceId: s.Id, AppId: app.Id}
	sa.Delete()

	appUnit := unit.Unit{Name: app.Name}
	serviceUnit := unit.Unit{Name: s.Name}
	appUnit.RemoveRelation(&serviceUnit)

	return nil
}

func (s *Service) ServiceType() (st *ServiceType) {
	st = &ServiceType{Id: s.ServiceTypeId}
	st.Get()
	return
}
