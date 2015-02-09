// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
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
	oldCluster := dCluster
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
		dCluster = oldCluster
	}()
	iaasInstance := &TestHealerIaaS{}
	iaas.RegisterIaasProvider("my-healer-iaas", iaasInstance)
	iaasInstance.addr = "127.0.0.1"
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInstance.addr = "localhost"
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
	dCluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:     "127.0.0.1",
		unitsToAdd: 1,
		app:        appInstance,
		imageId:    imageId,
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

	healer := Healer{
		cluster:               cluster,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")

	containers, err := listAllContainers()
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
			containers, err := listAllContainers()
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
	healer := Healer{
		cluster:               cluster,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	created, err := healer.healNode(&nodes[0])
	c.Assert(err, check.ErrorMatches, ".*error creating new machine.*")
	c.Assert(created.Address, check.Equals, "")
	nodes, err = cluster.UnfilteredNodes()
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
	iaasInstance := &TestHealerIaaS{}
	iaas.RegisterIaasProvider("my-healer-iaas", iaasInstance)
	iaasInstance.addr = "127.0.0.1"
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInstance.err = fmt.Errorf("my create machine error")
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
	healer := Healer{
		cluster:               cluster,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := cluster.UnfilteredNodes()
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
	nodes, err = cluster.UnfilteredNodes()
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
	iaasInstance := &TestHealerIaaS{}
	iaas.RegisterIaasProvider("my-healer-iaas", iaasInstance)
	iaasInstance.addr = "127.0.0.1"
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInstance.addr = "localhost"
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
	healer := Healer{
		cluster:               cluster,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := cluster.UnfilteredNodes()
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
	nodes, err = cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")
}

func (s *S) TestHealerHealNodeDestroyError(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	iaasInstance := &TestHealerIaaS{}
	oldCluster := dCluster
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		iaasInstance.delErr = nil
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
		dCluster = oldCluster
		machines, _ = iaas.ListMachines()
	}()
	iaasInstance.delErr = fmt.Errorf("my destroy error")
	iaas.RegisterIaasProvider("my-healer-iaas", iaasInstance)
	iaasInstance.addr = "127.0.0.1"
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInstance.addr = "localhost"
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
	dCluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:     "127.0.0.1",
		unitsToAdd: 1,
		app:        appInstance,
		imageId:    imageId,
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

	healer := Healer{
		cluster:               cluster,
		disabledTime:          0,
		failuresBeforeHealing: 1,
		waitTimeNewMachine:    1 * time.Second,
	}
	nodes, err := cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node1.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "127.0.0.1")

	containers, err := listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")

	machines, err := iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
	c.Assert(machines[0].Address, check.Equals, "127.0.0.1")

	nodes[0].Metadata["iaas"] = "my-healer-iaas"
	created, err := healer.healNode(&nodes[0])
	c.Assert(err, check.ErrorMatches, "(?s)Unable to destroy machine.*my destroy error")
	c.Assert(created.Address, check.Equals, fmt.Sprintf("http://localhost:%d", urlPort(node2.URL())))

	nodes, err = cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(urlPort(nodes[0].Address), check.Equals, urlPort(node2.URL()))
	c.Assert(urlToHost(nodes[0].Address), check.Equals, "localhost")

	machines, err = iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 2)
	c.Assert(machines[0].Address, check.Equals, "127.0.0.1")
	c.Assert(machines[1].Address, check.Equals, "localhost")

	done := make(chan bool)
	go func() {
		for range time.Tick(100 * time.Millisecond) {
			containers, err := listAllContainers()
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

func (s *S) TestHealContainer(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	oldCluster := dCluster
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		dCluster = oldCluster
	}()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
	)
	c.Assert(err, check.IsNil)
	dCluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:     "127.0.0.1",
		unitsToAdd: 1,
		app:        appInstance,
		imageId:    imageId,
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

	containers, err := listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")

	node1.PrepareFailure("createError", "/containers/create")

	locker := &appLocker{}
	locked := locker.lock(containers[0].AppName)
	c.Assert(locked, check.Equals, true)
	defer locker.unlock(containers[0].AppName)
	_, err = healContainer(containers[0], locker)
	c.Assert(err, check.IsNil)

	containers, err = listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].HostAddr, check.Equals, "localhost")
}

