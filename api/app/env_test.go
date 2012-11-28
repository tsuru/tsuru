// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/fs/testing"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"os"
	"path"
)

func (s *S) TestRewriteEnvMessage(c *C) {
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{
		Name:  "time",
		Teams: []string{s.team.Name},
		Units: []Unit{
			{AgentState: "started", MachineAgentState: "running", InstanceState: "running", Machine: 1},
		},
	}
	msg := message{
		app:     &app,
		success: make(chan bool),
	}
	env <- msg
	c.Assert(<-msg.success, Equals, true)
	c.Assert(commandmocker.Ran(dir), Equals, true)
}

func (s *S) TestDoesNotSendInTheSuccessChannelIfItIsNil(c *C) {
	defer func() {
		r := recover()
		c.Assert(r, IsNil)
	}()
	dir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{
		Name:      "rainmaker",
		Framework: "",
		Teams:     []string{s.team.Name},
		Units: []Unit{
			{Machine: 1},
		},
	}
	err = db.Session.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	msg := message{
		app: &app,
	}
	env <- msg
}

func (s *S) TestEnvironConfPath(c *C) {
	expected := path.Join(os.ExpandEnv("${HOME}"), ".juju", "environments.yaml")
	c.Assert(environConfPath, Equals, expected)
}

func (s *S) TestFileSystem(c *C) {
	fsystem = &testing.RecordingFs{}
	c.Assert(filesystem(), DeepEquals, fsystem)
	fsystem = nil
	c.Assert(filesystem(), DeepEquals, fs.OsFs{})
	fsystem = s.rfs
}
