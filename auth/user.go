// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto"
	"crypto/rand"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/validation"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrUserNotFound      = stderrors.New("user not found")
	ErrUserAlreadyHasKey = stderrors.New("user already has this key")
	ErrKeyNotFound       = stderrors.New("key not found")
)

type Key struct {
	Name    string
	Content string
}

type User struct {
	Email    string
	Password string
	Keys     []Key
	quota.Quota
	APIKey string
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
		return nil, &errors.ValidationError{Message: "Invalid email."}
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
		if k.Name == key.Name || k.Content == key.Content {
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
	if key.Name == "" {
		key.Name = fmt.Sprintf("%s-%d", u.Email, len(u.Keys)+1)
	}
	if u.HasKey(key) {
		return ErrUserAlreadyHasKey
	}
	actions := []*action.Action{
		&addKeyInGandalfAction,
		&addKeyInDatabaseAction,
	}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(&key, u)
}

func (u *User) addKeyGandalf(key *Key) error {
	serverURL, err := repository.ServerURL()
	if err != nil {
		return err
	}
	gandalfClient := gandalf.Client{Endpoint: serverURL}
	if err := gandalfClient.AddKey(u.Email, keyToMap([]Key{*key})); err != nil {
		return fmt.Errorf("Failed to add key to git server: %s", err)
	}
	return nil
}

func (u *User) addKeyDB(key *Key) error {
	u.Keys = append(u.Keys, *key)
	return u.Update()
}

func (u *User) RemoveKey(key Key) error {
	actualKey, index := u.FindKey(key)
	if index < 0 {
		return ErrKeyNotFound
	}
	err := u.removeKeyGandalf(&actualKey)
	if err != nil {
		return err
	}
	copy(u.Keys[index:], u.Keys[index+1:])
	u.Keys = u.Keys[:len(u.Keys)-1]
	return u.Update()
}

func (u *User) removeKeyGandalf(key *Key) error {
	serverURL, err := repository.ServerURL()
	if err != nil {
		return err
	}
	gandalfClient := gandalf.Client{Endpoint: serverURL}
	if err := gandalfClient.RemoveKey(u.Email, key.Name); err != nil {
		return fmt.Errorf("Failed to remove the key from git server: %s", err)
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
	gURL, err := repository.ServerURL()
	if err != nil {
		return nil, err
	}
	c := gandalf.Client{Endpoint: gURL}
	return c.ListKeys(u.Email)
}

func (u *User) CreateOnGandalf() error {
	gURL, err := repository.ServerURL()
	if err != nil {
		return err
	}
	c := gandalf.Client{Endpoint: gURL}
	if _, err := c.NewUser(u.Email, keyToMap(u.Keys)); err != nil {
		return fmt.Errorf("Failed to create user in the git server: %s", err)
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
