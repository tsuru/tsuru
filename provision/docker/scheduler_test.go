// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func checkContainerInContainerSlices(c container, cList []container) error {
	for _, cont := range cList {
		if cont.ID == c.ID {
			return nil
		}
	}
	return errors.New("container is not in list")
}

func (s *S) TestSchedulerSchedule(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	a2 := app.App{Name: "mirror", Teams: []string{"tsuruteam"}, Pool: "pool1"}
	a3 := app.App{Name: "dedication", Teams: []string{"nodockerforme"}, Pool: "pool1"}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	cont2 := container{ID: "2", Name: "mirror1", AppName: a2.Name}
	cont3 := container{ID: "3", Name: "dedication1", AppName: a3.Name}
	err := s.storage.Apps().Insert(a1, a2, a3)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{a1.Name, a2.Name, a3.Name}}})
	p := provision.Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	err = provision.AddPool(p.Name)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	defer provision.RemovePool(p.Name)
	contColl := s.p.collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name}}})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url1:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url2:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, "http://url0:1234")
	opts = docker.CreateContainerOptions{Name: cont2.Name}
	node, err = scheduler.Schedule(clusterInstance, opts, a2.Name)
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, "http://url1:1234")
	opts = docker.CreateContainerOptions{Name: cont3.Name}
	node, err = scheduler.Schedule(clusterInstance, opts, a3.Name)
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, "http://url2:1234")
}

func (s *S) TestSchedulerScheduleByTeamOwner(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{}, TeamOwner: "tsuruteam"}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{"tsuruteam"}}
	err = provision.AddPool(p.Name)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(p.Name)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	c.Assert(err, check.IsNil)
	contColl := s.p.collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": cont1.Name})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, "http://url0:1234")
}

func (s *S) TestSchedulerScheduleByTeams(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nopool"}}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{"tsuruteam"}}
	err = provision.AddPool(p.Name)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(p.Name)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	c.Assert(err, check.IsNil)
	contColl := s.p.collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": cont1.Name})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, "http://url0:1234")
}

func (s *S) TestSchedulerScheduleNoName(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	a2 := app.App{Name: "mirror", Teams: []string{"tsuruteam"}, Pool: "pool1"}
	a3 := app.App{Name: "dedication", Teams: []string{"nodockerforme"}, Pool: "pool1"}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	cont2 := container{ID: "2", Name: "mirror1", AppName: a2.Name}
	cont3 := container{ID: "3", Name: "dedication1", AppName: a3.Name}
	err := s.storage.Apps().Insert(a1, a2, a3)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{a1.Name, a2.Name, a3.Name}}})
	p := provision.Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	err = provision.AddPool(p.Name)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	defer provision.RemovePool(p.Name)
	contColl := s.p.collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name}}})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url1:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url2:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, "http://url0:1234")
	container, err := s.p.getContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.HostAddr, check.Equals, "")
}

func (s *S) TestSchedulerScheduleFallback(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{}}
	err = provision.AddPool(p.Name)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(p.Name)
	contColl := s.p.collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": cont1.Name})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, "http://url0:1234")
}

func (s *S) TestSchedulerNoFallback(c *check.C) {
	provision.RemovePool("test-fallback")
	a := app.App{Name: "bill", Teams: []string{"jean"}}
	err := s.storage.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": a.Name})
	cont1 := container{ID: "1", Name: "bill", AppName: a.Name}
	contColl := s.p.collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	defer contColl.Remove(bson.M{"name": cont1.Name})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, a.Name)
	c.Assert(node.Address, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, errNoFallback)
}

func (s *S) TestSchedulerNoNodesNoPool(c *check.C) {
	provision.RemovePool("test-fallback")
	app := app.App{Name: "bill", Teams: []string{"jean"}}
	err := s.storage.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app.Name})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{}
	node, err := scheduler.Schedule(clusterInstance, opts, app.Name)
	c.Assert(node.Address, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, errNoFallback)
}

