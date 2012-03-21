package service

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/unit"
)

type ServiceBinding struct {
	ServiceConfigId int
	AppId           int
	UserId          int
	BindingTokenId  int
	Name            string
}

func (s *ServiceBinding) Create() error {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	query := "INSERT INTO service_bindings (service_config_id, app_id, user_id, binding_token_id, name) VALUES (?, ?, ?, ?, ?)"
	insertStmt, err := db.Prepare(query)
	if err != nil {
		panic(err)
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	stmt := tx.Stmt(insertStmt)
	stmt.Exec(s.ServiceConfigId, s.AppId, s.UserId, s.BindingTokenId, s.Name)
	tx.Commit()

	u := unit.Unit{Name: s.Name, Type: "mysql"}
	err = u.Create()

	return err
}
