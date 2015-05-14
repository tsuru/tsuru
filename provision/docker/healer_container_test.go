// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestHealContainer(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
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
		toAdd:       map[string]int{"web": 1},
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

	containers, err := p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")

	node1.PrepareFailure("createError", "/containers/create")

	locker := &appLocker{}
	locked := locker.lock(containers[0].AppName)
	c.Assert(locked, check.Equals, true)
	defer locker.unlock(containers[0].AppName)
	healer := containerHealer{provisioner: &p}
	_, err = healer.healContainer(containers[0], locker)
	c.Assert(err, check.IsNil)

	containers, err = p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].HostAddr, check.Equals, "localhost")
}

func (s *S) TestRunContainerHealer(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
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
		toAdd:       map[string]int{"web": 2},
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

	containers, err := p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")
	c.Assert(containers[1].HostAddr, check.Equals, "127.0.0.1")

	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	coll := p.collection()
	err = coll.Update(bson.M{"id": toMoveCont.ID}, toMoveCont)
	c.Assert(err, check.IsNil)

	node1.PrepareFailure("createError", "/containers/create")

	healer := containerHealer{provisioner: &p, maxUnresponsiveTime: 1 * time.Minute}
	healer.runContainerHealerOnce()

	containers, err = p.listAllContainers()
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

func (s *S) TestRunContainerHealerShutdown(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
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
		toAdd:       map[string]int{"web": 2},
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

	containers, err := p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")
	c.Assert(containers[1].HostAddr, check.Equals, "127.0.0.1")

	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	coll := p.collection()
	err = coll.Update(bson.M{"id": toMoveCont.ID}, toMoveCont)
	c.Assert(err, check.IsNil)

	node1.PrepareFailure("createError", "/containers/create")

	healer := containerHealer{provisioner: &p, maxUnresponsiveTime: 1 * time.Minute, done: make(chan bool)}
	go healer.runContainerHealer()
	healer.Shutdown()

	containers, err = p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	hosts := []string{containers[0].HostAddr, containers[1].HostAddr}
	sort.Strings(hosts)
	c.Assert(hosts[0], check.Equals, "127.0.0.1")
	c.Assert(hosts[1], check.Equals, "localhost")
}

func (s *S) TestRunContainerHealerConcurrency(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
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
		toAdd:       map[string]int{"web": 2},
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

	containers, err := p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")
	c.Assert(containers[1].HostAddr, check.Equals, "127.0.0.1")
	toMoveCont := containers[1]

	node1.PrepareFailure("createError", "/containers/create")

	healer := containerHealer{provisioner: &p}
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		healer.healContainerIfNeeded(toMoveCont)
		wg.Done()
	}()
	go func() {
		healer.healContainerIfNeeded(toMoveCont)
		wg.Done()
	}()
	wg.Wait()

	containers, err = p.listAllContainers()
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
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
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
		toAdd:       map[string]int{"web": 2},
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

	containers, err := p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")
	c.Assert(containers[1].HostAddr, check.Equals, "127.0.0.1")
	toMoveCont := containers[1]

	node1.PrepareFailure("createError", "/containers/create")

	healer := containerHealer{provisioner: &p}
	healer.healContainerIfNeeded(toMoveCont)
	healer.healContainerIfNeeded(toMoveCont)

	containers, err = p.listAllContainers()
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

func (s *S) TestRunContainerHealerDoesntHealWithProcessRunning(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
	)
	c.Assert(err, check.IsNil)
	var p dockerProvisioner
	err = p.Initialize()
	c.Assert(err, check.IsNil)
	p.cluster = cluster

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appInstance := &app.App{Name: "myapp", Platform: "python"}
	err = conn.Apps().Insert(appInstance)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appInstance.Name})
	p.Provision(appInstance)
	defer p.Destroy(appInstance)
	imageId, err := appCurrentImageName(appInstance.GetName())
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"procfile": "web: python ./myapp",
	}
	err = saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	cont, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toHost:      "127.0.0.1",
		toAdd:       map[string]int{"web": 1},
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"Titles": [], "Processes": [["y", "Procfile"], ["x", "python ./myapp"]]}`)
	})
	node1.CustomHandler("/containers/"+cont[0].ID+"/top", handler)

	toMoveCont := cont[0]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	coll := p.collection()
	err = coll.Update(bson.M{"id": toMoveCont.ID}, toMoveCont)
	c.Assert(err, check.IsNil)

	healer := containerHealer{provisioner: &p, maxUnresponsiveTime: 1 * time.Minute}
	healer.runContainerHealerOnce()

	healingColl, err := healingCollection()
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 0)
}

func (s *S) TestRunContainerHealerWithError(c *check.C) {
	rollback := startTestRepositoryServer()
	defer rollback()
	node1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	node2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cluster, err := cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: node1.URL()},
		cluster.Node{Address: fmt.Sprintf("http://localhost:%d", urlPort(node2.URL()))},
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
		toAdd:       map[string]int{"web": 2},
		app:         appInstance,
		imageId:     imageId,
		provisioner: &p,
	})
	c.Assert(err, check.IsNil)

	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	appStruct := &app.App{Name: appInstance.GetName()}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	defer conn.Apps().Remove(bson.M{"name": appStruct.Name})

	containers, err := p.listAllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, "127.0.0.1")
	c.Assert(containers[1].HostAddr, check.Equals, "127.0.0.1")

	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	coll := p.collection()
	err = coll.Update(bson.M{"id": toMoveCont.ID}, toMoveCont)
	c.Assert(err, check.IsNil)

	node1.PrepareFailure("createError", "/containers/create")
	node2.PrepareFailure("createError", "/containers/create")

	healer := containerHealer{provisioner: &p, maxUnresponsiveTime: 1 * time.Minute}
	healer.runContainerHealerOnce()

	containers, err = p.listAllContainers()
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
	coll := s.p.collection()
	err := coll.Insert(toMoveCont)
	c.Assert(err, check.IsNil)
	healer := containerHealer{provisioner: s.p}
	err = healer.healContainerIfNeeded(toMoveCont)
	c.Assert(err, check.ErrorMatches, "Containers healing: number of healings for container cont8 in the last 30 minutes exceeds limit of 3: 7")
	healingColl, err := healingCollection()
	var events []healingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 7)
}
