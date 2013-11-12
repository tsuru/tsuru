// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package quota implements per-user/app quota management.
//
// It has a Usage type, that is used to manage generic quotas, and functions
// and methods to interact with the Usage type.
package quota

import (
	"errors"
	"fmt"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/safe"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

var (
	ErrQuotaAlreadyExists = errors.New("Quota already exists")
	ErrQuotaNotFound      = errors.New("Quota not found")
)

var locker = safe.MultiLocker()

// Usage represents the usage of a user/app. It contains information about the
// limit of items, and the current amount of items in use by the user.
type usage struct {
	// Owner identifier (e.g.: the email).
	Owner string
	// Slice of items, each identified by a string.
	Items []string
	// Maximum length of Items.
	Limit uint
}

// Create stores a new quota in the database.
func Create(owner string, quota uint) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Quota().Insert(usage{Owner: owner, Limit: quota})
	if e, ok := err.(*mgo.LastError); ok && e.Code == 11000 {
		return ErrQuotaAlreadyExists
	}
	return err
}

// Delete destroys the quota allocated for the owner.
func Delete(owner string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	q := map[string]interface{}{"owner": owner}
	err = conn.Quota().Remove(q)
	if err != nil && err.Error() == "not found" {
		return ErrQuotaNotFound
	}
	return err
}

// Reserve reserves the given items to the owner.
//
// It may allocate part of the items before exceeding the quota. See the
// example for more details.
func Reserve(owner string, items ...string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	locker.Lock(owner)
	defer locker.Unlock(owner)
	var u usage
	err = conn.Quota().Find(bson.M{"owner": owner}).One(&u)
	if err != nil {
		return ErrQuotaNotFound
	}
	available := int(u.Limit) - len(u.Items)
	if available < 0 {
		available = 0
	}
	if available < len(items) {
		return &QuotaExceededError{Requested: uint(len(items)), Available: uint(available)}
	}
	update := bson.M{"$addToSet": bson.M{"items": bson.M{"$each": items}}}
	err = conn.Quota().Update(bson.M{"owner": owner}, update)
	if err != nil {
		return err
	}
	return nil
}

// Release releases the given items from the owner.
//
// It returns an error when the given owner does not exist, but returns nil
// when any of the given items do not not belong to the owner.
func Release(owner string, items ...string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	update := bson.M{"$pullAll": bson.M{"items": items}}
	err = conn.Quota().Update(bson.M{"owner": owner}, update)
	if err != nil && err.Error() == "not found" {
		return ErrQuotaNotFound
	}
	return err
}

// Set defines a new value for the quota of the given owner.
//
// It allows the database to become in an inconsistent state: a owner may be
// able to have 8 items, and a limit of 7. See the example for more details.
func Set(owner string, value uint) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	update := bson.M{"$set": bson.M{"limit": value}}
	err = conn.Quota().Update(bson.M{"owner": owner}, update)
	if err != nil && err.Error() == "not found" {
		return ErrQuotaNotFound
	}
	return err
}

// Items returns a slice containing all items allocated to the given owner, and
// an unsigned integer indicating how many items are still available.
func Items(owner string) ([]string, uint, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()
	var u usage
	err = conn.Quota().Find(bson.M{"owner": owner}).One(&u)
	if err != nil {
		return nil, 0, ErrQuotaNotFound
	}
	available := int(u.Limit) - len(u.Items)
	if available < 0 {
		available = 0
	}
	return u.Items, uint(available), nil
}

type QuotaExceededError struct {
	Requested uint
	Available uint
}

func (err *QuotaExceededError) Error() string {
	return fmt.Sprintf("Quota exceeded. Available: %d. Requested: %d.", err.Available, err.Requested)
}
