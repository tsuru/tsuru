package user

import (
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha512"
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/database"
	"time"
)

const (
	SALT        = "tsuru-salt"
	TOKENEXPIRE = 7 * 24 * time.Hour
	TOKENKEY    = "tsuru-key"
)

func hashPassword(password string) string {
	salt := []byte(SALT)
	return fmt.Sprintf("%x", pbkdf2.Key([]byte(password), salt, 4096, len(salt)*8, sha512.New))
}

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
	u.Password = hashPassword(u.Password)
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

func (u *User) Login(password string) bool {
	hashedPassword := hashPassword(password)
	return u.Password == hashedPassword
}

type Token struct {
	U          *User
	Token      []byte
	ValidUntil time.Time
}

func NewToken(u *User) (*Token, error) {
	if u == nil {
		return nil, errors.New("User is nil")
	}
	if u.Email == "" {
		return nil, errors.New("Impossible to generate tokens for users without email")
	}
	h := sha512.New()
	h.Write([]byte(u.Email))
	h.Write([]byte(TOKENKEY))
	h.Write([]byte(time.Now().Format(time.UnixDate)))
	t := Token{U: u}
	t.ValidUntil = time.Now().Add(TOKENEXPIRE)
	t.Token = h.Sum(nil)
	return &t, nil
}

func (t *Token) Create() error {
	if t.U.Id < 1 {
		return errors.New("User does not have an id")
	}
	_, err := database.Db.Exec("INSERT INTO usertokens (user_id, token, valid_until) VALUES (?, ?, ?)", t.U.Id, fmt.Sprintf("%x", t.Token), t.ValidUntil.Format(time.UnixDate))
	return err
}

func GetUserByToken(token string) (*User, error) {
	var valid string
	u := new(User)
	row := database.Db.QueryRow("SELECT u.id, u.email, u.password, t.valid_until FROM users u INNER JOIN usertokens t ON t.user_id = u.id WHERE t.token = ?", token)
	err := row.Scan(&u.Id, &u.Email, &u.Password, &valid)
	if err != nil {
		return nil, errors.New("Token not found")
	}
	t, err := time.Parse(time.UnixDate, valid)
	if err != nil {
		return nil, err
	}
	if t.Sub(time.Now()) < 1 {
		return nil, errors.New("Token has expired")
	}
	return u, nil
}
