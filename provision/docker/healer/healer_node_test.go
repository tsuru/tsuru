// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"fmt"
	"time"

	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision/docker/bs"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/tsurutest"
	"gopkg.in/check.v1"
)

func (s *S) TestHealerHealNode(c *check.C) {
	factory, iaasInst := dockertest.NewHealerIaaSConstructorWithInst("127.0.0.1")
	iaas.RegisterIaasProvider("my-healer-iaas", factory)
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInst.Addr = "localhost"
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-protocol", "http")
	config.Set("iaas:node-port", dockertest.URLPort(node2.URL()))
	defer config.Unset("iaas:node-protocol")
	defer config.Unset("iaas:node-port")
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	app := provisiontest.NewFakeApp("myapp", "python", 0)
	_, err = p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 1},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)

	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner:           p,
		FailuresBeforeHealing: 1,
		WaitTimeNewMachine:    time.Minute,
	})
	nodes, err := p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")

	containers := p.AllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")

	machines, err := iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "127.0.0.1")

	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	created, err := healer.healNode(&nodes[0])
	c.Assert(err, check.IsNil)
	c.Assert(created.Address, check.Equals, fmt.Sprintf("http://localhost:%d", dockertest.URLPort(node2.URL())))
	nodes, err = p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node2.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "localhost")

	machines, err = iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "localhost")

	err = tsurutest.WaitCondition(5*time.Second, func() bool {
		containers := p.AllContainers()
		return len(containers) == 1 && containers[0].HostAddr == "localhost"
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestHealerHealNodeWithoutIaaS(c *check.C) {
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner:           p,
		FailuresBeforeHealing: 1,
		WaitTimeNewMachine:    time.Second,
	})
	nodes, err := p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	created, err := healer.healNode(&nodes[0])
	c.Assert(err, check.ErrorMatches, ".*error creating new machine.*")
	c.Assert(created.Address, check.Equals, "")
	nodes, err = p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
}

func (s *S) TestHealerHealNodeCreateMachineError(c *check.C) {
	factory, iaasInst := dockertest.NewHealerIaaSConstructorWithInst("127.0.0.1")
	iaas.RegisterIaasProvider("my-healer-iaas", factory)
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInst.Addr = "localhost"
	iaasInst.Err = fmt.Errorf("my create machine error")
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1.PrepareFailure("pingErr", "/_ping")
	p.Cluster().StartActiveMonitoring(100 * time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	p.Cluster().StopActiveMonitoring()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner:           p,
		FailuresBeforeHealing: 1,
		WaitTimeNewMachine:    time.Minute,
	})
	nodes, err := p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
	c.Assert(nodes[0].FailureCount() > 0, check.Equals, true)
	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	created, err := healer.healNode(&nodes[0])
	c.Assert(err, check.ErrorMatches, ".*my create machine error.*")
	c.Assert(created.Address, check.Equals, "")
	c.Assert(nodes[0].FailureCount(), check.Equals, 0)
	nodes, err = p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
}

func (s *S) TestHealerHealNodeWaitAndRegisterError(c *check.C) {
	iaas.RegisterIaasProvider("my-healer-iaas", dockertest.NewHealerIaaSConstructor("127.0.0.1", nil))
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaas.RegisterIaasProvider("my-healer-iaas", dockertest.NewHealerIaaSConstructor("localhost", nil))
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2.PrepareFailure("ping-failure", "/_ping")
	config.Set("iaas:node-protocol", "http")
	config.Set("iaas:node-port", dockertest.URLPort(node2.URL()))
	defer config.Unset("iaas:node-protocol")
	defer config.Unset("iaas:node-port")
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1.PrepareFailure("pingErr", "/_ping")
	p.Cluster().StartActiveMonitoring(100 * time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	p.Cluster().StopActiveMonitoring()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner:           p,
		FailuresBeforeHealing: 1,
		WaitTimeNewMachine:    time.Second,
	})
	nodes, err := p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
	c.Assert(nodes[0].FailureCount() > 0, check.Equals, true)
	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	created, err := healer.healNode(&nodes[0])
	c.Assert(err, check.ErrorMatches, ".*timeout waiting for result.*")
	c.Assert(created.Address, check.Equals, "")
	c.Assert(nodes[0].FailureCount(), check.Equals, 0)
	nodes, err = p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
}

