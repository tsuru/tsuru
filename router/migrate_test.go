// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestMigrateUniqueCollectionAllFixed(c *check.C) {
	coll := s.conn.Collection("routers")
	coll.DropIndex("app")
	toInsertApps := []string{"a1", "a3", "a5", "a6"}
	appColl := s.conn.Apps()
	for _, aName := range toInsertApps {
		err := appColl.Insert(bson.M{"_id": aName, "name": aName})
		c.Assert(err, check.IsNil)
	}
	entries := []routerAppEntry{
		{App: "a1", Router: "r1"},
		{App: "a1", Router: "r1"},
		{App: "a3", Router: "r3"},
		{App: "a4", Router: "r3"},
		{App: "a5", Router: "r3"},
	}
	for _, e := range entries {
		err := coll.Insert(e)
		c.Assert(err, check.IsNil)
	}
	err := MigrateUniqueCollection()
	c.Assert(err, check.IsNil)
	var dbEntries []routerAppEntry
	err = coll.Find(nil).Sort("app", "router").All(&dbEntries)
	c.Assert(err, check.IsNil)
	for i := range dbEntries {
		dbEntries[i].ID = ""
	}
	c.Assert(dbEntries, check.DeepEquals, []routerAppEntry{
		{App: "a1", Router: "r1"},
		{App: "a3", Router: "r3"},
		{App: "a5", Router: "r3"},
		{App: "a6", Router: "a6"},
	})
	collWithIdx, err := collection()
	c.Assert(err, check.IsNil)
	indexes, err := collWithIdx.Indexes()
	c.Assert(err, check.IsNil)
	c.Assert(indexes, check.HasLen, 2)
}

func (s *S) TestMigrateUniqueCollectionInvalid(c *check.C) {
	coll := s.conn.Collection("routers")
	coll.DropIndex("app")
	toInsertApps := []string{"a1", "a2", "a3", "a5", "a6"}
	appColl := s.conn.Apps()
	for _, aName := range toInsertApps {
		err := appColl.Insert(bson.M{"_id": aName, "name": aName})
		c.Assert(err, check.IsNil)
	}
	entries := []routerAppEntry{
		{App: "a1", Router: "r1"},
		{App: "a1", Router: "r1"},
		{App: "a2", Router: "r1"},
		{App: "a2", Router: "r2"},
		{App: "a2", Router: "r2"},
		{App: "a3", Router: "r3"},
		{App: "a4", Router: "r3"},
		{App: "a5", Router: "r3"},
	}
	for _, e := range entries {
		err := coll.Insert(e)
		c.Assert(err, check.IsNil)
	}
	err := MigrateUniqueCollection()
	c.Assert(err, check.ErrorMatches, `(?s)WARNING.*app "a2".*`)
	var dbEntries []routerAppEntry
	err = coll.Find(nil).Sort("app", "router").All(&dbEntries)
	c.Assert(err, check.IsNil)
	for i := range dbEntries {
		dbEntries[i].ID = ""
	}
	c.Assert(dbEntries, check.DeepEquals, []routerAppEntry{
		{App: "a1", Router: "r1"},
		{App: "a2", Router: "r1"},
		{App: "a2", Router: "r2"},
		{App: "a2", Router: "r2"},
		{App: "a3", Router: "r3"},
		{App: "a5", Router: "r3"},
		{App: "a6", Router: "a6"},
	})
}
