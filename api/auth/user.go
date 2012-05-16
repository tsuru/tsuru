package auth

import (
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha512"
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"launchpad.net/mgo/bson"
	"time"
)

const (
	defaultSalt       = "tsuru-salt"
	defaultExpiration = 7 * 24 * time.Hour
	defaultKey        = "tsuru-key"
)

var salt, tokenKey string
var tokenExpire time.Duration

func init() {
	loadConfig()
}

func loadConfig() {
	var err error
	if salt, err = config.GetString("auth:salt"); err != nil {
		salt = defaultSalt
	}
	if iface, err := config.Get("auth:token-expire-days"); err == nil {
		day := int64(iface.(int))
		tokenExpire = time.Duration(day * 24 * int64(time.Hour))
	} else {
		tokenExpire = defaultExpiration
	}
	if tokenKey, err = config.GetString("auth:token-key"); err != nil {
		tokenKey = defaultKey
	}
}

func hashPassword(password string) string {
	salt := []byte(salt)
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

type Key struct {
	Name    string
	Content string
}

type User struct {
	Email    string
	Password string
	Tokens   []Token
	Keys     []Key
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

func (u *User) findKey(key Key) (Key, int) {
	for i, k := range u.Keys {
		if k.Content == key.Content {
			return k, i
		}
	}
	return Key{}, -1
}

func (u *User) hasKey(key Key) bool {
	_, index := u.findKey(key)
	return index > -1
}

func (u *User) addKey(key Key) error {
	u.Keys = append(u.Keys, key)
	return nil
}

func (u *User) removeKey(key Key) error {
	_, index := u.findKey(key)
	last := len(u.Keys) - 1
	u.Keys[index] = u.Keys[last]
	u.Keys = u.Keys[:last]
	return nil
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
	h.Write([]byte(tokenKey))
	h.Write([]byte(time.Now().Format(time.UnixDate)))
	t := Token{}
	t.ValidUntil = time.Now().Add(tokenExpire)
	t.Token = fmt.Sprintf("%x", h.Sum(nil))
	return &t, nil
}

func CheckToken(token string) (*User, error) {
	if token == "" {
		return nil, errors.New("You must provide the token")
	}
	u, err := GetUserByToken(token)
	if err != nil {
		return nil, errors.New("Invalid token")
	}
	return u, nil
}
