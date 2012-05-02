package auth

import (
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha512"
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/db"
	"launchpad.net/mgo/bson"
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

type Team struct {
	Name  string
	Users []*User
}

func (t *Team) ContainsUser(u *User) bool {
	for _, user := range t.Users {
		if u.Email == user.Email {
			return true
		}
	}
	return false
}

func (t *Team) AddUser(u *User) error {
	if t.ContainsUser(u) {
		return errors.New(fmt.Sprintf("User %s is alread in the team %s.", u.Email, t.Name))
	}
	t.Users = append(t.Users, u)
	return nil
}

func (t *Team) RemoveUser(u *User) error {
	index := -1
	for i, user := range t.Users {
		if u.Email == user.Email {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New(fmt.Sprintf("User %s is not in the team %s.", u.Email, t.Name))
	}
	last := len(t.Users) - 1
	t.Users[index] = t.Users[last]
	t.Users = t.Users[:last]
	return nil
}

type User struct {
	Email    string
	Password string
	Tokens   []Token
}

func GetUserByToken(token string) (*User, error) {
	c := db.Session.Users()
	u := new(User)
	query := bson.M{"tokens.token": token}
	err := c.Find(query).One(&u)

	if err != nil {
		return nil, errors.New("Token not found")
	}
	if u.Tokens[0].ValidUntil.Sub(time.Now()) < 1 {
		return nil, errors.New("Token has expired")
	}
	return u, nil
}

func (u *User) Create() error {
	u.hashPassword()
	return db.Session.Users().Insert(u)
}

func (u *User) hashPassword() {
	u.Password = hashPassword(u.Password)
}

func (u *User) Get() error {
	var filter = bson.M{}
	filter["email"] = u.Email
	return db.Session.Users().Find(filter).One(&u)
}

func (u *User) Login(password string) bool {
	hashedPassword := hashPassword(password)
	return u.Password == hashedPassword
}

func (u *User) CreateToken() (*Token, error) {
	if u.Email == "" {
		return nil, errors.New("User does not have an email")
	}
	t, _ := NewToken(u)
	u.Tokens = append(u.Tokens, *t)
	c := db.Session.Users()
	err := c.Update(bson.M{"email": u.Email}, u)
	return t, err
}

type Token struct {
	Token      string
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
	t := Token{}
	t.ValidUntil = time.Now().Add(TOKENEXPIRE)
	t.Token = fmt.Sprintf("%x", h.Sum(nil))
	return &t, nil
}
