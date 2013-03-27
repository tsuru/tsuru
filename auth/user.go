// Copyright 2013 tsuru authors. All rights reserved.
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

const defaultExpiration = 7 * 24 * time.Hour

var salt, tokenKey string
var tokenExpire time.Duration

func loadConfig() error {
	if salt == "" && tokenKey == "" {
		var err error
		if salt, err = config.GetString("auth:salt"); err != nil {
			return errors.New(`Setting "auth:salt" is undefined.`)
		}
		if iface, err := config.Get("auth:token-expire-days"); err == nil {
			day := int64(iface.(int))
			tokenExpire = time.Duration(day * 24 * int64(time.Hour))
		} else {
			tokenExpire = defaultExpiration
		}
		if tokenKey, err = config.GetString("auth:token-key"); err != nil {
			return errors.New(`Setting "auth:token-key" is undefined.`)
		}
	}
	return nil
}

func hashPassword(password string) string {
	err := loadConfig()
	if err != nil {
		panic(err)
	}
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
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	u := new(User)
	query := bson.M{"tokens.token": token}
	err = conn.Users().Find(query).One(&u)
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
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	u.HashPassword()
	return conn.Users().Insert(u)
}

func (u *User) Update() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Users().Update(bson.M{"email": u.Email}, u)
}

func (u *User) HashPassword() {
	u.Password = hashPassword(u.Password)
}

func (u *User) Get() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Users().Find(bson.M{"email": u.Email}).One(&u)
}

func (u *User) CheckPassword(password string) bool {
	hashedPassword := hashPassword(password)
	return u.Password == hashedPassword
}

func (u *User) CreateToken() (*Token, error) {
	if u.Email == "" {
		return nil, errors.New("User does not have an email")
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	t, _ := newToken(u)
	u.Tokens = append(u.Tokens, *t)
	err = conn.Users().Update(bson.M{"email": u.Email}, u)
	return t, err
}

// Teams returns a slice containing all teams that the user is member of.
func (u *User) Teams() (teams []Team, err error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Teams().Find(bson.M{"users": u.Email}).All(&teams)
	return
}

func (u *User) FindKey(key Key) (Key, int) {
	for i, k := range u.Keys {
		if k.Content == key.Content {
			return k, i
		}
	}
	return Key{}, -1
}

func (u *User) HasKey(key Key) bool {
	_, index := u.FindKey(key)
	return index > -1
}

func (u *User) AddKey(key Key) error {
	u.Keys = append(u.Keys, key)
	return nil
}

func (u *User) RemoveKey(key Key) error {
	_, index := u.FindKey(key)
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

func (u *User) AllowedApps() ([]string, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var alwdApps []map[string]string
	teams, err := u.Teams()
	if err != nil {
		return []string{}, err
	}
	teamNames := GetTeamsNames(teams)
	q := bson.M{"teams": bson.M{"$in": teamNames}}
	if err := conn.Apps().Find(q).Select(bson.M{"name": 1}).All(&alwdApps); err != nil {
		return []string{}, err
	}
	appNames := make([]string, len(alwdApps))
	for i, v := range alwdApps {
		appNames[i] = v["name"]
	}
	return appNames, nil
}

func (u *User) AllowedAppsByTeam(team string) ([]string, error) {
	conn, err := db.Conn()
	if err != nil {
		return []string{}, err
	}
	defer conn.Close()
	alwdApps := []map[string]string{}
	if err := conn.Apps().Find(bson.M{"teams": bson.M{"$in": []string{team}}}).Select(bson.M{"name": 1}).All(&alwdApps); err != nil {
		return []string{}, err
	}
	appNames := make([]string, len(alwdApps))
	for i, v := range alwdApps {
		appNames[i] = v["name"]
	}
	return appNames, nil
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
	if err := loadConfig(); err != nil {
		return nil, err
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
