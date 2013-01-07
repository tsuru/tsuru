// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/testing"
	. "launchpad.net/gocheck"
	"strings"
)

func (s *ELBSuite) TestHandleMessage(c *C) {
	id1 := s.server.NewInstance()
	id2 := s.server.NewInstance()
	defer s.server.RemoveInstance(id1)
	defer s.server.RemoveInstance(id2)
	app := testing.NewFakeApp("symfonia", "python", 1)
	manager := ELBManager{}
	err := manager.Create(app)
	c.Assert(err, IsNil)
	defer manager.Destroy(app)
	output := strings.Replace(simpleCollectOutput, "i-00004444", id1, -1)
	output = strings.Replace(output, "i-00004445", id2, -1)
	tmpdir, err := commandmocker.Add("juju", output)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	msg := queue.Message{
		Action: addUnitToLoadBalancer,
		Args:   []string{"symfonia", "symfonia/0", "symfonia/1"},
	}
	handle(&msg)
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, IsNil)
	c.Assert(resp.LoadBalancerDescriptions, HasLen, 1)
	instances := resp.LoadBalancerDescriptions[0].Instances
	c.Assert(instances, HasLen, 2)
	c.Assert(instances[0].InstanceId, Equals, id1)
	c.Assert(instances[1].InstanceId, Equals, id2)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
}
