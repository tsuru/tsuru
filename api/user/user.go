package api

import (
	"github.com/timeredbull/tsuru/database"
)

type User struct {
	Id       int
	Email    string
	Password string
}

func (u *User) Create() error {
	stmt, _ := database.Db.Prepare("INSERT INTO users (email, password) VALUES (?, ?)")
	stmt.Exec(u.Email, u.Password)
	return nil
}