func (s *S) TestHealerHealNodeDestroyError(c *check.C) {
	factory, iaasInst := dockertest.NewHealerIaaSConstructorWithInst("127.0.0.1")
	iaasInst.DelErr = fmt.Errorf("my destroy error")
	iaas.RegisterIaasProvider("my-healer-iaas", factory)
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInst.Addr = "localhost"
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-protocol", "http")
	config.Set("iaas:node-port", dockertest.URLPort(node2.URL()))
	defer config.Unset("iaas:node-protocol")
	defer config.Unset("iaas:node-port")
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()

	app := provisiontest.NewFakeApp("myapp", "python", 0)
	_, err = p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 1},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)

	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner:           p,
		FailuresBeforeHealing: 1,
		WaitTimeNewMachine:    time.Minute,
	})
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")

	containers := p.AllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")

	machines, err := iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "127.0.0.1")

	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	created, err := healer.healNode(&nodes[0])
	c.Assert(err, check.IsNil)
	c.Assert(created.Address, check.Equals, fmt.Sprintf("http://localhost:%d", dockertest.URLPort(node2.URL())))

	nodes, err = p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node2.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "localhost")

	machines, err = iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "localhost")

	err = tsurutest.WaitCondition(5*time.Second, func() bool {
		containers := p.AllContainers()
		return len(containers) == 1 && containers[0].HostAddr == "localhost"
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestHealerHandleError(c *check.C) {
	factory, iaasInst := dockertest.NewHealerIaaSConstructorWithInst("127.0.0.1")
	iaas.RegisterIaasProvider("my-healer-iaas", factory)
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInst.Addr = "localhost"
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-protocol", "http")
	config.Set("iaas:node-port", dockertest.URLPort(node2.URL()))
	defer config.Unset("iaas:node-protocol")
	defer config.Unset("iaas:node-port")
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()

	app := provisiontest.NewFakeApp("myapp", "python", 0)
	_, err = p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 1},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)

	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner:           p,
		FailuresBeforeHealing: 1,
		WaitTimeNewMachine:    time.Minute,
	})
	nodes, err := p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")

	machines, err := iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "127.0.0.1")

	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	nodes[0].Metadata["Failures"] = "2"

	waitTime := healer.HandleError(&nodes[0])
	c.Assert(waitTime, check.Equals, time.Duration(0))

	nodes, err = p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node2.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "localhost")

	machines, err = iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "localhost")

	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "node-healing")
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingNode.Address, check.Equals, fmt.Sprintf("http://127.0.0.1:%d/", dockertest.URLPort(node1.URL())))
	c.Assert(events[0].CreatedNode.Address, check.Equals, fmt.Sprintf("http://localhost:%d", dockertest.URLPort(node2.URL())))
}

func (s *S) TestHealerHandleErrorDoesntTriggerEventIfNotNeeded(c *check.C) {
	healer := NewNodeHealer(NodeHealerArgs{
		DisabledTime:          20,
		FailuresBeforeHealing: 1,
		WaitTimeNewMachine:    time.Minute,
	})
	node := cluster.Node{Address: "addr", Metadata: map[string]string{
		"Failures":    "2",
		"LastSuccess": "something",
	}}
	waitTime := healer.HandleError(&node)
	c.Assert(waitTime, check.Equals, time.Duration(20))
	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 0)
	node = cluster.Node{Address: "addr", Metadata: map[string]string{
		"Failures":    "0",
		"LastSuccess": "something",
		"iaas":        "invalid",
	}}
	waitTime = healer.HandleError(&node)
	c.Assert(waitTime, check.Equals, time.Duration(20))
	healingColl, err = healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 0)
	node = cluster.Node{Address: "addr", Metadata: map[string]string{
		"Failures": "2",
		"iaas":     "invalid",
	}}
	waitTime = healer.HandleError(&node)
	c.Assert(waitTime, check.Equals, time.Duration(20))
	healingColl, err = healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 0)
}

func (s *S) TestHealerHandleErrorDoesntTriggerEventIfHealingCountTooLarge(c *check.C) {
	nodes := []cluster.Node{
		{Address: "addr1"}, {Address: "addr2"}, {Address: "addr3"}, {Address: "addr4"},
		{Address: "addr5"}, {Address: "addr6"}, {Address: "addr7"}, {Address: "addr8"},
	}
	for i := 0; i < len(nodes)-1; i++ {
		evt, err := NewHealingEvent(nodes[i])
		c.Assert(err, check.IsNil)
		err = evt.Update(nodes[i+1], nil)
		c.Assert(err, check.IsNil)
	}
	iaas.RegisterIaasProvider("my-healer-iaas", dockertest.NewHealerIaaSConstructor("127.0.0.1", nil))
	healer := NewNodeHealer(NodeHealerArgs{
		DisabledTime:          20,
		FailuresBeforeHealing: 1,
		WaitTimeNewMachine:    time.Minute,
	})
	nodes[7].Metadata = map[string]string{
		"Failures":    "2",
		"LastSuccess": "something",
		"iaas":        "my-healer-iaas",
	}
	waitTime := healer.HandleError(&nodes[7])
	c.Assert(waitTime, check.Equals, time.Duration(20))
	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 7)
}

func (s *S) newFakeDockerProvisioner(servers ...string) (*dockertest.FakeDockerProvisioner, error) {
	p, err := dockertest.NewFakeDockerProvisioner(servers...)
	if err != nil {
		return nil, err
	}
	p.SetContainers("127.0.0.1", nil)
	p.SetContainers("localhost", nil)
	err = bs.RegisterQueueTask(p)
	return p, err
}
