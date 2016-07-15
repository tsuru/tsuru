// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package user

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/tsuru/gandalf/db"
	"github.com/tsuru/gandalf/repository"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrUserNotFound      = errors.New("user not found")

	userNameRegexp = regexp.MustCompile(`\s|[^aA-zZ0-9-+.@]|(^$)`)
)

type User struct {
	Name string `bson:"_id"`
}

// Creates a new user and write his/her keys into authorized_keys file.
//
// The authorized_keys file belongs to the user running the process.
func New(name string, keys map[string]string) (*User, error) {
	log.Debugf(`Creating user "%s"`, name)
	u := &User{Name: name}
	if v, err := u.isValid(); !v {
		log.Errorf("user.New: %s", err.Error())
		return u, err
	}
	conn, err := db.Conn()
	if err != nil {
		addr, _ := db.DbConfig()
		return nil, errors.New(fmt.Sprintf("Failed to connect to MongoDB of Gandalf %q - %s.", addr, err.Error()))
	}
	defer conn.Close()
	if err := conn.User().Insert(&u); err != nil {
		if mgo.IsDup(err) {
			return nil, ErrUserAlreadyExists
		}
		log.Errorf("user.New: %s", err)
		return nil, err
	}
	return u, addKeys(keys, u.Name)
}

func (u *User) isValid() (isValid bool, err error) {
	if userNameRegexp.MatchString(u.Name) {
		return false, &InvalidUserError{message: "username is not valid"}
	}
	return true, nil
}

// Removes a user.
// Also removes it's associated keys from authorized_keys and repositories
// It handles user with repositories specially when:
// - a user has at least one repository:
//     - if he/she is the only one with access to the repository, the removal will stop and return an error
//     - if there are more than one user with access to the repository, gandalf will first revoke user's access and then remove the user permanently
// - a user has no repositories: gandalf will simply remove the user
func Remove(name string) error {
	var u *User
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.User().Find(bson.M{"_id": name}).One(&u); err != nil {
		if err == mgo.ErrNotFound {
			return ErrUserNotFound
		}
		return err
	}
	if err := u.handleAssociatedRepositories(); err != nil {
		return err
	}
	if err := conn.User().RemoveId(u.Name); err != nil {
		return fmt.Errorf("Could not remove user: %s", err.Error())
	}
	return removeUserKeys(u.Name)
}

func (u *User) handleAssociatedRepositories() error {
	var repos []repository.Repository
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.Repository().Find(bson.M{"users": u.Name}).All(&repos); err != nil {
		return err
	}
	for _, r := range repos {
		if len(r.Users) == 1 {
			return errors.New("Could not remove user: user is the only one with access to at least one of it's repositories")
		}
	}
	for _, r := range repos {
		for i, v := range r.Users {
			if v == u.Name {
				r.Users[i], r.Users = r.Users[len(r.Users)-1], r.Users[:len(r.Users)-1]
				if err := conn.Repository().Update(bson.M{"_id": r.Name}, r); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

// AddKey adds new SSH keys to the list of user keys for the provided username.
//
// Returns an error in case the user does not exist.
func AddKey(username string, k map[string]string) error {
	var u User
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.User().FindId(username).One(&u); err != nil {
		return ErrUserNotFound
	}
	return addKeys(k, u.Name)
}

// UpdateKey updates the content of the given key.
func UpdateKey(username string, k Key) error {
	var u User
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.User().FindId(username).One(&u); err != nil {
		return ErrUserNotFound
	}
	return updateKey(k.Name, k.Body, u.Name)
}

// RemoveKey removes the key from the database and from authorized_keys file.
//
// If the user or the key is not found, returns an error.
func RemoveKey(username, keyname string) error {
	var u User
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.User().FindId(username).One(&u); err != nil {
		return ErrUserNotFound
	}
	return removeKey(keyname, username)
}

type InvalidUserError struct {
	message string
}

func (err *InvalidUserError) Error() string {
	return err.message
}
