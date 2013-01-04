// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha512"
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
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
	var t Token
	for _, tk := range u.Tokens {
		if tk.Token == token {
			t = tk
			break
		}
	}
	if t.Token == "" || t.ValidUntil.Sub(time.Now()) < 1 {
		return nil, errors.New("Token has expired")
	}
	return u, nil
}

func (u *User) Create() error {
	u.hashPassword()
	return db.Session.Users().Insert(u)
}

func (u *User) update() error {
	return db.Session.Users().Update(bson.M{"email": u.Email}, u)
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
	t, _ := newToken(u)
	u.Tokens = append(u.Tokens, *t)
	c := db.Session.Users()
	err := c.Update(bson.M{"email": u.Email}, u)
	return t, err
}

// Teams returns a slice containing all teams that the user is member
func (u *User) Teams() (teams []Team, err error) {
	err = db.Session.Teams().Find(bson.M{"users": u.Email}).All(&teams)
	return
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
	copy(u.Keys[index:], u.Keys[index+1:])
	u.Keys = u.Keys[:len(u.Keys)-1]
	return nil
}

func (u *User) IsAdmin() bool {
	adminTeamName, err := config.GetString("admin-team")
	if err != nil {
		return false
	}
	teams, err := u.Teams()
	if err != nil {
		return false
	}
	for _, t := range teams {
		if t.Name == adminTeamName {
			return true
		}
	}
	return false
}

type Token struct {
	Token      string
	ValidUntil time.Time
}

func newToken(u *User) (*Token, error) {
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
