// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type TestHealerIaaS struct {
	addr   string
	err    error
	delErr error
}

func (t *TestHealerIaaS) DeleteMachine(m *iaas.Machine) error {
	if t.delErr != nil {
		return t.delErr
	}
	return nil
}

func (t *TestHealerIaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	if t.err != nil {
		return nil, t.err
	}
	m := iaas.Machine{
		Id:      "m-" + t.addr,
		Status:  "running",
		Address: t.addr,
	}
	return &m, nil
}

func (TestHealerIaaS) Describe() string {
	return "iaas describe"
}

func urlPort(uStr string) int {
	url, _ := url.Parse(uStr)
	_, port, _ := net.SplitHostPort(url.Host)
	portInt, _ := strconv.Atoi(port)
	return portInt
}

func (s *S) TestHealerHealNode(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	factory, iaasInst := newHealerIaaSConstructorWithInst("127.0.0.1")
	iaas.RegisterIaasProvider("my-healer-iaas", factory)
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInst.addr = "localhost"
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-protocol", "http")
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-protocol")
	defer config.Unset("iaas:node-port")
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
	)
	c.Assert(err, check.IsNil)
	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster = cluster
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"procfile": "web: python ./myapp",
	}
	err = saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	healer := nodeHealer{
		locks:                 make(map[string]*sync.Mutex),
		provisioner:           &p,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := p.getCluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")

	containers, err := p.listAllContainers()
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
	c.Assert(created.Address, check.Equals, fmt.Sprintf("http://localhost:%d", urlPort(node2.URL())))
	nodes, err = cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node2.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "localhost")

	machines, err = iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "localhost")

	done := make(chan bool)
	go func() {
		for range time.Tick(100 * time.Millisecond) {
			containers, err := p.listAllContainers()
			if err == nil && len(containers) == 1 && containers[0].HostAddr == "localhost" {
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		c.Fatal("Timed out waiting for containers to move")
	}
}

func (s *S) TestHealerHealNodeWithoutIaaS(c *check.C) {
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
	)
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster = cluster
	healer := nodeHealer{
		locks:                 make(map[string]*sync.Mutex),
		provisioner:           &p,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := p.getCluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	created, err := healer.healNode(&nodes[0])
	c.Assert(err, check.ErrorMatches, ".*error creating new machine.*")
	c.Assert(created.Address, check.Equals, "")
	nodes, err = p.getCluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
}

func (s *S) TestHealerHealNodeCreateMachineError(c *check.C) {
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	factory, iaasInst := newHealerIaaSConstructorWithInst("127.0.0.1")
	iaas.RegisterIaasProvider("my-healer-iaas", factory)
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInst.addr = "localhost"
	iaasInst.err = fmt.Errorf("my create machine error")
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
	)
	c.Assert(err, check.IsNil)
	node1.PrepareFailure("pingErr", "/_ping")
	cluster.StartActiveMonitoring(100 * time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	cluster.StopActiveMonitoring()
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster = cluster
	healer := nodeHealer{
		locks:                 make(map[string]*sync.Mutex),
		provisioner:           &p,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := p.getCluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
	c.Assert(nodes[0].FailureCount() > 0, check.Equals, true)
	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	created, err := healer.healNode(&nodes[0])
	c.Assert(err, check.ErrorMatches, ".*my create machine error.*")
	c.Assert(created.Address, check.Equals, "")
	c.Assert(nodes[0].FailureCount(), check.Equals, 0)
	nodes, err = p.getCluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
}

func (s *S) TestHealerHealNodeWaitAndRegisterError(c *check.C) {
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	iaas.RegisterIaasProvider("my-healer-iaas", newHealerIaaSConstructor("127.0.0.1", nil))
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaas.RegisterIaasProvider("my-healer-iaas", newHealerIaaSConstructor("localhost", nil))
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2.PrepareFailure("ping-failure", "/_ping")
	config.Set("iaas:node-protocol", "http")
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-protocol")
	defer config.Unset("iaas:node-port")
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
	)
	c.Assert(err, check.IsNil)
	node1.PrepareFailure("pingErr", "/_ping")
	cluster.StartActiveMonitoring(100 * time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	cluster.StopActiveMonitoring()
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster = cluster
	healer := nodeHealer{
		locks:                 make(map[string]*sync.Mutex),
		provisioner:           &p,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := p.getCluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
	c.Assert(nodes[0].FailureCount() > 0, check.Equals, true)
	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	created, err := healer.healNode(&nodes[0])
	c.Assert(err, check.ErrorMatches, ".*error registering new node.*")
	c.Assert(created.Address, check.Equals, "")
	c.Assert(nodes[0].FailureCount(), check.Equals, 0)
	nodes, err = p.getCluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
}

func (s *S) TestHealerHealNodeDestroyError(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
		machines, _ = iaas.ListMachines()
	}()
	factory, iaasInst := newHealerIaaSConstructorWithInst("127.0.0.1")
	iaasInst.delErr = fmt.Errorf("my destroy error")
	iaas.RegisterIaasProvider("my-healer-iaas", factory)
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInst.addr = "localhost"
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-protocol", "http")
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-protocol")
	defer config.Unset("iaas:node-port")
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
	)
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"procfile": "web: python ./myapp",
	}
	err = saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	healer := nodeHealer{
		locks:                 make(map[string]*sync.Mutex),
		provisioner:           &p,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := p.getCluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")

	containers, err := p.listAllContainers()
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
	c.Assert(created.Address, check.Equals, fmt.Sprintf("http://localhost:%d", urlPort(node2.URL())))

	nodes, err = p.getCluster().UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node2.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "localhost")

	machines, err = iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "localhost")

	done := make(chan bool)
	go func() {
		for range time.Tick(100 * time.Millisecond) {
			containers, err := p.listAllContainers()
			if err == nil && len(containers) == 1 && containers[0].HostAddr == "localhost" {
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		c.Fatal("Timed out waiting for containers to move")
	}
}

func (s *S) TestHealerHandleError(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	defer func() {
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
	}()
	factory, iaasInst := newHealerIaaSConstructorWithInst("127.0.0.1")
	iaas.RegisterIaasProvider("my-healer-iaas", factory)
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInst.addr = "localhost"
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-protocol", "http")
	config.Set("iaas:node-port", urlPort(node2.URL()))
	defer config.Unset("iaas:node-protocol")
	defer config.Unset("iaas:node-port")
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
	)
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"procfile": "web: python ./myapp",
	}
	err = saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{
		Name: appInstance.GetName(),
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	healer := nodeHealer{
		locks:                 make(map[string]*sync.Mutex),
		provisioner:           &p,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")

	machines, err := iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "127.0.0.1")

	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	nodes[0].Metadata["Failures"] = "2"

	waitTime := healer.HandleError(&nodes[0])
	c.Assert(waitTime, check.Equals, time.Duration(0))

	nodes, err = cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node2.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "localhost")

	machines, err = iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "localhost")

	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "node-healing")
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingNode.Address, check.Equals, fmt.Sprintf("http://127.0.0.1:%d/", urlPort(node1.URL())))
	c.Assert(events[0].CreatedNode.Address, check.Equals, fmt.Sprintf("http://localhost:%d", urlPort(node2.URL())))
}

func (s *S) TestHealerHandleErrorDoesntTriggerEventIfNotNeeded(c *check.C) {
	healer := nodeHealer{
		locks:                 make(map[string]*sync.Mutex),
		provisioner:           nil,
		disabledTime:          20,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	node := cluster.Node{Address: "addr", Metadata: map[string]string{
		"Failures":    "2",
		"LastSuccess": "something",
	}}
	waitTime := healer.HandleError(&node)
	c.Assert(waitTime, check.Equals, time.Duration(20))
	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []healingEvent
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
		evt, err := newHealingEvent(nodes[i])
		c.Assert(err, check.IsNil)
		err = evt.update(nodes[i+1], nil)
		c.Assert(err, check.IsNil)
	}
	iaas.RegisterIaasProvider("my-healer-iaas", newHealerIaaSConstructor("127.0.0.1", nil))
	healer := nodeHealer{
		locks:                 make(map[string]*sync.Mutex),
		provisioner:           nil,
		disabledTime:          20,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
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
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 7)
}
