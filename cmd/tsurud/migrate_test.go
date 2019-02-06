// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	docker "github.com/fsouza/go-dockerclient"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	check "gopkg.in/check.v1"
)

func (s *S) TestMigrateBSEnvs(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	entries, err := nodecontainer.LoadNodeContainersForPools(nodecontainer.BsDefaultName)
	c.Assert(err, check.Equals, nodecontainer.ErrNodeContainerNotFound)
	c.Assert(entries, check.DeepEquals, map[string]nodecontainer.NodeContainerConfig(nil))
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
	entries, err = nodecontainer.LoadNodeContainersForPools(nodecontainer.BsDefaultName)
	c.Assert(err, check.IsNil)
	defaultEntry := entries[""]
	c.Assert(defaultEntry.Config.Env, check.HasLen, 5)
	c.Assert(defaultEntry.Config.Env[0], check.Matches, `TSURU_TOKEN=\w{40}`)
	defaultEntry.Config.Env = defaultEntry.Config.Env[1:]
	entries[""] = defaultEntry
	expected := map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "big-sibling", PinnedImage: "tsuru/bs@shacabum", Config: docker.Config{
			Image: "tsuru/bs:v1",
			Env: []string{
				"TSURU_ENDPOINT=http://tsuru.server:8080/",
				"HOST_PROC=/prochost",
				"SYSLOG_LISTEN_ADDRESS=udp://0.0.0.0:1514",
				"FOO=1",
			},
		}, HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			NetworkMode:   "host",
			Binds:         []string{"/proc:/prochost:ro"},
		}},
	}
	c.Assert(entries, check.DeepEquals, expected)
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
	entries, err = nodecontainer.LoadNodeContainersForPoolsMerge(nodecontainer.BsDefaultName, true)
	c.Assert(err, check.IsNil)
	for k, v := range entries {
		v.Config.Env = v.Config.Env[1:]
		entries[k] = v
	}
	expectedBase := expected[""]
	expectedP1 := expectedBase
	expectedP2 := expectedBase
	expectedP3 := expectedBase
	expectedBase.Config.Env = append(expectedBase.Config.Env, "FOO=1")
	baseEnvs := append([]string{}, expectedBase.Config.Env...)
	expectedP1.Config.Env = append(baseEnvs, "A=x")
	expectedP2.Config.Env = append(baseEnvs, "A=y")
	expectedP3.Config.Env = append(baseEnvs, "B=z", "FOO=2")
	c.Assert(entries[""], check.DeepEquals, expectedBase)
	c.Assert(entries["p1"], check.DeepEquals, expectedP1)
	c.Assert(entries["p2"], check.DeepEquals, expectedP2)
	c.Assert(entries["p3"], check.DeepEquals, expectedP3)
}
