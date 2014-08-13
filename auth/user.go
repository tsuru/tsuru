// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	stderrors "errors"
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/validation"
	"gopkg.in/mgo.v2/bson"
)

const (
	emailError = "Invalid email."
)

var ErrUserNotFound = stderrors.New("User not found")

type Key struct {
	Name    string
	Content string
}

type User struct {
	Email    string
	Password string
	Keys     []Key
	quota.Quota
}

// keyToMap converts a Key array into a map maybe we should store a map
// directly instead of having a convertion
func keyToMap(keys []Key) map[string]string {
	keysMap := make(map[string]string, len(keys))
	for _, k := range keys {
		keysMap[k.Name] = k.Content
	}
	return keysMap
}

func GetUserByEmail(email string) (*User, error) {
	if !validation.ValidateEmail(email) {
		return nil, &errors.ValidationError{Message: emailError}
	}
	var u User
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": email}).One(&u)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return &u, nil
}

func (u *User) Create() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	if u.Quota.Limit == 0 {
		u.Quota = quota.Unlimited
		if limit, err := config.GetInt("quota:apps-per-user"); err == nil && limit > -1 {
			u.Quota.Limit = limit
		}
	}
	defer conn.Close()
	return conn.Users().Insert(u)
}

func (u *User) Delete() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Users().Remove(bson.M{"email": u.Email})
}

func (u *User) Update() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Users().Update(bson.M{"email": u.Email}, u)
}

// Teams returns a slice containing all teams that the user is member of.
func (u *User) Teams() ([]Team, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var teams []Team
	err = conn.Teams().Find(bson.M{"users": u.Email}).All(&teams)
	if err != nil {
		return nil, err
	}
	return teams, nil
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
	if index < 0 {
		return stderrors.New("Key not found")
	}
	copy(u.Keys[index:], u.Keys[index+1:])
	u.Keys = u.Keys[:len(u.Keys)-1]
	return nil
}

func (u *User) AddKeyGandalf(key *Key) error {
	key.Name = fmt.Sprintf("%s-%d", u.Email, len(u.Keys)+1)
	gURL := repository.ServerURL()
	if err := (&gandalf.Client{Endpoint: gURL}).AddKey(u.Email, keyToMap([]Key{*key})); err != nil {
		return fmt.Errorf("Failed to add key to git server: %s", err)
	}
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
		return nil, err
	}
	teamNames := GetTeamsNames(teams)
	q := bson.M{"teams": bson.M{"$in": teamNames}}
	if err := conn.Apps().Find(q).Select(bson.M{"name": 1}).All(&alwdApps); err != nil {
		return nil, err
	}
	appNames := make([]string, len(alwdApps))
	for i, v := range alwdApps {
		appNames[i] = v["name"]
	}
	return appNames, nil
}

func (u *User) ListKeys() (map[string]string, error) {
	gURL := repository.ServerURL()
	c := gandalf.Client{Endpoint: gURL}
	return c.ListKeys(u.Email)
}

func (u *User) CreateOnGandalf() error {
	gURL := repository.ServerURL()
	c := gandalf.Client{Endpoint: gURL}
	if _, err := c.NewUser(u.Email, keyToMap(u.Keys)); err != nil {
		return fmt.Errorf("Failed to create user in the git server: %s", err)
	}
	return nil
}
