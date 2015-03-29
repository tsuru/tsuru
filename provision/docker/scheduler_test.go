// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/cmdtest"
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
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}}
	a2 := app.App{Name: "mirror", Teams: []string{"tsuruteam"}}
	a3 := app.App{Name: "dedication", Teams: []string{"nodockerforme"}}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	cont2 := container{ID: "2", Name: "mirror1", AppName: a2.Name}
	cont3 := container{ID: "3", Name: "dedication1", AppName: a3.Name}
	err := s.storage.Apps().Insert(a1, a2, a3)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{a1.Name, a2.Name, a3.Name}}})
	coll := s.storage.Collection(schedulerCollection)
	p := Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	err = coll.Insert(p)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := s.p.collection()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name}}})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
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

func (s *S) TestSchedulerScheduleNoName(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}}
	a2 := app.App{Name: "mirror", Teams: []string{"tsuruteam"}}
	a3 := app.App{Name: "dedication", Teams: []string{"nodockerforme"}}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	cont2 := container{ID: "2", Name: "mirror1", AppName: a2.Name}
	cont3 := container{ID: "3", Name: "dedication1", AppName: a3.Name}
	err := s.storage.Apps().Insert(a1, a2, a3)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{a1.Name, a2.Name, a3.Name}}})
	coll := s.storage.Collection(schedulerCollection)
	p := Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	err = coll.Insert(p)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := s.p.collection()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name}}})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
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
	coll := s.storage.Collection(schedulerCollection)
	p := Pool{Name: "pool1"}
	err = coll.Insert(p)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := s.p.collection()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": cont1.Name})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, "http://url0:1234")
}

func (s *S) TestSchedulerScheduleTeamOwner(c *check.C) {
	a1 := app.App{
		Name:      "impius",
		Teams:     []string{"nodockerforme"},
		TeamOwner: "tsuruteam",
	}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	coll := s.storage.Collection(schedulerCollection)
	p := Pool{Name: "pool1", Teams: []string{"tsuruteam"}}
	err = coll.Insert(p)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := s.p.collection()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": cont1.Name})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, "http://url0:1234")
}

func (s *S) TestSchedulerNoFallback(c *check.C) {
	app := app.App{Name: "bill", Teams: []string{"jean"}}
	err := s.storage.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app.Name})
	cont1 := container{ID: "1", Name: "bill", AppName: app.Name}
	contColl := s.p.collection()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	defer contColl.Remove(bson.M{"name": cont1.Name})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, app.Name)
	c.Assert(node.Address, check.Equals, "")
	c.Assert(err, check.Equals, errNoFallback)
}

func (s *S) TestSchedulerNoNodesNoPool(c *check.C) {
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{}
	node, err := scheduler.Schedule(clusterInstance, opts, "")
	c.Assert(node.Address, check.Equals, "")
	c.Assert(err, check.Equals, errNoFallback)
}

func (s *S) TestSchedulerNoNodesWithFallbackPool(c *check.C) {
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, check.IsNil)
	err = scheduler.addPool("mypool")
	c.Assert(err, check.IsNil)
	err = scheduler.addPool("mypool2")
	c.Assert(err, check.IsNil)
	defer scheduler.removePool("mypool")
	defer scheduler.removePool("mypool2")
	opts := docker.CreateContainerOptions{}
	node, err := scheduler.Schedule(clusterInstance, opts, "")
	c.Assert(node.Address, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Matches, "No nodes found with one of the following metadata: pool=mypool, pool=mypool2")
}

