// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"fmt"
	"net"
	"net/url"
	"runtime"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/iaas"
	tsurunet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
	"github.com/tsuru/tsuru/provision/docker/nodecontainer"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/tsurutest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
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
	healer.Shutdown()
	nodes, err := p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")

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
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "localhost")

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
	healer.Shutdown()
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
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")
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
	healer.Shutdown()
	nodes, err := p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")
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
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")
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
	healer.Shutdown()
	nodes, err := p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")
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
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")
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
	healer.Shutdown()
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")

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
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "localhost")

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
	healer.Shutdown()
	nodes, err := p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")

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
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "localhost")

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
	c.Assert(events[0].Reason, check.Equals, "2 consecutive failures")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingNode.Address, check.Equals, fmt.Sprintf("http://127.0.0.1:%d/", dockertest.URLPort(node1.URL())))
	c.Assert(events[0].CreatedNode.Address, check.Equals, fmt.Sprintf("http://localhost:%d", dockertest.URLPort(node2.URL())))
}

func (s *S) TestHealerHandleErrorDoesntTriggerEventIfNotNeeded(c *check.C) {
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner:           p,
		DisabledTime:          20,
		FailuresBeforeHealing: 1,
		WaitTimeNewMachine:    time.Minute,
	})
	healer.Shutdown()
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
	p, err := s.newFakeDockerProvisioner("addr7")
	c.Assert(err, check.IsNil)
	iaas.RegisterIaasProvider("my-healer-iaas", dockertest.NewHealerIaaSConstructor("127.0.0.1", nil))
	healer := NewNodeHealer(NodeHealerArgs{
		DisabledTime:          20,
		FailuresBeforeHealing: 1,
		WaitTimeNewMachine:    time.Minute,
		Provisioner:           p,
	})
	healer.Shutdown()
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

func (s *S) TestHealerUpdateNodeData(c *check.C) {
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner: p,
	})
	healer.Shutdown()
	data := provision.NodeStatusData{
		Addrs: []string{"127.0.0.1"},
		Checks: []provision.NodeCheckResult{
			{Name: "ok1", Successful: true},
			{Name: "ok2", Successful: true},
		},
	}
	err = healer.UpdateNodeData(data)
	c.Assert(err, check.IsNil)
	coll, err := nodeDataCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	var result nodeStatusData
	err = coll.FindId(node1.URL()).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.LastSuccess.IsZero(), check.Equals, false)
	c.Assert(result.LastUpdate.IsZero(), check.Equals, false)
	c.Assert(result.Checks[0].Time.IsZero(), check.Equals, false)
	result.LastUpdate = time.Time{}
	result.LastSuccess = time.Time{}
	result.Checks[0].Time = time.Time{}
	c.Assert(result, check.DeepEquals, nodeStatusData{
		Address: node1.URL(),
		Checks:  []nodeChecks{{Checks: data.Checks}},
	})
}

func (s *S) TestHealerUpdateNodeDataSavesLast10Checks(c *check.C) {
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner: p,
	})
	healer.Shutdown()
	for i := 0; i < 20; i++ {
		data := provision.NodeStatusData{
			Addrs: []string{"127.0.0.1"},
			Checks: []provision.NodeCheckResult{
				{Name: fmt.Sprintf("ok1-%d", i), Successful: true},
				{Name: fmt.Sprintf("ok2-%d", i), Successful: true},
			},
		}
		err = healer.UpdateNodeData(data)
		c.Assert(err, check.IsNil)
	}
	coll, err := nodeDataCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	var result nodeStatusData
	err = coll.FindId(node1.URL()).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.LastSuccess.IsZero(), check.Equals, false)
	c.Assert(result.LastUpdate.IsZero(), check.Equals, false)
	result.LastUpdate = time.Time{}
	result.LastSuccess = time.Time{}
	c.Assert(result.Checks, check.HasLen, 10)
	expectedChecks := []nodeChecks{}
	for i, check := range result.Checks {
		expectedChecks = append(expectedChecks, nodeChecks{
			Time: check.Time,
			Checks: []provision.NodeCheckResult{
				{Name: fmt.Sprintf("ok1-%d", 10+i), Successful: true},
				{Name: fmt.Sprintf("ok2-%d", 10+i), Successful: true},
			},
		})
	}
	c.Assert(result, check.DeepEquals, nodeStatusData{
		Address: node1.URL(),
		Checks:  expectedChecks,
	})
}

