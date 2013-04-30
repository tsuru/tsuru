// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package quota implements per-user quota management.
//
// It has a Usage type, that is used to manage generic quotas, and functions
// and methods to interact with the Usage type.
package quota

import (
	"errors"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo"
)

var (
	ErrQuotaAlreadyExists = errors.New("Quota already exists")
	ErrQuotaNotFound      = errors.New("Quota not found")
)

var locker = multiLocker{m: make(map[string]*sync.Mutex)}

// Usage represents the usage of a user. It contains information about the
// limit of items, and the current amount of items in use by the user.
type Usage struct {
	// Identifier for the user (e.g.: the email).
	User string
	// Slice of items, each identified by a string.
	Items []string
	// Maximum length of Items.
	Limit uint
}

// Create stores a new quota in the database.
func Create(user string, quota uint) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Quota().Insert(Usage{User: user, Limit: quota})
	if e, ok := err.(*mgo.LastError); ok && e.Code == 11000 {
		return ErrQuotaAlreadyExists
	}
	return err
}

// Delete destroys the quota allocated for the user.
func Delete(user string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	q := map[string]interface{}{"user": user}
	err = conn.Quota().Remove(q)
	if err != nil && err.Error() == "not found" {
		return ErrQuotaNotFound
	}
	return err
}

// Get returns the quota instance of the given user.
func Get(user string) (*Usage, error) {
	var usage Usage
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	q := map[string]interface{}{"user": user}
	err = conn.Quota().Find(q).One(&usage)
	if err != nil && err.Error() == "not found" {
		return nil, ErrQuotaNotFound
	}
	return &usage, err
}
