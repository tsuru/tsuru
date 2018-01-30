// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

func newFakeAppInDB(name, platform string, units int) *provisiontest.FakeApp {
	a := provisiontest.NewFakeApp(name, platform, units)
	appStruct := &app.App{
		Name:     a.GetName(),
		Platform: a.GetPlatform(),
	}
	conn, err := db.Conn()
	if err == nil {
		defer conn.Close()
		conn.Apps().Insert(appStruct)
	}
	return a
}

func (s *S) TestHealContainer(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	containers := []container.Container{
		{Container: types.Container{ID: "cont1", AppName: "app1"}},
		{Container: types.Container{ID: "cont2", AppName: "app1"}},
		{Container: types.Container{ID: "cont3", AppName: "app2"}},
	}
	p.SetContainers("localhost", containers)
	locker := dockertest.NewFakeLocker()
	locked := locker.Lock(containers[0].AppName)
	c.Assert(locked, check.Equals, true)
	defer locker.Unlock(containers[0].AppName)
	healer := NewContainerHealer(ContainerHealerArgs{Provisioner: p, Locker: locker})
	_, err = healer.healContainer(containers[0])
	c.Assert(err, check.IsNil)
	expected := []container.Container{
		{Container: types.Container{ID: "cont1-recreated", HostAddr: "localhost", AppName: "app1"}},
		{Container: types.Container{ID: "cont2", AppName: "app1", HostAddr: "localhost"}},
		{Container: types.Container{ID: "cont3", AppName: "app2", HostAddr: "localhost"}},
	}
	c.Assert(p.Containers("localhost"), check.DeepEquals, expected)
}