func (s *S) TestHealerUpdateNodeDataNodeAddrNotFound(c *check.C) {
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner: p,
	})
	healer.Shutdown()
	data := provision.NodeStatusData{
		Addrs: []string{"10.0.0.1"},
		Checks: []provision.NodeCheckResult{
			{Name: "ok1", Successful: true},
			{Name: "ok2", Successful: true},
		},
	}
	err = healer.UpdateNodeData(data)
	c.Assert(err, check.ErrorMatches, `\[node healer update\] node not found for addrs: \[10.0.0.1\]`)
}

func (s *S) TestHealerUpdateNodeDataNodeFromUnits(c *check.C) {
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
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
		Provisioner: p,
	})
	healer.Shutdown()
	conts := p.AllContainers()
	c.Assert(conts, check.HasLen, 1)
	data := provision.NodeStatusData{
		Units: []provision.UnitStatusData{
			{ID: conts[0].ID},
		},
		Addrs: []string{"10.0.0.1"},
		Checks: []provision.NodeCheckResult{
			{Name: "ok1", Successful: true},
			{Name: "ok2", Successful: true},
		},
	}
	err = healer.UpdateNodeData(data)
	c.Assert(err, check.IsNil)
	coll, err := nodeDataCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	var result nodeStatusData
	err = coll.FindId(node1.URL()).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.LastSuccess.IsZero(), check.Equals, false)
	c.Assert(result.LastUpdate.IsZero(), check.Equals, false)
	c.Assert(result.Checks[0].Time.IsZero(), check.Equals, false)
	result.LastUpdate = time.Time{}
	result.LastSuccess = time.Time{}
	result.Checks[0].Time = time.Time{}
	c.Assert(result, check.DeepEquals, nodeStatusData{
		Address: node1.URL(),
		Checks:  []nodeChecks{{Checks: data.Checks}},
	})
}

func (s *S) TestHealerUpdateNodeDataAmbiguousContainers(c *check.C) {
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2Url, _ := url.Parse(node2.URL())
	_, port, _ := net.SplitHostPort(node2Url.Host)
	node2Addr := fmt.Sprintf("http://localhost:%s/", port)
	p, err := s.newFakeDockerProvisioner(node1.URL(), node2Addr)
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
	_, err = p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node2Addr,
		App:       app,
		Amount:    map[string]int{"web": 1},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	conts := p.AllContainers()
	c.Assert(conts, check.HasLen, 2)
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner: p,
	})
	healer.Shutdown()
	data := provision.NodeStatusData{
		Units: []provision.UnitStatusData{
			{ID: conts[0].ID},
			{ID: conts[1].ID},
		},
		Addrs: []string{"10.0.0.1"},
		Checks: []provision.NodeCheckResult{
			{Name: "ok1", Successful: true},
			{Name: "ok2", Successful: true},
		},
	}
	err = healer.UpdateNodeData(data)
	c.Assert(err, check.ErrorMatches, `\[node healer update\] containers match multiple nodes: http://.*?/ and http://.*?/`)
}

func (s *S) TestHealerUpdateNodeDataAmbiguousAddrs(c *check.C) {
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2Url, _ := url.Parse(node2.URL())
	_, port, _ := net.SplitHostPort(node2Url.Host)
	node2Addr := fmt.Sprintf("http://localhost:%s/", port)
	p, err := s.newFakeDockerProvisioner(node1.URL(), node2Addr)
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner: p,
	})
	healer.Shutdown()
	data := provision.NodeStatusData{
		Addrs: []string{"127.0.0.1", "localhost"},
		Checks: []provision.NodeCheckResult{
			{Name: "ok1", Successful: true},
			{Name: "ok2", Successful: true},
		},
	}
	err = healer.UpdateNodeData(data)
	c.Assert(err, check.ErrorMatches, `\[node healer update\] addrs match multiple nodes: \[.*? .*?\]`)
}