func (s *S) TestRunContainerHealer(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	oldCluster := dCluster
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		dCluster = oldCluster
	}()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
	)
	c.Assert(err, check.IsNil)
	dCluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:     "127.0.0.1",
		unitsToAdd: 2,
		app:        appInstance,
		imageId:    imageId,
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

	containers, err := listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")
	c.Assert(containers[1].HostAddr, check.Equals, "127.0.0.1")

	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	coll := collection()
	err = coll.Update(bson.M{"id": toMoveCont.ID}, toMoveCont)
	c.Assert(err, check.IsNil)

	node1.PrepareFailure("createError", "/containers/create")

	runContainerHealerOnce(1 * time.Minute)

	containers, err = listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	hosts := []string{containers[0].HostAddr, containers[1].HostAddr}
	sort.Strings(hosts)
	c.Assert(hosts[0], check.Equals, "127.0.0.1")
	c.Assert(hosts[1], check.Equals, "localhost")

	healingColl, err := healingCollection()
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "container-healing")
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingContainer.HostAddr, check.Equals, "127.0.0.1")
	c.Assert(events[0].CreatedContainer.HostAddr, check.Equals, "localhost")
}

func (s *S) TestRunContainerHealerConcurrency(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	oldCluster := dCluster
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		dCluster = oldCluster
	}()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
	)
	c.Assert(err, check.IsNil)
	dCluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:     "127.0.0.1",
		unitsToAdd: 2,
		app:        appInstance,
		imageId:    imageId,
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

	containers, err := listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")
	c.Assert(containers[1].HostAddr, check.Equals, "127.0.0.1")
	toMoveCont := containers[1]

	node1.PrepareFailure("createError", "/containers/create")

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		healContainerIfNeeded(toMoveCont)
		wg.Done()
	}()
	go func() {
		healContainerIfNeeded(toMoveCont)
		wg.Done()
	}()
	wg.Wait()

	containers, err = listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	hosts := []string{containers[0].HostAddr, containers[1].HostAddr}
	sort.Strings(hosts)
	c.Assert(hosts[0], check.Equals, "127.0.0.1")
	c.Assert(hosts[1], check.Equals, "localhost")

	healingColl, err := healingCollection()
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "container-healing")
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingContainer.HostAddr, check.Equals, "127.0.0.1")
	c.Assert(events[0].CreatedContainer.HostAddr, check.Equals, "localhost")
}

func (s *S) TestRunContainerHealerAlreadyHealed(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	oldCluster := dCluster
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		dCluster = oldCluster
	}()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
	)
	c.Assert(err, check.IsNil)
	dCluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:     "127.0.0.1",
		unitsToAdd: 2,
		app:        appInstance,
		imageId:    imageId,
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

	containers, err := listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")
	c.Assert(containers[1].HostAddr, check.Equals, "127.0.0.1")
	toMoveCont := containers[1]

	node1.PrepareFailure("createError", "/containers/create")

	healContainerIfNeeded(toMoveCont)
	healContainerIfNeeded(toMoveCont)

	containers, err = listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	hosts := []string{containers[0].HostAddr, containers[1].HostAddr}
	sort.Strings(hosts)
	c.Assert(hosts[0], check.Equals, "127.0.0.1")
	c.Assert(hosts[1], check.Equals, "localhost")

	healingColl, err := healingCollection()
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "container-healing")
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingContainer.HostAddr, check.Equals, "127.0.0.1")
	c.Assert(events[0].CreatedContainer.HostAddr, check.Equals, "localhost")
}

