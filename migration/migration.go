// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package migration provides a "micro-framework" for migration management:
// each migration is a simple function that returns an error. All migration
// functions are executed in the order they were registered.
package migration

import (
	"errors"
	"fmt"
	"io"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/mgo.v2/bson"
)

// ErrDuplicateMigration is the error returned by Register when the given name
// is already in use.
var ErrDuplicateMigration = errors.New("there's already a migration with this name")

// MigrateFunc represents a migration function, that can be registered with the
// Register function. Migrations are later ran in the registration order, and
// this package keeps track of which migrate have ran already.
type MigrateFunc func() error

type migration struct {
	Name string
	Ran  bool
	fn   MigrateFunc
}

var migrations []migration

// Register register a new migration for later execution with the Run
// functions.
func Register(name string, fn MigrateFunc) error {
	for _, m := range migrations {
		if m.Name == name {
			return ErrDuplicateMigration
		}
	}
	migrations = append(migrations, migration{Name: name, fn: fn})
	return nil
}

// Run runs all registered migrations. Migrations are executed in the order
// that they were registered.
func Run(w io.Writer, dry bool) error {
	migrations, err := getMigrations()
	if err != nil {
		return err
	}
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	for _, m := range migrations {
		fmt.Fprintf(w, "Running %q... ", m.Name)
		if !dry {
			err := m.fn()
			if err != nil {
				return err
			}
			m.Ran = true
			err = coll.Insert(m)
			if err != nil {
				return err
			}
		}
		fmt.Fprintln(w, "OK")
	}
	return nil
}

func getMigrations() ([]migration, error) {
	coll, err := collection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	result := make([]migration, 0, len(migrations))
	names := make([]string, len(migrations))
	for i, m := range migrations {
		names[i] = m.Name
	}
	query := bson.M{"name": bson.M{"$in": names}, "ran": true}
	var ran []migration
	err = coll.Find(query).All(&ran)
	if err != nil {
		return nil, err
	}
	for _, m := range migrations {
		var found bool
		for _, r := range ran {
			if r.Name == m.Name {
				found = true
				break
			}
		}
		if !found {
			result = append(result, m)
		}
	}
	return result, nil
}

func collection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Collection("migrations"), nil
}