func (s *S) TestFindNodesForHealingNoNodes(c *check.C) {
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner: p,
	})
	healer.Shutdown()
	nodes, nodesMap, err := healer.findNodesForHealing()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
	n, err := p.Cluster().GetNode(node1.URL())
	c.Assert(err, check.IsNil)
	c.Assert(nodesMap, check.DeepEquals, map[string]*cluster.Node{
		n.Address: &n,
	})
}

func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}

func (s *S) TestFindNodesForHealingWithConfNoEntries(c *check.C) {
	conf := healerConfig()
	err := conf.SaveBase(NodeHealerConfig{Enabled: boolPtr(true), MaxUnresponsiveTime: intPtr(1)})
	c.Assert(err, check.IsNil)
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner: p,
	})
	healer.Shutdown()
	time.Sleep(1200 * time.Millisecond)
	nodes, nodesMap, err := healer.findNodesForHealing()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
	n, err := p.Cluster().GetNode(node1.URL())
	c.Assert(err, check.IsNil)
	c.Assert(nodesMap, check.DeepEquals, map[string]*cluster.Node{
		n.Address: &n,
	})
}

func (s *S) TestFindNodesForHealingLastUpdateDefault(c *check.C) {
	conf := healerConfig()
	err := conf.SaveBase(NodeHealerConfig{Enabled: boolPtr(true), MaxUnresponsiveTime: intPtr(1)})
	c.Assert(err, check.IsNil)
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner: p,
	})
	healer.Shutdown()
	healer.started = time.Now().Add(-3 * time.Second)
	data := provision.NodeStatusData{
		Addrs:  []string{"127.0.0.1"},
		Checks: []provision.NodeCheckResult{},
	}
	err = healer.UpdateNodeData(data)
	c.Assert(err, check.IsNil)
	time.Sleep(1200 * time.Millisecond)
	nodes, nodesMap, err := healer.findNodesForHealing()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	n, err := p.Cluster().GetNode(node1.URL())
	c.Assert(err, check.IsNil)
	c.Assert(nodesMap, check.DeepEquals, map[string]*cluster.Node{
		n.Address: &n,
	})
}

func (s *S) TestFindNodesForHealingLastUpdateWithRecentStarted(c *check.C) {
	conf := healerConfig()
	err := conf.SaveBase(NodeHealerConfig{Enabled: boolPtr(true), MaxUnresponsiveTime: intPtr(1)})
	c.Assert(err, check.IsNil)
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	p, err := s.newFakeDockerProvisioner(node1.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewNodeHealer(NodeHealerArgs{
		Provisioner: p,
	})
	healer.Shutdown()
	data := provision.NodeStatusData{
		Addrs:  []string{"127.0.0.1"},
		Checks: []provision.NodeCheckResult{},
	}
	err = healer.UpdateNodeData(data)
	c.Assert(err, check.IsNil)
	time.Sleep(1200 * time.Millisecond)
	nodes, nodesMap, err := healer.findNodesForHealing()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
	n, err := p.Cluster().GetNode(node1.URL())
	c.Assert(err, check.IsNil)
	c.Assert(nodesMap, check.DeepEquals, map[string]*cluster.Node{
		n.Address: &n,
	})
}

func (s *S) TestCheckActiveHealing(c *check.C) {
	conf := healerConfig()
	err := conf.SaveBase(NodeHealerConfig{Enabled: boolPtr(true), MaxUnresponsiveTime: intPtr(1)})
	c.Assert(err, check.IsNil)
	factory, iaasInst := dockertest.NewHealerIaaSConstructorWithInst("127.0.0.1")
	iaas.RegisterIaasProvider("my-healer-iaas", factory)
	_, err = iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
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
		Provisioner:        p,
		WaitTimeNewMachine: time.Minute,
	})
	healer.Shutdown()
	healer.started = time.Now().Add(-3 * time.Second)

	data := provision.NodeStatusData{
		Addrs:  []string{"127.0.0.1"},
		Checks: []provision.NodeCheckResult{},
	}
	err = healer.UpdateNodeData(data)
	c.Assert(err, check.IsNil)
	time.Sleep(1200 * time.Millisecond)

	nodes, err := p.Cluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node1.URL()))
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")
	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	_, err = p.Cluster().UpdateNode(nodes[0])
	c.Assert(err, check.IsNil)

	machines, err := iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "127.0.0.1")

	healer.runActiveHealing()

	nodes, err = p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node2.URL()))
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "localhost")

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
	c.Assert(events[0].Reason, check.Matches, `last update \d+\.\d*?s ago, last success \d+\.\d*?s ago`)
	extraObj := events[0].Extra.(bson.M)
	c.Assert(extraObj["time"].(time.Time).IsZero(), check.Equals, false)
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingNode.Address, check.Equals, fmt.Sprintf("http://127.0.0.1:%d/", dockertest.URLPort(node1.URL())))
	c.Assert(events[0].CreatedNode.Address, check.Equals, fmt.Sprintf("http://localhost:%d", dockertest.URLPort(node2.URL())))
}

