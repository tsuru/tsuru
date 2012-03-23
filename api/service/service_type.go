package service

import (
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3"
)

type ServiceType struct {
	Id    int64
	Name  string
	Charm string
}

func (st *ServiceType) Get() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	var query string
	var rows *sql.Rows
	var err error
	switch {
	case st.Id != 0:
		query = "SELECT id, name, charm FROM service_types WHERE id = ?"
		rows, err = db.Query(query, st.Id)
	case st.Name != "":
		query = "SELECT id, name, charm FROM service_types WHERE name = ?"
		rows, err = db.Query(query, st.Name)
	case st.Charm != "":
		query = "SELECT id, name, charm FROM service_types WHERE charm = ?"
		rows, err = db.Query(query, st.Charm)
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
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	result = make([]ServiceType, 0)

	query := "select id, charm, name from service_types"
	rows, err := db.Query(query)
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
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "INSERT INTO service_types (name, charm) VALUES (?, ?)"
	insertStmt, err := db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := db.Begin()
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
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "DELETE FROM service_types WHERE name = ? AND charm = ?"
	insertStmt, err := db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	stmt.Exec(st.Name, st.Charm)
	tx.Commit()

	return nil
}