func (s *S) TestRunContainerHealer(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	app := newFakeAppInDB("myapp", "python", 2)
	node1 := p.Servers()[0]
	containers, err := p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	node1.MutateContainer(containers[0].ID, docker.State{Running: false, Restarting: false})
	node1.MutateContainer(containers[1].ID, docker.State{Running: false, Restarting: false})

	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().UTC().Add(-5 * time.Minute)
	p.PrepareListResult([]container.Container{containers[0], toMoveCont}, nil)

	node1.PrepareFailure("createError", "/containers/create")

	healer := NewContainerHealer(ContainerHealerArgs{
		Provisioner:         p,
		MaxUnresponsiveTime: time.Minute,
		Locker:              dockertest.NewFakeLocker(),
	})
	healer.runContainerHealerOnce()

	expected := []dockertest.ContainerMoving{
		{
			ContainerID: toMoveCont.ID,
			HostFrom:    toMoveCont.HostAddr,
			HostTo:      "",
		},
	}
	movings := p.Movings()
	c.Assert(movings, check.DeepEquals, expected)
	queries := p.Queries()
	c.Assert(queries, check.HasLen, 1)
	queryTime := queries[0]["lastsuccessstatusupdate"].(bson.M)["$lt"].(time.Time)
	delete(queries[0], "lastsuccessstatusupdate")
	c.Assert(time.Now().UTC().Add(-1*time.Minute).Sub(queryTime) < time.Second, check.Equals, true)
	c.Assert(queries, check.DeepEquals, []bson.M{{
		"id":      bson.M{"$ne": ""},
		"appname": bson.M{"$ne": ""},
		"$or": []bson.M{
			{"hostport": bson.M{"$ne": ""}},
			{"processname": bson.M{"$ne": ""}},
		},
		"status": bson.M{"$nin": []string{
			provision.StatusBuilding.String(),
			provision.StatusAsleep.String(),
		}},
	}})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "container", Value: toMoveCont.ID},
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: "myapp"}},
			{Target: event.Target{Type: "container", Value: toMoveCont.ID + "-recreated"}},
		},
		Kind: "healer",
		StartCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       toMoveCont.ID,
		},
		EndCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       bson.M{"$ne": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunContainerHealerCreatedContainer(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	app := newFakeAppInDB("myapp", "python", 2)
	node1 := p.Servers()[0]
	containers, err := p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	node1.MutateContainer(containers[0].ID, docker.State{Running: false, Restarting: false})
	node1.MutateContainer(containers[1].ID, docker.State{Running: false, Restarting: false})
	toMoveCont := containers[1]
	toMoveCont.MongoID = bson.NewObjectIdWithTime(time.Now().Add(-2 * time.Minute))
	p.PrepareListResult([]container.Container{containers[0], toMoveCont}, nil)
	node1.PrepareFailure("createError", "/containers/create")
	healer := NewContainerHealer(ContainerHealerArgs{
		Provisioner:         p,
		MaxUnresponsiveTime: time.Minute,
		Locker:              dockertest.NewFakeLocker(),
	})
	healer.runContainerHealerOnce()
	expected := []dockertest.ContainerMoving{
		{
			ContainerID: toMoveCont.ID,
			HostFrom:    toMoveCont.HostAddr,
			HostTo:      "",
		},
	}
	movings := p.Movings()
	c.Assert(movings, check.DeepEquals, expected)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "container", Value: toMoveCont.ID},
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: "myapp"}},
			{Target: event.Target{Type: "container", Value: toMoveCont.ID + "-recreated"}},
		},
		Kind: "healer",
		StartCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       toMoveCont.ID,
		},
		EndCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       bson.M{"$ne": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunContainerHealerStoppedContainer(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	app := newFakeAppInDB("myapp", "python", 2)
	node1 := p.Servers()[0]
	containers, err := p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	node1.MutateContainer(containers[0].ID, docker.State{Running: false, Restarting: false})
	node1.MutateContainer(containers[1].ID, docker.State{Dead: true})
	toMoveCont := containers[1]
	err = toMoveCont.SetStatus(p.ClusterClient(), provision.StatusStopped, false)
	c.Assert(err, check.IsNil)
	toMoveCont.LastSuccessStatusUpdate = time.Now().UTC().Add(-5 * time.Minute)
	p.PrepareListResult([]container.Container{containers[0], toMoveCont}, nil)
	node1.PrepareFailure("createError", "/containers/create")
	healer := NewContainerHealer(ContainerHealerArgs{
		Provisioner:         p,
		MaxUnresponsiveTime: time.Minute,
		Locker:              dockertest.NewFakeLocker(),
	})
	healer.runContainerHealerOnce()
	expected := []dockertest.ContainerMoving{
		{
			ContainerID: toMoveCont.ID,
			HostFrom:    toMoveCont.HostAddr,
			HostTo:      "",
		},
	}
	movings := p.Movings()
	c.Assert(movings, check.DeepEquals, expected)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "container", Value: toMoveCont.ID},
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: "myapp"}},
			{Target: event.Target{Type: "container", Value: toMoveCont.ID + "-recreated"}},
		},
		Kind: "healer",
		StartCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       toMoveCont.ID,
		},
		EndCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       bson.M{"$ne": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunContainerHealerStoppedContainerAlreadyStopped(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	app := newFakeAppInDB("myapp", "python", 2)
	node1 := p.Servers()[0]
	containers, err := p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	node1.MutateContainer(containers[0].ID, docker.State{Running: false, Restarting: false})
	node1.MutateContainer(containers[1].ID, docker.State{Running: false, Restarting: false})
	toMoveCont := containers[1]
	err = toMoveCont.SetStatus(p.ClusterClient(), provision.StatusStopped, false)
	c.Assert(err, check.IsNil)
	toMoveCont.LastSuccessStatusUpdate = time.Now().UTC().Add(-5 * time.Minute)
	p.PrepareListResult([]container.Container{containers[0], toMoveCont}, nil)
	node1.PrepareFailure("createError", "/containers/create")
	healer := NewContainerHealer(ContainerHealerArgs{
		Provisioner:         p,
		MaxUnresponsiveTime: time.Minute,
		Locker:              dockertest.NewFakeLocker(),
	})
	healer.runContainerHealerOnce()
	movings := p.Movings()
	c.Assert(movings, check.IsNil)
	c.Assert(eventtest.EventDesc{
		IsEmpty: true,
	}, eventtest.HasEvent)
	conts, err := p.ListContainers(bson.M{"id": toMoveCont.ID})
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 1)
	c.Assert(conts[0].Status, check.Equals, provision.StatusStopped.String())
	c.Assert(time.Since(conts[0].LastSuccessStatusUpdate) < time.Minute, check.Equals, true)
}

func (s *S) TestRunContainerHealerCreatedContainerNoProcess(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	app := newFakeAppInDB("myapp", "python", 2)
	containers, err := p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  p.Servers()[0].URL(),
		App:       app,
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	notToMove := containers[1]
	notToMove.MongoID = bson.NewObjectIdWithTime(time.Now().Add(-2 * time.Minute))
	notToMove.ProcessName = ""
	p.PrepareListResult([]container.Container{containers[0], notToMove}, nil)
	healer := NewContainerHealer(ContainerHealerArgs{
		Provisioner:         p,
		MaxUnresponsiveTime: time.Minute,
		Locker:              dockertest.NewFakeLocker(),
	})
	healer.runContainerHealerOnce()
	movings := p.Movings()
	c.Assert(movings, check.IsNil)
	c.Assert(eventtest.EventDesc{
		IsEmpty: true,
	}, eventtest.HasEvent)
}