func (s *S) TestSchedulerScheduleWithMemoryAwareness(c *check.C) {
	app1 := app.App{Name: "skyrim", Plan: app.Plan{Memory: 60000}}
	err := s.storage.Apps().Insert(app1)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app1.Name})
	app2 := app.App{Name: "oblivion", Plan: app.Plan{Memory: 20000}}
	err = s.storage.Apps().Insert(app2)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app2.Name})
	segSched := segregatedScheduler{
		maxMemoryRatio:      0.8,
		totalMemoryMetadata: "totalMemory",
		provisioner:         s.p,
	}
	err = segSched.addPool("mypool")
	c.Assert(err, check.IsNil)
	defer segSched.removePool("mypool")
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
	cont1 := container{ID: "pre1", Name: "existingUnit1", AppName: "skyrim", HostAddr: "server1"}
	contColl := s.p.collection()
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
	_, err = segSched.Schedule(clusterInstance, opts, cont.AppName)
	c.Assert(err, check.ErrorMatches, "No nodes found with enough memory for container of \"oblivion\": 0.0191MB.")
}

func (s *S) TestChooseNodeDistributesNodesEqually(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
		{Address: "http://server3:1234"},
		{Address: "http://server4:1234"},
	}
	contColl := s.p.collection()
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

func (s *S) TestAddPool(c *check.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	defer coll.RemoveId("pool1")
	err := seg.addPool("pool1")
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddPoolWithoutNameShouldBreak(c *check.C) {
	var seg segregatedScheduler
	err := seg.addPool("")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Pool name is required.")
}

func (s *S) TestRemovePool(c *check.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	err = seg.removePool("pool1")
	c.Assert(err, check.IsNil)
	p, err := coll.FindId("pool1").Count()
	c.Assert(err, check.IsNil)
	c.Assert(p, check.Equals, 0)
}

func (s *S) TestAddTeamToPool(c *check.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = seg.addTeamsToPool("pool1", []string{"ateam", "test"})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Teams, check.DeepEquals, []string{"ateam", "test"})
}

func (s *S) TestAddTeamToPollWithTeams(c *check.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1", Teams: []string{"test", "ateam"}}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = seg.addTeamsToPool(pool.Name, []string{"pteam"})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Teams, check.DeepEquals, []string{"test", "ateam", "pteam"})
}

func (s *S) TestAddTeamToNonExistentPool(c *check.C) {
	var seg segregatedScheduler
	err := seg.addTeamsToPool("pool1", []string{"ateam"})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "pool not found")
}

func (s *S) TestAddTeamToPollShouldNotAcceptDuplicatedTeam(c *check.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1", Teams: []string{"test", "ateam"}}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = seg.addTeamsToPool(pool.Name, []string{"ateam"})
	c.Assert(err, check.NotNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Teams, check.DeepEquals, []string{"test", "ateam"})
}

func (s *S) TestRemoveTeamsFromPool(c *check.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1", Teams: []string{"test", "ateam"}}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = seg.removeTeamsFromPool(pool.Name, []string{"test"})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Teams, check.DeepEquals, []string{"ateam"})
}

func (s *S) TestAddPoolToSchedulerCmdInfo(c *check.C) {
	expected := cmd.Info{
		Name:    "docker-pool-add",
		Usage:   "docker-pool-add <pool>",
		Desc:    "Add a pool to cluster",
		MinArgs: 1,
	}
	cmd := addPoolToSchedulerCmd{}
	c.Assert(cmd.Info(), check.DeepEquals, &expected)
}

func (s *S) TestAddPoolToTheSchedulerCmd(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"poolTest"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/pool"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := addPoolToSchedulerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemovePoolFromSchedulerCmdInfo(c *check.C) {
	expected := cmd.Info{
		Name:    "docker-pool-remove",
		Usage:   "docker-pool-remove <pool> [-y]",
		Desc:    "Remove a pool to cluster",
		MinArgs: 1,
	}
	cmd := removePoolFromSchedulerCmd{}
	c.Assert(cmd.Info(), check.DeepEquals, &expected)
}

