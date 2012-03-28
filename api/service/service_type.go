package service

import (
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/timeredbull/tsuru/database"
)

type ServiceType struct {
	Id    int64
	Name  string
	Charm string
}

func (st *ServiceType) Get() error {
	var query string
	var rows *sql.Rows
	var err error
	switch {
	case st.Id != 0:
		query = "SELECT id, name, charm FROM service_types WHERE id = ?"
		rows, err = Db.Query(query, st.Id)
	case st.Name != "":
		query = "SELECT id, name, charm FROM service_types WHERE name = ?"
		rows, err = Db.Query(query, st.Name)
	case st.Charm != "":
		query = "SELECT id, name, charm FROM service_types WHERE charm = ?"
		rows, err = Db.Query(query, st.Charm)
	}

	if err != nil {
		panic(err)
	}

	if rows != nil {
		for rows.Next() {
			rows.Scan(&st.Id, &st.Name, &st.Charm)
		}
	} else {
		return errors.New("No results found")
	}

	return nil
}

func (s *ServiceType) All() (result []ServiceType) {
	result = make([]ServiceType, 0)

	query := "select id, charm, name from service_types"
	rows, err := Db.Query(query)
	if err != nil {
		panic(err)
	}

	var id int64
	var charm string
	var name string
	var se ServiceType
	for rows.Next() {
		rows.Scan(&id, &charm, &name)
		se = ServiceType{
			Id:    id,
			Charm: charm,
			Name:  name,
		}
		result = append(result, se)
	}

	return
}

func (st *ServiceType) Create() error {
	query := "INSERT INTO service_types (name, charm) VALUES (?, ?)"
	insertStmt, err := Db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := Db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	result, err := stmt.Exec(st.Name, st.Charm)
	if err != nil {
		panic(err)
	}
	tx.Commit()

	st.Id, err = result.LastInsertId()

	return err
}

func (st *ServiceType) Delete() error {
	query := "DELETE FROM service_types WHERE name = ? AND charm = ?"
	insertStmt, err := Db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := Db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	stmt.Exec(st.Name, st.Charm)
	tx.Commit()

	return nil
}