func (s *S) TestRunContainerHealerShutdown(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()

	node1 := p.Servers()[0]
	app := newFakeAppInDB("myapp", "python", 0)
	_, err = p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)

	containers := p.AllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, net.URLToHost(node1.URL()))
	c.Assert(containers[1].HostAddr, check.Equals, net.URLToHost(node1.URL()))
	node1.MutateContainer(containers[0].ID, docker.State{Running: false, Restarting: false})
	node1.MutateContainer(containers[1].ID, docker.State{Running: false, Restarting: false})

	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-5 * time.Minute)

	p.PrepareListResult([]container.Container{containers[0], toMoveCont}, nil)
	node1.PrepareFailure("createError", "/containers/create")

	healer := NewContainerHealer(ContainerHealerArgs{
		Provisioner:         p,
		MaxUnresponsiveTime: time.Minute,
		Done:                make(chan bool),
		Locker:              dockertest.NewFakeLocker(),
	})
	ch := make(chan bool)
	go func() {
		defer close(ch)
		healer.RunContainerHealer()
	}()
	healer.Shutdown(context.Background())
	<-ch

	expected := dockertest.ContainerMoving{
		ContainerID: toMoveCont.ID,
		HostFrom:    toMoveCont.HostAddr,
		HostTo:      "",
	}
	movings := p.Movings()
	c.Assert(movings, check.DeepEquals, []dockertest.ContainerMoving{expected})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "container", Value: toMoveCont.ID},
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: "myapp"}},
			{Target: event.Target{Type: "container", Value: toMoveCont.ID + "-recreated"}},
		},
		Kind: "healer",
		StartCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       toMoveCont.ID,
		},
		EndCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       bson.M{"$ne": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunContainerHealerConcurrency(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := newFakeAppInDB("myapp", "python", 0)
	_, err = p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)

	containers := p.AllContainers()
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, net.URLToHost(node1.URL()))
	c.Assert(containers[1].HostAddr, check.Equals, net.URLToHost(node1.URL()))
	node1.MutateContainer(containers[0].ID, docker.State{Running: false, Restarting: false})
	node1.MutateContainer(containers[1].ID, docker.State{Running: false, Restarting: false})
	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-5 * time.Minute)

	p.PrepareListResult([]container.Container{containers[0], toMoveCont}, nil)
	node1.PrepareFailure("createError", "/containers/create")

	healer := NewContainerHealer(ContainerHealerArgs{Provisioner: p, Locker: dockertest.NewFakeLocker()})
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

	expected := dockertest.ContainerMoving{
		ContainerID: toMoveCont.ID,
		HostFrom:    toMoveCont.HostAddr,
		HostTo:      "",
	}
	movings := p.Movings()
	c.Assert(movings, check.DeepEquals, []dockertest.ContainerMoving{expected})

	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "container", Value: toMoveCont.ID},
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: "myapp"}},
			{Target: event.Target{Type: "container", Value: toMoveCont.ID + "-recreated"}},
		},
		Kind: "healer",
		StartCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       toMoveCont.ID,
		},
		EndCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       bson.M{"$ne": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunContainerHealerAlreadyHealed(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := newFakeAppInDB("myapp", "python", 0)
	_, err = p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	containers := p.AllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, net.URLToHost(node1.URL()))
	c.Assert(containers[1].HostAddr, check.Equals, net.URLToHost(node1.URL()))
	node1.MutateContainer(containers[0].ID, docker.State{Running: false, Restarting: false})
	node1.MutateContainer(containers[1].ID, docker.State{Running: false, Restarting: false})
	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-5 * time.Minute)
	p.PrepareListResult([]container.Container{containers[0], toMoveCont}, nil)
	node1.PrepareFailure("createError", "/containers/create")
	healer := NewContainerHealer(ContainerHealerArgs{Provisioner: p, Locker: dockertest.NewFakeLocker()})
	err = healer.healContainerIfNeeded(toMoveCont)
	c.Assert(err, check.IsNil)
	err = healer.healContainerIfNeeded(toMoveCont)
	c.Assert(err, check.IsNil)
	expected := dockertest.ContainerMoving{
		ContainerID: toMoveCont.ID,
		HostFrom:    toMoveCont.HostAddr,
		HostTo:      "",
	}
	movings := p.Movings()
	c.Assert(movings, check.DeepEquals, []dockertest.ContainerMoving{expected})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "container", Value: toMoveCont.ID},
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: "myapp"}},
			{Target: event.Target{Type: "container", Value: toMoveCont.ID + "-recreated"}},
		},
		Kind: "healer",
		StartCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       toMoveCont.ID,
		},
		EndCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       bson.M{"$ne": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunContainerHealerRemovedFromDB(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := newFakeAppInDB("myapp", "python", 0)
	_, err = p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 1},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	containers := p.AllContainers()
	c.Assert(err, check.IsNil)
	p.DeleteContainer(containers[0].ID)
	node1.MutateContainer(containers[0].ID, docker.State{Running: false, Restarting: false})
	toMoveCont := containers[0]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-5 * time.Minute)
	p.PrepareListResult([]container.Container{containers[0], toMoveCont}, nil)
	healer := NewContainerHealer(ContainerHealerArgs{Provisioner: p, Locker: dockertest.NewFakeLocker()})
	err = healer.healContainerIfNeeded(toMoveCont)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRunContainerHealerDoesntHealWhenContainerIsRunning(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := newFakeAppInDB("myapp", "python", 0)
	cont, err := p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 1},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	node1.MutateContainer(cont[0].ID, docker.State{Running: true, Restarting: false})

	toMoveCont := cont[0]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	p.PrepareListResult([]container.Container{toMoveCont}, nil)

	healer := NewContainerHealer(ContainerHealerArgs{
		Provisioner:         p,
		MaxUnresponsiveTime: time.Minute,
		Locker:              dockertest.NewFakeLocker(),
	})
	healer.runContainerHealerOnce()
	c.Assert(eventtest.EventDesc{
		IsEmpty: true,
	}, eventtest.HasEvent)
}

