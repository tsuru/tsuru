// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto"
	"crypto/rand"
	_ "crypto/sha256"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/validation"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrUserNotFound = stderrors.New("user not found")
	ErrInvalidKey   = stderrors.New("invalid key")
)

type User struct {
	Email    string
	Password string
	quota.Quota
	APIKey string
}

// ListUsers list all users registred in tsuru
func ListUsers() ([]User, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var users []User
	err = conn.Users().Find(nil).All(&users)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func GetUserByEmail(email string) (*User, error) {
	if !validation.ValidateEmail(email) {
		return nil, &errors.ValidationError{Message: "invalid email"}
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
	defer conn.Close()
	if u.Quota.Limit == 0 {
		u.Quota = quota.Unlimited
		if limit, err := config.GetInt("quota:apps-per-user"); err == nil && limit > -1 {
			u.Quota.Limit = limit
		}
	}
	err = conn.Users().Insert(u)
	if err != nil {
		return err
	}
	err = u.createOnRepositoryManager()
	if err != nil {
		u.Delete()
	}
	return err
}

func (u *User) Delete() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Users().Remove(bson.M{"email": u.Email})
	if err != nil {
		log.Errorf("failed to remove user %q from the database: %s", u.Email, err)
	}
	err = repository.Manager().RemoveUser(u.Email)
	if err != nil {
		log.Errorf("failed to remove user %q from the repository manager: %s", u.Email, err)
	}
	return nil
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

func (u *User) AddKey(key repository.Key) error {
	if key.Name == "" {
		return ErrInvalidKey
	}
	return repository.Manager().(repository.KeyRepositoryManager).AddKey(u.Email, key)
}

func (u *User) RemoveKey(key repository.Key) error {
	err := repository.Manager().(repository.KeyRepositoryManager).RemoveKey(u.Email, key)
	if err != nil {
		return err
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
	keys, err := repository.Manager().(repository.KeyRepositoryManager).ListKeys(u.Email)
	if err != nil {
		return nil, err
	}
	keysMap := make(map[string]string, len(keys))
	for _, key := range keys {
		keysMap[key.Name] = key.Body
	}
	return keysMap, nil
}

func (u *User) createOnRepositoryManager() error {
	err := repository.Manager().CreateUser(u.Email)
	if err != nil {
		return err
	}
	return nil
}

func (u *User) ShowAPIKey() (string, error) {
	if u.APIKey == "" {
		u.RegenerateAPIKey()
	}
	return u.APIKey, u.Update()
}

func (u *User) RegenerateAPIKey() (string, error) {
	random_byte := make([]byte, 32)
	_, err := rand.Read(random_byte)
	if err != nil {
		return "", err
	}
	h := crypto.SHA256.New()
	h.Write([]byte(u.Email))
	h.Write(random_byte)
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	u.APIKey = fmt.Sprintf("%x", h.Sum(nil))
	return u.APIKey, u.Update()
}
