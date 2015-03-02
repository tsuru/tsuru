// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dbtest provides utilities test functions and types for interacting
// with the database during tests.
package dbtest

import (
	"strings"

	"gopkg.in/mgo.v2"
)

// ClearAllCollections removes all registers from all collections in the given
// Mongo database.
func ClearAllCollections(db *mgo.Database) error {
	return ClearAllCollectionsExcept(db, nil)
}

func ClearAllCollectionsExcept(db *mgo.Database, toKeep []string) error {
	colls, err := db.CollectionNames()
	if err != nil {
		return err
	}
	for _, collName := range colls {
		var coll *mgo.Collection
		if strings.Index(collName, "system.") != -1 {
			continue
		}
		for i := range toKeep {
			if collName == toKeep[i] {
				goto next
			}
		}
		coll = db.C(collName)
		_, err = coll.RemoveAll(nil)
		if err != nil {
			coll.DropCollection()
		}
	next:
	}
	return nil
}
