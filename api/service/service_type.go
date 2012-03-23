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
		query = "SELECT id, name, charm FROM service_type WHERE id = ?"
		rows, err = db.Query(query, st.Id)
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

func (st *ServiceType) Create() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "INSERT INTO service_type (name, charm) VALUES (?, ?)"
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

	query := "DELETE FROM service_type WHERE name = ? AND charm = ?"
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
