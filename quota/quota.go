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
	"labix.org/v2/mgo/bson"
	"sync"
)

var (
	ErrQuotaAlreadyExists = errors.New("Quota already exists")
	ErrQuotaExceeded      = errors.New("Quota exceeded")
	ErrQuotaNotFound      = errors.New("Quota not found")
)

var locker = multiLocker{m: make(map[string]*sync.Mutex)}

// Usage represents the usage of a user. It contains information about the
// limit of items, and the current amount of items in use by the user.
type usage struct {
	// A unique identifier for the user (e.g.: the email).
	User string

	// The slice of items, each identified by a string.
	Items []string

	// The maximum length of Items.
	Limit uint
	mut   sync.Mutex
}

// Create stores a new quota in the database.
func Create(user string, quota uint) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Quota().Insert(usage{User: user, Limit: quota})
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

// Reserve increases the number of items in use for the user.
func Reserve(user, item string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	locker.Lock(user)
	defer locker.Unlock(user)
	var u usage
	err = conn.Quota().Find(bson.M{"user": user}).One(&u)
	if err != nil {
		return ErrQuotaNotFound
	}
	if uint(len(u.Items)) == u.Limit {
		return ErrQuotaExceeded
	}
	update := bson.M{"$addToSet": bson.M{"items": item}}
	return conn.Quota().Update(bson.M{"user": user}, update)
}
