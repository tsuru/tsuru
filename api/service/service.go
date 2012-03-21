package service

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/unit"
)

type Service struct {
	AppId           int
	Name            string
}

func (s *Service) Create() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "INSERT INTO service (app_id, name) VALUES (?, ?)"
	insertStmt, err := db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	stmt.Exec(s.AppId, s.Name)
	tx.Commit()

	u := unit.Unit{Name: s.Name, Type: "mysql"}
	err = u.Create()

	return err
}
