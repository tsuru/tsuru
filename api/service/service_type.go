package service

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

type ServiceType struct {
	Id    int
	Name  string
	Charm string
}

func (st *ServiceType) Create() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "INSERT INTO service_type (id, name, charm) VALUES (?, ?, ?)"
	insertStmt, err := db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	stmt.Exec(st.Id, st.Name, st.Charm)
	tx.Commit()

	return nil
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
