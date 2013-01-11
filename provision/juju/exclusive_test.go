// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"flag"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"time"
)

// This file contains tests that are not safe to run with other packages. To
// run these tests, use the flag -exclusive. For example:
//
//     % go test -exclusive

var exclusive = flag.Bool("exclusive", false, "Set to true to indicate that no other package tests are running.")

var _ = Suite(&ExclusiveSuite{})

type ExclusiveSuite struct {
	s S
}

func (s *ExclusiveSuite) SetUpSuite(c *C) {
	if !*exclusive {
		c.Skip("Not in exclusive mode.")
	}
	s.s.SetUpSuite(c)
	config.Set("queue-server", "127.0.0.1:11300")
}

func (s *ExclusiveSuite) TearDownSuite(c *C) {
	if !*exclusive {
		c.Skip("Not in exclusive mode.")
	}
	s.s.TearDownSuite(c)
}

func (s *ExclusiveSuite) TestCollectStatusIDChangeDisabledELB(c *C) {
	testing.CleanQueues(app.QueueName)
	s.s.TestCollectStatusIDChangeDisabledELB(c)
	msg, err := queue.Get(app.QueueName, 1e6)
	c.Assert(err, IsNil)
	defer msg.Delete()
	c.Assert(msg.Action, Equals, app.RegenerateApprcAndStart)
	c.Assert(msg.Args, DeepEquals, []string{"as_i_rise", "as_i_rise/0"})
}

func (s *ExclusiveSuite) TestCollectStatusIDChangeFromPending(c *C) {
	testing.CleanQueues(app.QueueName)
	tmpdir, err := commandmocker.Add("juju", collectOutput)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	p := JujuProvisioner{}
	err = p.unitsCollection().Insert(instance{UnitName: "as_i_rise/0", InstanceId: "pending"})
	c.Assert(err, IsNil)
	defer p.unitsCollection().Remove(bson.M{"_id": bson.M{"$in": []string{"as_i_rise/0", "the_infanta/0"}}})
	_, err = p.CollectStatus()
	c.Assert(err, IsNil)
	done := make(chan int8)
	go func() {
		for {
			q := bson.M{"_id": "as_i_rise/0", "instanceid": "i-00000439"}
			ct, err := p.unitsCollection().Find(q).Count()
			c.Assert(err, IsNil)
			if ct == 1 {
				done <- 1
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(2e9):
		c.Fatal("Did not update the unit after 2 seconds.")
	}
	_, err = queue.Get(app.QueueName, 1e6)
	c.Assert(err, NotNil)
}