func (s *S) TestTryHealingNodeConcurrent(c *check.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
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
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "127.0.0.1")
	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	machines, err := iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "127.0.0.1")
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			healErr := healer.tryHealingNode(&nodes[0], "something", "extra")
			c.Assert(healErr, check.IsNil)
		}()
	}
	wg.Wait()
	nodes, err = p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(tsurunet.URLToHost(nodes[0].Address), check.Equals, "localhost")
	c.Assert(dockertest.URLPort(nodes[0].Address), check.Equals, dockertest.URLPort(node2.URL()))
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
	c.Assert(events[0].Reason, check.Equals, "something")
	c.Assert(events[0].Extra, check.Equals, "extra")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingNode.Address, check.Equals, fmt.Sprintf("http://127.0.0.1:%d/", dockertest.URLPort(node1.URL())))
	c.Assert(events[0].CreatedNode.Address, check.Equals, fmt.Sprintf("http://localhost:%d", dockertest.URLPort(node2.URL())))
}

func (s *S) TestUpdateConfigIgnoresEmpty(c *check.C) {
	err := UpdateConfig("", NodeHealerConfig{
		Enabled:             boolPtr(true),
		MaxUnresponsiveTime: intPtr(1),
	})
	c.Assert(err, check.IsNil)
	conf := healerConfig()
	var nodeConf NodeHealerConfig
	err = conf.Load("p1", &nodeConf)
	c.Assert(err, check.IsNil)
	c.Assert(nodeConf, check.DeepEquals, NodeHealerConfig{
		Enabled:                      boolPtr(true),
		MaxUnresponsiveTime:          intPtr(1),
		EnabledInherited:             true,
		MaxUnresponsiveTimeInherited: true,
		MaxTimeSinceSuccessInherited: true,
	})
	err = UpdateConfig("p1", NodeHealerConfig{
		MaxTimeSinceSuccess: intPtr(2),
	})
	c.Assert(err, check.IsNil)
	nodeConf = NodeHealerConfig{}
	err = conf.Load("p1", &nodeConf)
	c.Assert(err, check.IsNil)
	c.Assert(nodeConf, check.DeepEquals, NodeHealerConfig{
		Enabled:                      boolPtr(true),
		MaxUnresponsiveTime:          intPtr(1),
		MaxTimeSinceSuccess:          intPtr(2),
		EnabledInherited:             true,
		MaxUnresponsiveTimeInherited: true,
		MaxTimeSinceSuccessInherited: false,
	})
	err = UpdateConfig("p1", NodeHealerConfig{
		MaxTimeSinceSuccess: intPtr(2),
		MaxUnresponsiveTime: intPtr(9),
	})
	c.Assert(err, check.IsNil)
	nodeConf = NodeHealerConfig{}
	err = conf.Load("p1", &nodeConf)
	c.Assert(err, check.IsNil)
	c.Assert(nodeConf, check.DeepEquals, NodeHealerConfig{
		Enabled:                      boolPtr(true),
		MaxUnresponsiveTime:          intPtr(9),
		MaxTimeSinceSuccess:          intPtr(2),
		EnabledInherited:             true,
		MaxUnresponsiveTimeInherited: false,
		MaxTimeSinceSuccessInherited: false,
	})

}

func (s *S) newFakeDockerProvisioner(servers ...string) (*dockertest.FakeDockerProvisioner, error) {
	p, err := dockertest.NewFakeDockerProvisioner(servers...)
	if err != nil {
		return nil, err
	}
	p.SetContainers("127.0.0.1", nil)
	p.SetContainers("localhost", nil)
	err = nodecontainer.RegisterQueueTask(p)
	return p, err
}