func (s *S) TestSchedulerNoNodesWithFallbackPool(c *check.C) {
	provision.RemovePool("test-fallback")
	app := app.App{Name: "bill", Teams: []string{"jean"}}
	err := s.storage.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app.Name})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	err = provision.AddPool("mypool")
	c.Assert(err, check.IsNil)
	err = provision.AddPool("mypool2")
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("mypool")
	defer provision.RemovePool("mypool2")
	opts := docker.CreateContainerOptions{}
	node, err := scheduler.Schedule(clusterInstance, opts, app.Name)
	c.Assert(node.Address, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Matches, "No nodes found with one of the following metadata: pool=mypool, pool=mypool2")
}

func (s *S) TestSchedulerScheduleWithMemoryAwareness(c *check.C) {
	logBuf := bytes.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(logBuf, false))
	defer log.SetLogger(nil)
	app1 := app.App{Name: "skyrim", Plan: app.Plan{Memory: 60000}, Pool: "mypool"}
	err := s.storage.Apps().Insert(app1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app1.Name})
	app2 := app.App{Name: "oblivion", Plan: app.Plan{Memory: 20000}, Pool: "mypool"}
	err = s.storage.Apps().Insert(app2)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app2.Name})
	segSched := segregatedScheduler{
		maxMemoryRatio:      0.8,
		totalMemoryMetadata: "totalMemory",
		provisioner:         s.p,
	}
	err = provision.AddPool("mypool")
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("mypool")
	clusterInstance, err := cluster.New(&segSched, &cluster.MapStorage{},
		cluster.Node{Address: "http://server1:1234", Metadata: map[string]string{
			"totalMemory": "100000",
			"pool":        "mypool",
		}},
		cluster.Node{Address: "http://server2:1234", Metadata: map[string]string{
			"totalMemory": "100000",
			"pool":        "mypool",
		}},
	)
	c.Assert(err, check.Equals, nil)
	s.p.cluster = clusterInstance
	cont1 := container{ID: "pre1", Name: "existingUnit1", AppName: "skyrim", HostAddr: "server1"}
	contColl := s.p.collection()
	defer contColl.Close()
	defer contColl.RemoveAll(bson.M{"appname": "skyrim"})
	defer contColl.RemoveAll(bson.M{"appname": "oblivion"})
	err = contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	for i := 0; i < 5; i++ {
		cont := container{ID: string(i), Name: fmt.Sprintf("unit%d", i), AppName: "oblivion"}
		err := contColl.Insert(cont)
		c.Assert(err, check.IsNil)
		opts := docker.CreateContainerOptions{
			Name: cont.Name,
		}
		node, err := segSched.Schedule(clusterInstance, opts, cont.AppName)
		c.Assert(err, check.IsNil)
		c.Assert(node, check.NotNil)
	}
	n, err := contColl.Find(bson.M{"hostaddr": "server1"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 2)
	n, err = contColl.Find(bson.M{"hostaddr": "server2"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 4)
	n, err = contColl.Find(bson.M{"hostaddr": "server1", "appname": "oblivion"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 1)
	n, err = contColl.Find(bson.M{"hostaddr": "server2", "appname": "oblivion"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 4)
	cont := container{ID: "post-error", Name: "post-error-1", AppName: "oblivion"}
	err = contColl.Insert(cont)
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{
		Name: cont.Name,
	}
	node, err := segSched.Schedule(clusterInstance, opts, cont.AppName)
	c.Assert(err, check.IsNil)
	c.Assert(node, check.NotNil)
	c.Assert(logBuf.String(), check.Matches, `(?s).*WARNING: no nodes found with enough memory for container of "oblivion": 0.0191MB.*`)
}

func (s *S) TestChooseNodeDistributesNodesEqually(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
		{Address: "http://server3:1234"},
		{Address: "http://server4:1234"},
	}
	contColl := s.p.collection()
	defer contColl.Close()
	defer contColl.RemoveAll(bson.M{"appname": "coolapp9"})
	cont1 := container{ID: "pre1", Name: "existingUnit1", AppName: "coolapp9", HostAddr: "server1"}
	err := contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	cont2 := container{ID: "pre2", Name: "existingUnit2", AppName: "coolapp9", HostAddr: "server2"}
	err = contColl.Insert(cont2)
	c.Assert(err, check.Equals, nil)
	numberOfUnits := 38
	unitsPerNode := (numberOfUnits + 2) / 4
	wg := sync.WaitGroup{}
	wg.Add(numberOfUnits)
	sched := segregatedScheduler{provisioner: s.p}
	for i := 0; i < numberOfUnits; i++ {
		go func(i int) {
			defer wg.Done()
			cont := container{ID: string(i), Name: fmt.Sprintf("unit%d", i), AppName: "coolapp9"}
			err := contColl.Insert(cont)
			c.Assert(err, check.IsNil)
			node, err := sched.chooseNode(nodes, cont.Name, "coolapp9")
			c.Assert(err, check.IsNil)
			c.Assert(node, check.NotNil)
		}(i)
	}
	wg.Wait()
	n, err := contColl.Find(bson.M{"hostaddr": "server1"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, unitsPerNode)
	n, err = contColl.Find(bson.M{"hostaddr": "server2"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, unitsPerNode)
	n, err = contColl.Find(bson.M{"hostaddr": "server3"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, unitsPerNode)
	n, err = contColl.Find(bson.M{"hostaddr": "server4"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, unitsPerNode)
}

func (s *S) TestChooseNodeDistributesNodesEquallyDifferentApps(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
	}
	contColl := s.p.collection()
	defer contColl.Close()
	defer contColl.RemoveAll(bson.M{"appname": "skyrim"})
	defer contColl.RemoveAll(bson.M{"appname": "oblivion"})
	cont1 := container{ID: "pre1", Name: "existingUnit1", AppName: "skyrim", HostAddr: "server1"}
	err := contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	cont2 := container{ID: "pre2", Name: "existingUnit2", AppName: "skyrim", HostAddr: "server1"}
	err = contColl.Insert(cont2)
	c.Assert(err, check.Equals, nil)
	numberOfUnits := 2
	wg := sync.WaitGroup{}
	wg.Add(numberOfUnits)
	sched := segregatedScheduler{provisioner: s.p}
	for i := 0; i < numberOfUnits; i++ {
		go func(i int) {
			defer wg.Done()
			cont := container{ID: string(i), Name: fmt.Sprintf("unit%d", i), AppName: "oblivion"}
			err := contColl.Insert(cont)
			c.Assert(err, check.IsNil)
			node, err := sched.chooseNode(nodes, cont.Name, "oblivion")
			c.Assert(err, check.IsNil)
			c.Assert(node, check.NotNil)
		}(i)
	}
	wg.Wait()
	n, err := contColl.Find(bson.M{"hostaddr": "server1"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 3)
	n, err = contColl.Find(bson.M{"hostaddr": "server2"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 1)
	n, err = contColl.Find(bson.M{"hostaddr": "server1", "appname": "oblivion"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 1)
	n, err = contColl.Find(bson.M{"hostaddr": "server2", "appname": "oblivion"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 1)
}

func (s *S) TestChooseContainerToBeRemoved(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
	}
	contColl := s.p.collection()
	defer contColl.Close()
	defer contColl.RemoveAll(bson.M{"appname": "coolapp9"})
	cont1 := container{
		ID:       "pre1",
		Name:     "existingUnit1",
		AppName:  "coolapp9",
		HostAddr: "server1"}
	err := contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	cont2 := container{
		ID:       "pre2",
		Name:     "existingUnit2",
		AppName:  "coolapp9",
		HostAddr: "server2"}
	err = contColl.Insert(cont2)
	c.Assert(err, check.Equals, nil)
	cont3 := container{
		ID:       "pre3",
		Name:     "existingUnit1",
		AppName:  "coolapp9",
		HostAddr: "server1"}
	err = contColl.Insert(cont3)
	c.Assert(err, check.Equals, nil)
	scheduler := segregatedScheduler{provisioner: s.p}
	containerID, err := scheduler.chooseContainerFromMaxContainersCountInNode(nodes, "coolapp9")
	c.Assert(err, check.IsNil)
	c.Assert(containerID, check.Equals, "pre1")
}

func (s *S) TestGetContainerFromHost(c *check.C) {
	contColl := s.p.collection()
	defer contColl.Close()
	defer contColl.RemoveAll(bson.M{"appname": "coolapp9"})
	cont1 := container{
		ID:       "pre1",
		Name:     "existingUnit1",
		AppName:  "coolapp9",
		HostAddr: "server1"}
	err := contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	scheduler := segregatedScheduler{provisioner: s.p}
	id, err := scheduler.getContainerFromHost("server1", "coolapp9")
	c.Assert(err, check.IsNil)
	c.Assert(id, check.Equals, "pre1")
	_, err = scheduler.getContainerFromHost("server2", "coolapp9")
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetRemovableContainer(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	cont2 := container{ID: "2", Name: "mirror1", AppName: a1.Name}
	a2 := app.App{Name: "notimpius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	cont3 := container{ID: "3", Name: "dedication1", AppName: a2.Name}
	cont4 := container{ID: "4", Name: "dedication2", AppName: a2.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	err = s.storage.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	defer s.storage.Apps().RemoveAll(bson.M{"name": a2.Name})
	p := provision.Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	err = provision.AddPool(p.Name)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(p.Name, p.Teams)
	defer provision.RemovePool(p.Name)
	contColl := s.p.collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3, cont4,
	)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name, cont4.Name}}})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url1:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	opts = docker.CreateContainerOptions{Name: cont2.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	opts = docker.CreateContainerOptions{Name: cont3.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, a2.Name)
	c.Assert(err, check.IsNil)
	opts = docker.CreateContainerOptions{Name: cont4.Name}
	_, err = scheduler.Schedule(clusterInstance, opts, a2.Name)
	c.Assert(err, check.IsNil)
	cont, err := scheduler.GetRemovableContainer(a1.Name, clusterInstance)
	c.Assert(err, check.IsNil)
	cs := container{ID: cont}
	var contList []container
	err = contColl.Find(bson.M{"appname": a1.Name}).Select(bson.M{"id": 1}).All(&contList)
	c.Assert(err, check.IsNil)
	err = checkContainerInContainerSlices(cs, contList)
	c.Assert(err, check.IsNil)
}

func (s *S) TestNodesToHosts(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
	}
	scheduler := segregatedScheduler{provisioner: s.p}
	hosts, hostsMap := scheduler.nodesToHosts(nodes)
	c.Assert(hosts, check.NotNil)
	c.Assert(hostsMap, check.NotNil)
	c.Assert(len(hosts), check.Equals, 2)
	c.Assert(hostsMap[hosts[0]], check.Equals, nodes[0].Address)
}

func (s *S) TestChooseContainerToBeRemovedMultipleApps(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
	}
	contColl := s.p.collection()
	defer contColl.Close()
	cont1 := container{ID: "pre1", AppName: "coolapp1", HostAddr: "server1"}
	err := contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	cont2 := container{ID: "pre2", AppName: "coolapp1", HostAddr: "server1"}
	err = contColl.Insert(cont2)
	c.Assert(err, check.IsNil)
	cont3 := container{ID: "pre3", AppName: "coolapp1", HostAddr: "server1"}
	err = contColl.Insert(cont3)
	c.Assert(err, check.IsNil)
	cont4 := container{ID: "pre4", AppName: "coolapp2", HostAddr: "server1"}
	err = contColl.Insert(cont4)
	c.Assert(err, check.IsNil)
	cont5 := container{ID: "pre5", AppName: "coolapp2", HostAddr: "server2"}
	err = contColl.Insert(cont5)
	c.Assert(err, check.IsNil)
	cont6 := container{ID: "pre6", AppName: "coolapp2", HostAddr: "server2"}
	err = contColl.Insert(cont6)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	containerID, err := scheduler.chooseContainerFromMaxContainersCountInNode(nodes, "coolapp2")
	c.Assert(err, check.IsNil)
	c.Assert(containerID == "pre5" || containerID == "pre6", check.Equals, true)
}