func (s *S) TestRunContainerHealerDoesntHealWhenContainerIsRestarting(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := newFakeAppInDB("myapp", "python", 0)
	cont, err := p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 1},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)
	node1.MutateContainer(cont[0].ID, docker.State{Running: false, Restarting: true})

	toMoveCont := cont[0]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	p.PrepareListResult([]container.Container{toMoveCont}, nil)

	healer := NewContainerHealer(ContainerHealerArgs{
		Provisioner:         p,
		MaxUnresponsiveTime: time.Minute,
		Locker:              dockertest.NewFakeLocker(),
	})
	healer.runContainerHealerOnce()
	c.Assert(eventtest.EventDesc{
		IsEmpty: true,
	}, eventtest.HasEvent)
}

func (s *S) TestRunContainerHealerWithError(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := newFakeAppInDB("myapp", "python", 0)
	_, err = p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)

	containers := p.AllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, net.URLToHost(node1.URL()))
	c.Assert(containers[1].HostAddr, check.Equals, net.URLToHost(node1.URL()))
	node1.MutateContainer(containers[0].ID, docker.State{Running: false, Restarting: false})
	node1.MutateContainer(containers[1].ID, docker.State{Running: false, Restarting: false})

	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	p.PrepareListResult([]container.Container{containers[0], toMoveCont}, nil)
	p.FailMove(
		errors.New("cannot move container"),
		errors.New("cannot move container"),
		errors.New("cannot move container"),
	)

	healer := NewContainerHealer(ContainerHealerArgs{
		Provisioner:         p,
		MaxUnresponsiveTime: time.Minute,
		Locker:              dockertest.NewFakeLocker(),
	})
	healer.runContainerHealerOnce()

	containers = p.AllContainers()
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	hosts := []string{containers[0].HostAddr, containers[1].HostAddr}
	c.Assert(hosts[0], check.Equals, net.URLToHost(node1.URL()))
	c.Assert(hosts[1], check.Equals, net.URLToHost(node1.URL()))

	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "container", Value: toMoveCont.ID},
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: "myapp"}},
		},
		Kind: "healer",
		StartCustomData: map[string]interface{}{
			"hostaddr": "127.0.0.1",
			"id":       toMoveCont.ID,
		},
		ErrorMatches: `.*Error trying to heal containers.*`,
		EndCustomData: map[string]interface{}{
			"hostaddr": "",
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunContainerHealerThrottled(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := newFakeAppInDB("myapp", "python", 0)
	_, err = p.StartContainers(dockertest.StartContainersArgs{
		Endpoint:  node1.URL(),
		App:       app,
		Amount:    map[string]int{"web": 2},
		Image:     "tsuru/python",
		PullImage: true,
	})
	c.Assert(err, check.IsNil)

	containers := p.AllContainers()
	c.Assert(containers, check.HasLen, 2)
	c.Assert(containers[0].HostAddr, check.Equals, net.URLToHost(node1.URL()))
	c.Assert(containers[1].HostAddr, check.Equals, net.URLToHost(node1.URL()))
	node1.MutateContainer(containers[0].ID, docker.State{Running: false, Restarting: false})
	node1.MutateContainer(containers[1].ID, docker.State{Running: false, Restarting: false})
	toMoveCont := containers[1]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-5 * time.Minute)
	for i := 0; i < 3; i++ {
		var evt *event.Event
		evt, err = event.NewInternal(&event.Opts{
			Target:       event.Target{Type: "container", Value: toMoveCont.ID},
			InternalKind: "healer",
			CustomData:   toMoveCont,
			Allowed:      event.Allowed(permission.PermAppReadEvents),
		})
		c.Assert(err, check.IsNil)
		err = evt.DoneCustomData(nil, nil)
		c.Assert(err, check.IsNil)
	}
	healer := NewContainerHealer(ContainerHealerArgs{Provisioner: p, Locker: dockertest.NewFakeLocker()})
	err = healer.healContainerIfNeeded(toMoveCont)
	c.Assert(err, check.ErrorMatches, "Error trying to insert container healing event, healing aborted: event throttled, limit for healer on any container is 3 every 5m0s")
}