func (s *S) TestRunContainerHealerDoesntHealWithProcfileInTop(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	oldCluster := dCluster
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		dCluster = oldCluster
	}()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
	)
	c.Assert(err, check.IsNil)
	dCluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	cont, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:     "127.0.0.1",
		unitsToAdd: 1,
		app:        appInstance,
		imageId:    imageId,
	})
	c.Assert(err, check.IsNil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"Titles": [], "Processes": [["x", "ProcfileWatcher"]]}`)
	})
	node1.CustomHandler("/containers/"+cont[0].ID+"/top", handler)

	toMoveCont := cont[0]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	coll := collection()
	err = coll.Update(bson.M{"id": toMoveCont.ID}, toMoveCont)
	c.Assert(err, check.IsNil)

	runContainerHealerOnce(1 * time.Minute)

	healingColl, err := healingCollection()
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 0)
}

func (s *S) TestRunContainerHealerWithError(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	oldCluster := dCluster
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		dCluster = oldCluster
	}()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
	)
	c.Assert(err, check.IsNil)
	dCluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:     "127.0.0.1",
		unitsToAdd: 2,
		app:        appInstance,
		imageId:    imageId,
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

	containers, err := listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")
	c.Assert(containers[1].HostAddr, check.Equals, "127.0.0.1")

	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	coll := collection()
	err = coll.Update(bson.M{"id": toMoveCont.ID}, toMoveCont)
	c.Assert(err, check.IsNil)

	node1.PrepareFailure("createError", "/containers/create")
	node2.PrepareFailure("createError", "/containers/create")

	runContainerHealerOnce(1 * time.Minute)

	containers, err = listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	hosts := []string{containers[0].HostAddr, containers[1].HostAddr}
	c.Assert(hosts[0], check.Equals, "127.0.0.1")
	c.Assert(hosts[1], check.Equals, "127.0.0.1")

	healingColl, err := healingCollection()
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "container-healing")
	c.Assert(events[0].StartTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].EndTime, check.Not(check.DeepEquals), time.Time{})
	c.Assert(events[0].Error, check.Matches, "(?s).*Error trying to heal containers.*")
	c.Assert(events[0].Successful, check.Equals, false)
	c.Assert(events[0].FailingContainer.HostAddr, check.Equals, "127.0.0.1")
	c.Assert(events[0].CreatedContainer.HostAddr, check.Equals, "")
}

func (s *S) TestRunContainerHealerMaxCounterExceeded(c *check.C) {
	conts := []container{
		{ID: "cont1"}, {ID: "cont2"}, {ID: "cont3"}, {ID: "cont4"},
		{ID: "cont5"}, {ID: "cont6"}, {ID: "cont7"}, {ID: "cont8"},
	}
	for i := 0; i < len(conts)-1; i++ {
		evt, err := newHealingEvent(conts[i])
		c.Assert(err, check.IsNil)
		err = evt.update(conts[i+1], nil)
		c.Assert(err, check.IsNil)
	}
	toMoveCont := conts[7]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	coll := collection()
	err := coll.Insert(toMoveCont)
	c.Assert(err, check.IsNil)
	err = healContainerIfNeeded(toMoveCont)
	c.Assert(err, check.ErrorMatches, "Containers healing: number of healings for container cont8 in the last 30 minutes exceeds limit of 3: 7")
	healingColl, err := healingCollection()
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 7)
}

func (s *S) TestHealerHandleError(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	oldCluster := dCluster
	defer func() {
		cmutex.Lock()
		defer cmutex.Unlock()
		machines, _ := iaas.ListMachines()
		for _, m := range machines {
			m.Destroy()
		}
		dCluster = oldCluster
	}()
	iaasInstance := &TestHealerIaaS{}
	iaas.RegisterIaasProvider("my-healer-iaas", iaasInstance)
	iaasInstance.addr = "127.0.0.1"
	_, err := iaas.CreateMachineForIaaS("my-healer-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	iaasInstance.addr = "localhost"
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
	dCluster = cluster

	appInstance := provisiontest.NewFakeApp("myapp", "python", 0)
	var p dockerProvisioner
	defer p.Destroy(appInstance)
	p.Provision(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:     "127.0.0.1",
		unitsToAdd: 1,
		app:        appInstance,
		imageId:    imageId,
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

	healer := Healer{
		cluster:               cluster,
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
	healer := Healer{
		cluster:               nil,
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
	iaasInstance := &TestHealerIaaS{}
	iaas.RegisterIaasProvider("my-healer-iaas", iaasInstance)
	healer := Healer{
		cluster:               nil,
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
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 7)
}

func (s *S) TestHealingCountFor(c *check.C) {
	conts := []container{
		{ID: "cont1"}, {ID: "cont2"}, {ID: "cont3"}, {ID: "cont4"},
		{ID: "cont5"}, {ID: "cont6"}, {ID: "cont7"}, {ID: "cont8"},
	}
	for i := 0; i < len(conts)-1; i++ {
		evt, err := newHealingEvent(conts[i])
		c.Assert(err, check.IsNil)
		err = evt.update(conts[i+1], nil)
		c.Assert(err, check.IsNil)
	}
	count, err := healingCountFor("container", "cont8", time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 7)
}

func (s *S) TestHealingCountForOldEventsNotConsidered(c *check.C) {
	conts := []container{
		{ID: "cont1"}, {ID: "cont2"}, {ID: "cont3"}, {ID: "cont4"},
		{ID: "cont5"}, {ID: "cont6"}, {ID: "cont7"}, {ID: "cont8"},
	}
	for i := 0; i < len(conts)-1; i++ {
		evt, err := newHealingEvent(conts[i])
		c.Assert(err, check.IsNil)
		err = evt.update(conts[i+1], nil)
		c.Assert(err, check.IsNil)
		if i < 4 {
			coll, err := healingCollection()
			c.Assert(err, check.IsNil)
			defer coll.Close()
			evt.StartTime = time.Now().UTC().Add(-2 * time.Minute)
			err = coll.UpdateId(evt.ID, evt)
			c.Assert(err, check.IsNil)
		}
	}
	count, err := healingCountFor("container", "cont8", time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 3)
}

func (s *S) TestHealingCountForWithNode(c *check.C) {
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
	count, err := healingCountFor("node", "addr8", time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 7)
}
