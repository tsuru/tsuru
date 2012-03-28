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
	_, err := database.Db.Exec("INSERT INTO users (email, password) VALUES (?, ?)", u.Email, u.Password)
	return err
}

func (u *User) Get() error {
	row := database.Db.QueryRow("SELECT email, password FROM users WHERE id = ?", u.Id)
	err := row.Scan(&u.Email, &u.Password)
	return err
}
