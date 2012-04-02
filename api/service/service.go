package service

import (
	"database/sql"
	"errors"
	/* "github.com/cobrateam/gothic/sqlgen" */
	_ "github.com/mattn/go-sqlite3"
	//_ "github.com/ziutek/mymysql/godrv"
	. "github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/unit"
	. "github.com/timeredbull/tsuru/database"
)

type Service struct {
	Id            int64
	ServiceTypeId int64
	Name          string
}

func (s *Service) Get() error {
	var query string
	var rows *sql.Rows
	var err error
	switch {
	case s.Id != 0:
		query = "SELECT id, service_type_id, name FROM services WHERE id = ?"
		rows, err = Db.Query(query, s.Id)
	case s.Name != "":
		query = "SELECT id, service_type_id, name FROM services WHERE name = ?"
		rows, err = Db.Query(query, s.Name)
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
	result = make([]Service, 0)

	query := "select id, service_type_id, name from services"
	/* query := sqlgen.Select(s) */
	rows, err := Db.Query(query)
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
	query := "INSERT INTO services (service_type_id, name) VALUES (?, ?)"
	insertStmt, err := Db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := Db.Begin()
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
	query := "DELETE FROM services WHERE name = ? AND service_type_id = ?"
	insertStmt, err := Db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := Db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	stmt.Exec(s.Name, s.ServiceTypeId)
	tx.Commit()

	u := unit.Unit{Name: s.Name, Type: s.ServiceType().Charm}
	err = u.Destroy()

	return nil
}

func (s *Service) Bind(app *App) error {
	sa := ServiceApp{ServiceId: s.Id, AppId: app.Id}
	sa.Create()

	return nil
}

func (s *Service) Unbind(app *App) error {
	sa := ServiceApp{ServiceId: s.Id, AppId: app.Id}
	sa.Delete()

	return nil
}

func (s *Service) ServiceType() (st *ServiceType) {
	st = &ServiceType{Id: s.ServiceTypeId}
	st.Get()
	return
}