func (s *S) TestRemovePoolFromTheSchedulerCmd(c *check.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"poolTest"}, Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/pool"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := removePoolFromSchedulerCmd{}
	cmd.Flags().Parse(true, []string{"-y"})
	err := cmd.Run(&context, client)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemovePoolFromTheSchedulerCmdConfirmation(c *check.C) {
	var stdout bytes.Buffer
	context := cmd.Context{
		Args:   []string{"poolX"},
		Stdout: &stdout,
		Stdin:  strings.NewReader("n\n"),
	}
	command := removePoolFromSchedulerCmd{}
	err := command.Run(&context, nil)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "Are you sure you want to remove \"poolX\" pool? (y/n) Abort.\n")
}

func (s *S) TestListPoolsInTheSchedulerCmdInfo(c *check.C) {
	expected := cmd.Info{
		Name:  "docker-pool-list",
		Usage: "docker-pool-list",
		Desc:  "List available pools in the cluster",
	}
	cmd := listPoolsInTheSchedulerCmd{}
	c.Assert(cmd.Info(), check.DeepEquals, &expected)
}

func (s *S) TestListPoolsInTheSchedulerCmdRun(c *check.C) {
	var buf bytes.Buffer
	pool := Pool{Name: "pool1", Teams: []string{"tsuruteam", "ateam"}}
	pools := []Pool{pool}
	poolsJson, _ := json.Marshal(pools)
	ctx := cmd.Context{Stdout: &buf}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(poolsJson), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/pool"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := listPoolsInTheSchedulerCmd{}.Run(&ctx, client)
	c.Assert(err, check.IsNil)
	expected := `+-------+------------------+
| Pools | Teams            |
+-------+------------------+
| pool1 | tsuruteam, ateam |
+-------+------------------+
`
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestAddTeamsToPoolCmdInfo(c *check.C) {
	expected := cmd.Info{
		Name:    "docker-pool-teams-add",
		Usage:   "docker-pool-teams-add <pool> <teams>",
		Desc:    "Add team to a pool",
		MinArgs: 2,
	}
	cmd := addTeamsToPoolCmd{}
	c.Assert(cmd.Info(), check.DeepEquals, &expected)
}

func (s *S) TestAddTeamsToPoolCmdRun(c *check.C) {
	var buf bytes.Buffer
	ctx := cmd.Context{Stdout: &buf, Args: []string{"pool1", "team1", "team2"}}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/pool/team"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := addTeamsToPoolCmd{}.Run(&ctx, client)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemoveTeamsFromPoolCmdInfo(c *check.C) {
	expected := cmd.Info{
		Name:    "docker-pool-teams-remove",
		Usage:   "docker-pool-teams-remove <pool> <teams>",
		Desc:    "Remove team from pool",
		MinArgs: 2,
	}
	cmd := removeTeamsFromPoolCmd{}
	c.Assert(cmd.Info(), check.DeepEquals, &expected)
}

func (s *S) TestRemoveTeamsFromPoolCmdRun(c *check.C) {
	var buf bytes.Buffer
	ctx := cmd.Context{Stdout: &buf, Args: []string{"pool1", "team1"}}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/pool/team"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := removeTeamsFromPoolCmd{}.Run(&ctx, client)
	c.Assert(err, check.IsNil)
}

func (s *S) TestChooseContainerToBeRemoved(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
	}
	contColl := s.p.collection()
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
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	cont2 := container{ID: "2", Name: "mirror1", AppName: a1.Name}
	a2 := app.App{Name: "notimpius", Teams: []string{"tsuruteam", "nodockerforme"}}
	cont3 := container{ID: "3", Name: "dedication1", AppName: a2.Name}
	cont4 := container{ID: "4", Name: "dedication2", AppName: a2.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	err = s.storage.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	defer s.storage.Apps().RemoveAll(bson.M{"name": a2.Name})
	coll := s.storage.Collection(schedulerCollection)
	p := Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	err = coll.Insert(p)
	c.Assert(err, check.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := s.p.collection()
	err = contColl.Insert(
		cont1, cont2, cont3, cont4,
	)
	c.Assert(err, check.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name, cont4.Name}}})
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
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
