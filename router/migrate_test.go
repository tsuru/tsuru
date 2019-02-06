// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"github.com/globalsign/mgo/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestMigrateUniqueCollectionAllFixed(c *check.C) {
	coll := s.conn.Collection("routers")
	coll.DropIndex("app")
	toInsertApps := []struct {
		name string
		ip   string
	}{
		{"a1", "a1.com"},
		{"a3", "a3.com"},
		{"a5", "a5.com"},
		{"a6", "a6.com"},
		{"a8", "a8-2.com"},
		{"a8-2", "a8.com"},
		{"a9", "a9.com"},
	}
	appColl := s.conn.Apps()
	for _, a := range toInsertApps {
		err := appColl.Insert(bson.M{"_id": a.name, "name": a.name, "ip": a.ip})
		c.Assert(err, check.IsNil)
	}
	entries := []routerAppEntry{
		{App: "a1", Router: "r1"},
		{App: "a1", Router: "r1"},
		{App: "a3", Router: "r3"},
		{App: "a4", Router: "r3"},
		{App: "a5", Router: "r3"},
		{App: "a8", Router: "a8-2"},
		{App: "a8", Router: "a8"},
		{App: "a8-2", Router: "a8-2"},
		{App: "a8-2", Router: "a8"},
		{App: "ax", Router: "y"},
		{App: "ax", Router: "z"},
		{App: "a9", Router: "a9"},
		{App: "a9", Router: "a9"},
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
		{App: "a8", Router: "a8-2"},
		{App: "a8-2", Router: "a8"},
		{App: "a9", Router: "a9"},
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
	toInsertApps := []struct {
		name string
		ip   string
	}{
		{"a1", "a1.com"},
		{"a3", "a3.com"},
		{"a5", "a5.com"},
		{"a6", "a6.com"},
		{"a7", "xxx.com"},
		{"a8", "a8-2.com"},
		{"a8-2", "a8.com"},
		{"a9", "a9.com"},
	}
	appColl := s.conn.Apps()
	for _, a := range toInsertApps {
		err := appColl.Insert(bson.M{"_id": a.name, "name": a.name, "ip": a.ip})
		c.Assert(err, check.IsNil)
	}
	entries := []routerAppEntry{
		{App: "a1", Router: "r1"},
		{App: "a1", Router: "r1"},
		{App: "a3", Router: "r3"},
		{App: "a4", Router: "r3"},
		{App: "a5", Router: "r3"},
		{App: "a7", Router: "x1"},
		{App: "a7", Router: "x2"},
		{App: "a8", Router: "a8-2"},
		{App: "a8", Router: "a8"},
		{App: "a8-2", Router: "a8-2"},
		{App: "a8-2", Router: "a8"},
		{App: "ax", Router: "y"},
		{App: "ax", Router: "z"},
		{App: "a9", Router: "a9"},
		{App: "a9", Router: "a9"},
	}
	for _, e := range entries {
		err := coll.Insert(e)
		c.Assert(err, check.IsNil)
	}
	err := MigrateUniqueCollection()
	c.Assert(err, check.ErrorMatches, `(?s)ERROR.*app "a7".*`)
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
		{App: "a7", Router: "x1"},
		{App: "a7", Router: "x2"},
		{App: "a8", Router: "a8-2"},
		{App: "a8-2", Router: "a8"},
		{App: "a9", Router: "a9"},
	})
}
