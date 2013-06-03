// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
	"sort"
	"strings"
)

func (s *ELBSuite) TestHandleMessageWithoutUnits(c *gocheck.C) {
	instIds := make([]string, 3)
	for i := 0; i < len(instIds); i++ {
		id := s.server.NewInstance()
		defer s.server.RemoveInstance(id)
		instIds[i] = id
	}
	replace := []string{"i-00004444", "i-00004445", "i-00004450"}
	output := simpleCollectOutput
	for i, r := range replace {
		output = strings.Replace(output, r, instIds[i], 1)
	}
	tmpdir, err := commandmocker.Add("juju", output)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	app := testing.NewFakeApp("symfonia", "python", 1)
	manager := ELBManager{}
	err = manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer manager.Destroy(app)
	handle(&queue.Message{
		Action: addUnitToLoadBalancer,
		Args:   []string{"symfonia"},
	})
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	instances := resp.LoadBalancerDescriptions[0].Instances
	c.Assert(instances, gocheck.HasLen, 3)
	ids := []string{instances[0].InstanceId, instances[1].InstanceId, instances[2].InstanceId}
	sort.Strings(ids)
	sort.Strings(instIds)
	c.Assert(ids, gocheck.DeepEquals, instIds)
}

func (s *ELBSuite) TestHandleMessageWithUnits(c *gocheck.C) {
	id1 := s.server.NewInstance()
	id2 := s.server.NewInstance()
	defer s.server.RemoveInstance(id1)
	defer s.server.RemoveInstance(id2)
	app := testing.NewFakeApp("symfonia", "python", 1)
	manager := ELBManager{}
	err := manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer manager.Destroy(app)
	output := strings.Replace(simpleCollectOutput, "i-00004444", id1, -1)
	output = strings.Replace(output, "i-00004445", id2, -1)
	tmpdir, err := commandmocker.Add("juju", output)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	msg := queue.Message{
		Action: addUnitToLoadBalancer,
		Args:   []string{"symfonia", "symfonia/0", "symfonia/1"},
	}
	handle(&msg)
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	instances := resp.LoadBalancerDescriptions[0].Instances
	c.Assert(instances, gocheck.HasLen, 2)
	ids := []string{instances[0].InstanceId, instances[1].InstanceId}
	sort.Strings(ids)
	want := []string{id1, id2}
	sort.Strings(want)
	c.Assert(ids, gocheck.DeepEquals, want)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
}

func (s *ELBSuite) TestHandleMessagesWithPendingUnits(c *gocheck.C) {
	id := s.server.NewInstance()
	defer s.server.RemoveInstance(id)
	app := testing.NewFakeApp("2112", "python", 1)
	manager := ELBManager{}
	err := manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer manager.Destroy(app)
	output := strings.Replace(collectOutputNoInstanceID, "i-00004444", id, 1)
	tmpdir, err := commandmocker.Add("juju", output)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	handle(&queue.Message{
		Action: addUnitToLoadBalancer,
		Args:   []string{"2112", "2112/0", "2112/1"},
	})
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	instances := resp.LoadBalancerDescriptions[0].Instances
	c.Assert(instances, gocheck.HasLen, 1)
	c.Assert(instances[0].InstanceId, gocheck.Equals, id)
	msg, err := getQueue(queueName).Get(5e9)
	c.Assert(err, gocheck.IsNil)
	defer msg.Delete()
	c.Assert(msg.Action, gocheck.Equals, addUnitToLoadBalancer)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{"2112", "2112/1"})
}

func (s *ELBSuite) TestHandleMessagesAllPendingUnits(c *gocheck.C) {
	app := testing.NewFakeApp("2112", "python", 1)
	manager := ELBManager{}
	err := manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer manager.Destroy(app)
	tmpdir, err := commandmocker.Add("juju", collectOutputAllPending)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	msg := queue.Message{
		Action: addUnitToLoadBalancer,
		Args:   []string{"2112", "2112/0", "2112/1"},
	}
	handle(&msg)
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	instances := resp.LoadBalancerDescriptions[0].Instances
	c.Assert(instances, gocheck.HasLen, 0)
	got, err := getQueue(queueName).Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, msg)
	got.Delete()
}

func (s *ELBSuite) TestEnqueuePutMessagesInSpecificQueue(c *gocheck.C) {
	enqueue(&queue.Message{Action: "clean-everything"})
	msg, err := getQueue("default").Get(1e6)
	if err == nil {
		// cleaning up if the test fail
		defer msg.Delete()
		c.Fatalf("Expected non-nil error, got <nil>.")
	}
	msg, err = getQueue(queueName).Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Action, gocheck.Equals, "clean-everything")
	msg.Delete()
}