func (s *S) TestListUnresponsiveContainers(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var result []container.Container
	coll := p.Collection()
	defer coll.Close()
	now := time.Now().UTC()
	coll.Insert(
		container.Container{Container: types.Container{ID: "c1", AppName: "app_time_test", ProcessName: "p", LastSuccessStatusUpdate: now, HostPort: "80"}},
		container.Container{Container: types.Container{ID: "c2", AppName: "app_time_test", ProcessName: "p", LastSuccessStatusUpdate: now.Add(-1 * time.Minute), HostPort: "80"}},
		container.Container{Container: types.Container{ID: "c3", AppName: "app_time_test", ProcessName: "p", LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80"}},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err = listUnresponsiveContainers(p, 3*time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].ID, check.Equals, "c3")
}

func (s *S) TestListUnresponsiveContainersNoHostPort(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var result []container.Container
	coll := p.Collection()
	defer coll.Close()
	now := time.Now().UTC()
	coll.Insert(
		container.Container{Container: types.Container{ID: "c1", AppName: "app_time_test", LastSuccessStatusUpdate: now.Add(-10 * time.Minute)}},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err = listUnresponsiveContainers(p, 3*time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 0)
}

func (s *S) TestListUnresponsiveContainersIncludeStopped(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var result []container.Container
	coll := p.Collection()
	defer coll.Close()
	now := time.Now().UTC()
	coll.Insert(
		container.Container{Container: types.Container{ID: "c1", AppName: "app_time_test",
			LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80", Status: provision.StatusStopped.String()}},
		container.Container{Container: types.Container{ID: "c2", AppName: "app_time_test",
			LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80", Status: provision.StatusStarted.String()}},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err = listUnresponsiveContainers(p, 3*time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 2)
	ids := []string{result[0].ID, result[1].ID}
	sort.Strings(ids)
	c.Assert(ids, check.DeepEquals, []string{"c1", "c2"})
}

func (s *S) TestListUnresponsiveContainersAsleep(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var result []container.Container
	coll := p.Collection()
	defer coll.Close()
	now := time.Now().UTC()
	coll.Insert(
		container.Container{Container: types.Container{ID: "c1", AppName: "app_time_test",
			LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80", Status: provision.StatusAsleep.String()}},
		container.Container{Container: types.Container{ID: "c2", AppName: "app_time_test",
			LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80", Status: provision.StatusStarted.String()}},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err = listUnresponsiveContainers(p, 3*time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.HasLen, 1)
	c.Assert(result[0].ID, check.Equals, "c2")
}

func (s *S) TestListUnresponsiveContainersRotateResults(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	coll := p.Collection()
	defer coll.Close()
	now := time.Now().UTC()
	coll.Insert(
		container.Container{Container: types.Container{ID: "c1", AppName: "app_time_test", ProcessName: "p", LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80"}},
		container.Container{Container: types.Container{ID: "c2", AppName: "app_time_test", ProcessName: "p", LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80"}},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result1, err := listUnresponsiveContainers(p, 3*time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(result1, check.HasLen, 2)
	result2, err := listUnresponsiveContainers(p, 3*time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(result2, check.HasLen, 2)
	c.Assert(result1[0], check.DeepEquals, result2[1])
	c.Assert(result1[1], check.DeepEquals, result2[0])
}
