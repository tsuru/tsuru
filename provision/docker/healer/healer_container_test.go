// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"errors"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestHealContainer(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	containers := []container.Container{
		{ID: "cont1", AppName: "app1"},
		{ID: "cont2", AppName: "app1"},
		{ID: "cont3", AppName: "app2"},
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
		{ID: "cont1-recreated", HostAddr: "localhost", AppName: "app1"},
		{ID: "cont2", AppName: "app1", HostAddr: "localhost"},
		{ID: "cont3", AppName: "app2", HostAddr: "localhost"},
	}
	c.Assert(p.Containers("localhost"), check.DeepEquals, expected)
}

func (s *S) TestRunContainerHealer(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	app := provisiontest.NewFakeApp("myapp", "python", 2)
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
		"$or": []bson.M{
			{"hostport": bson.M{"$ne": ""}},
			{"processname": bson.M{"$ne": ""}},
		},
		"status": bson.M{"$nin": []string{
			provision.StatusStopped.String(),
			provision.StatusBuilding.String(),
		}},
	}})
	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "container-healing")
	c.Assert(events[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(events[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingContainer.HostAddr, check.Equals, "127.0.0.1")
	c.Assert(events[0].CreatedContainer.HostAddr, check.Equals, "127.0.0.1")
}

func (s *S) TestRunContainerHealerCreatedContainer(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	app := provisiontest.NewFakeApp("myapp", "python", 2)
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
	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "container-healing")
	c.Assert(events[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(events[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingContainer.HostAddr, check.Equals, "127.0.0.1")
	c.Assert(events[0].CreatedContainer.HostAddr, check.Equals, "127.0.0.1")
}

func (s *S) TestRunContainerHealerCreatedContainerNoProcess(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	app := provisiontest.NewFakeApp("myapp", "python", 2)
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
	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	n, err := healingColl.Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
}

func (s *S) TestRunContainerHealerShutdown(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()

	node1 := p.Servers()[0]
	app := provisiontest.NewFakeApp("myapp", "python", 0)
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
	healer.Shutdown()
	<-ch

	expected := dockertest.ContainerMoving{
		ContainerID: toMoveCont.ID,
		HostFrom:    toMoveCont.HostAddr,
		HostTo:      "",
	}
	movings := p.Movings()
	c.Assert(movings, check.DeepEquals, []dockertest.ContainerMoving{expected})
}

func (s *S) TestRunContainerHealerConcurrency(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := provisiontest.NewFakeApp("myapp", "python", 0)
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

	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "container-healing")
	c.Assert(events[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(events[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingContainer.HostAddr, check.Equals, "127.0.0.1")
	c.Assert(events[0].CreatedContainer.HostAddr, check.Equals, "127.0.0.1")
}

func (s *S) TestRunContainerHealerAlreadyHealed(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := provisiontest.NewFakeApp("myapp", "python", 0)
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
	healer.healContainerIfNeeded(toMoveCont)
	healer.healContainerIfNeeded(toMoveCont)

	expected := dockertest.ContainerMoving{
		ContainerID: toMoveCont.ID,
		HostFrom:    toMoveCont.HostAddr,
		HostTo:      "",
	}
	movings := p.Movings()
	c.Assert(movings, check.DeepEquals, []dockertest.ContainerMoving{expected})

	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "container-healing")
	c.Assert(events[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(events[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(events[0].Error, check.Equals, "")
	c.Assert(events[0].Successful, check.Equals, true)
	c.Assert(events[0].FailingContainer.HostAddr, check.Equals, "127.0.0.1")
	c.Assert(events[0].CreatedContainer.HostAddr, check.Equals, "127.0.0.1")
}

func (s *S) TestRunContainerHealerDoesntHealWhenContainerIsRunning(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := provisiontest.NewFakeApp("myapp", "python", 0)
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

	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 0)
}

func (s *S) TestRunContainerHealerDoesntHealWhenContainerIsRestarting(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := provisiontest.NewFakeApp("myapp", "python", 0)
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

	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 0)
}

func (s *S) TestRunContainerHealerWithError(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	node1 := p.Servers()[0]
	app := provisiontest.NewFakeApp("myapp", "python", 0)
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

	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].Action, check.Equals, "container-healing")
	c.Assert(events[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(events[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(events[0].Error, check.Matches, "(?s).*Error trying to heal containers.*")
	c.Assert(events[0].Successful, check.Equals, false)
	c.Assert(events[0].FailingContainer.HostAddr, check.Equals, "127.0.0.1")
	c.Assert(events[0].CreatedContainer.HostAddr, check.Equals, "")
}

func (s *S) TestRunContainerHealerMaxCounterExceeded(c *check.C) {
	conts := []container.Container{
		{ID: "cont1"}, {ID: "cont2"}, {ID: "cont3"}, {ID: "cont4"},
		{ID: "cont5"}, {ID: "cont6"}, {ID: "cont7"}, {ID: "cont8"},
	}
	for i := 0; i < len(conts)-1; i++ {
		evt, err := NewHealingEvent(conts[i])
		c.Assert(err, check.IsNil)
		err = evt.Update(conts[i+1], nil)
		c.Assert(err, check.IsNil)
	}
	toMoveCont := conts[7]
	toMoveCont.LastSuccessStatusUpdate = time.Now().Add(-2 * time.Minute)
	p, err := dockertest.NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	healer := NewContainerHealer(ContainerHealerArgs{Provisioner: p, Locker: dockertest.NewFakeLocker()})
	err = healer.healContainerIfNeeded(toMoveCont)
	c.Assert(err, check.ErrorMatches, "Containers healing: number of healings for container cont8 in the last 30 minutes exceeds limit of 3: 7")
	healingColl, err := healingCollection()
	c.Assert(err, check.IsNil)
	defer healingColl.Close()
	var events []HealingEvent
	err = healingColl.Find(nil).All(&events)
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 7)
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
		container.Container{ID: "c1", AppName: "app_time_test", ProcessName: "p", LastSuccessStatusUpdate: now, HostPort: "80"},
		container.Container{ID: "c2", AppName: "app_time_test", ProcessName: "p", LastSuccessStatusUpdate: now.Add(-1 * time.Minute), HostPort: "80"},
		container.Container{ID: "c3", AppName: "app_time_test", ProcessName: "p", LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80"},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err = listUnresponsiveContainers(p, 3*time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 1)
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
		container.Container{ID: "c1", AppName: "app_time_test", LastSuccessStatusUpdate: now.Add(-10 * time.Minute)},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err = listUnresponsiveContainers(p, 3*time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 0)
}

func (s *S) TestListUnresponsiveContainersStopped(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var result []container.Container
	coll := p.Collection()
	defer coll.Close()
	now := time.Now().UTC()
	coll.Insert(
		container.Container{ID: "c1", AppName: "app_time_test",
			LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80", Status: provision.StatusStopped.String()},
		container.Container{ID: "c2", AppName: "app_time_test",
			LastSuccessStatusUpdate: now.Add(-5 * time.Minute), HostPort: "80", Status: provision.StatusStarted.String()},
	)
	defer coll.RemoveAll(bson.M{"appname": "app_time_test"})
	result, err = listUnresponsiveContainers(p, 3*time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 1)
	c.Assert(result[0].ID, check.Equals, "c2")
}
