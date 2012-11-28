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
	"sync"
	"time"
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

func (s *S) TestRunCmdInexistentApp(c *C) {
	dir, err := commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{Name: "rainmaker"}
	msg := message{app: &app}
	runCmd("ls -lh", msg, 1e6)
	c.Assert(commandmocker.Ran(dir), Equals, false)
}

func (s *S) TestRunCmdSavingTheMachineLater(c *C) {
	var wg sync.WaitGroup
	dir, err := commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	app := App{Name: "rainmaker"}
	err = db.Session.Apps().Insert(app)
	c.Assert(err, IsNil)
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-time.After(1e9 + 1e6)
		app.Units = []Unit{
			{
				Machine:           1,
				AgentState:        "started",
				MachineAgentState: "running",
				InstanceState:     "running",
			},
		}
		err = db.Session.Apps().Update(bson.M{"name": app.Name}, app)
		c.Assert(err, IsNil)
	}()
	msg := message{
		app:     &app,
		success: make(chan bool),
	}
	go runCmd("ls -lh", msg, 1e6)
	wg.Wait()
	c.Assert(<-msg.success, Equals, true)
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
