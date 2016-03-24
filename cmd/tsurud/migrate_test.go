// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision/docker/bs"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestMigrateBSEnvs(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conf, err := bs.LoadConfig()
	c.Assert(err, check.IsNil)
	var entries map[string]bs.BSConfigEntry
	err = conf.LoadAll(&entries)
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]bs.BSConfigEntry{
		"": {},
	})
	coll := conn.Collection("bsconfig")
	err = coll.Insert(bson.M{
		"_id":   "bs",
		"image": "tsuru/bs@shacabum",
		"token": "999",
		"envs": []bson.M{
			{"name": "FOO", "value": "1"},
		},
		"pools": []bson.M{},
	})
	c.Assert(err, check.IsNil)
	err = migrateBSEnvs()
	c.Assert(err, check.IsNil)
	err = conf.LoadAll(&entries)
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]bs.BSConfigEntry{
		"": {Image: "tsuru/bs@shacabum", Token: "999", Envs: map[string]string{"FOO": "1"}},
	})
	err = migrateBSEnvs()
	c.Assert(err, check.IsNil)
	err = conf.LoadAll(&entries)
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]bs.BSConfigEntry{
		"": {Image: "tsuru/bs@shacabum", Token: "999", Envs: map[string]string{"FOO": "1"}},
	})
	err = coll.UpdateId("bs", bson.M{
		"$set": bson.M{"pools": []bson.M{
			{"name": "p1", "envs": []bson.M{{"name": "A", "value": "x"}}},
			{"name": "p2", "envs": []bson.M{{"name": "A", "value": "y"}}},
			{"name": "p3", "envs": []bson.M{{"name": "B", "value": "z"}, {"name": "FOO", "value": "2"}}},
		}},
	})
	c.Assert(err, check.IsNil)
	err = migrateBSEnvs()
	c.Assert(err, check.IsNil)
	err = conf.LoadAll(&entries)
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.DeepEquals, map[string]bs.BSConfigEntry{
		"":   {Image: "tsuru/bs@shacabum", Token: "999", Envs: map[string]string{"FOO": "1"}},
		"p1": {Image: "tsuru/bs@shacabum", Token: "999", Envs: map[string]string{"FOO": "1", "A": "x"}},
		"p2": {Image: "tsuru/bs@shacabum", Token: "999", Envs: map[string]string{"FOO": "1", "A": "y"}},
		"p3": {Image: "tsuru/bs@shacabum", Token: "999", Envs: map[string]string{"FOO": "2", "B": "z"}},
	})
}
