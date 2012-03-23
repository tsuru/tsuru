package service

import (
	//"fmt"
	"database/sql"
	"errors"
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

	if rows != nil {
		for rows.Next() {
			rows.Scan(&s.Id, &s.ServiceTypeId, &s.Name)
		}
	} else {
		return errors.New("Not found")
	}
	return nil
}

func (s *Service) All() (result []Service) {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	result = make([]Service, 0)

	query := "select id, service_type_id, name from service"
	rows, err := db.Query(query)
	if err != nil {
		panic(err)
	}

	var id int64
	var serviceTypeId int64
	var name string
	var se Service
	for rows.Next() {
		rows.Scan(&id, &serviceTypeId, &name)
		se = Service{
			Id:            id,
			ServiceTypeId: serviceTypeId,
			Name:          name,
		}
		result = append(result, se)
	}

	return
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
	//fmt.Println(s.ServiceTypeId)
	result, err := stmt.Exec(s.ServiceTypeId, s.Name)
	if err != nil {
		panic(err)
	}
	tx.Commit()

	s.Id, err = result.LastInsertId()
	if err != nil {
		panic(err)
	}

	u := unit.Unit{Name: s.Name, Type: "mysql"}
	err = u.Create()

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

	u := unit.Unit{Name: s.Name, Type: s.ServiceType().Name}
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
