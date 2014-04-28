// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestMoveContainers(c *gocheck.C) {
	cluster, nodes, err := s.startMultipleServersCluster()
	defer s.stopMultipleServersCluster(cluster, nodes)
	err = newImage("tsuru/python", s.server.URL())
	c.Assert(err, gocheck.IsNil)
	appInstance := testing.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	app.Provisioner = &p
	coll := collection()
	defer coll.Close()
	coll.Insert(container{ID: "container-id", AppName: appInstance.GetName(), Version: "container-version", Image: "tsuru/python"})
	defer coll.Remove(bson.M{"appname": appInstance.GetName()})
	units, err := addUnitsWithHost(appInstance, 2, "serverAddr1")
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name:     appInstance.GetName(),
		Platform: appInstance.GetPlatform(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, gocheck.IsNil)
	err = conn.Apps().Update(
		bson.M{"name": appStruct.Name},
		bson.M{"$set": bson.M{"units": units}},
	)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	err = moveContainers("serverAddr1", "serverAddr0", encoder)
	c.Assert(err, gocheck.IsNil)
	containers, err := listContainersByHost("serverAddr1")
	c.Assert(len(containers), gocheck.Equals, 0)
	containers, err = listContainersByHost("serverAddr0")
	c.Assert(len(containers), gocheck.Equals, 2)
	q, err := getQueue()
	c.Assert(err, gocheck.IsNil)
	for _ = range containers {
		_, err := q.Get(1e6)
		c.Assert(err, gocheck.IsNil)
		_, err = q.Get(1e6)
		c.Assert(err, gocheck.IsNil)
	}
}
