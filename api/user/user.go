package user

import (
	"crypto/sha512"
	"fmt"
	"github.com/timeredbull/tsuru/database"
)

const (
	SALT = "tsuru-salt"
)

type User struct {
	Id       int
	Email    string
	Password string
}

func (u *User) Create() error {
	u.hashPassword()
	_, err := database.Db.Exec("INSERT INTO users (email, password) VALUES (?, ?)", u.Email, u.Password)
	return err
}

func (u *User) hashPassword() {
	half := len(u.Password)/2
	b := []byte(u.Password[:half])
	b = append(b, []byte(SALT)...)
	b = append(b, []byte(u.Password[half:])...)
	hash := sha512.New()
	hash.Write(b)
	u.Password = fmt.Sprintf("%x", hash.Sum(nil))
}

func (u *User) Get() error {
	var field string
	var args = make([]interface{}, 1)
	if u.Id > 0 {
		field = "id"
		args[0] = u.Id
	} else {
		field = "email"
		args[0] = u.Email
	}
	row := database.Db.QueryRow(fmt.Sprintf("SELECT id, email, password FROM users WHERE %s = ?", field), args...)
	err := row.Scan(&u.Id, &u.Email, &u.Password)
	return err
}
