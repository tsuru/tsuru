// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestSchedulerSchedule(c *gocheck.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}}
	a2 := app.App{Name: "mirror", Teams: []string{"tsuruteam"}}
	a3 := app.App{Name: "dedication", Teams: []string{"nodockerforme"}}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	cont2 := container{ID: "2", Name: "mirror1", AppName: a2.Name}
	cont3 := container{ID: "3", Name: "dedication1", AppName: a3.Name}
	err := s.storage.Apps().Insert(a1, a2, a3)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{a1.Name, a2.Name, a3.Name}}})
	coll := s.storage.Collection(schedulerCollection)
	p := Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	err = coll.Insert(p)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := collection()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, gocheck.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name}}})
	var scheduler segregatedScheduler
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, gocheck.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	_, err = clusterInstance.Register("http://url1:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	_, err = clusterInstance.Register("http://url2:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.Address, gocheck.Equals, "http://url0:1234")
	opts = docker.CreateContainerOptions{Name: cont2.Name}
	node, err = scheduler.Schedule(clusterInstance, opts, a2.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.Address, gocheck.Equals, "http://url1:1234")
	opts = docker.CreateContainerOptions{Name: cont3.Name}
	node, err = scheduler.Schedule(clusterInstance, opts, a3.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.Address, gocheck.Equals, "http://url2:1234")
}

func (s *S) TestSchedulerScheduleNoName(c *gocheck.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}}
	a2 := app.App{Name: "mirror", Teams: []string{"tsuruteam"}}
	a3 := app.App{Name: "dedication", Teams: []string{"nodockerforme"}}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	cont2 := container{ID: "2", Name: "mirror1", AppName: a2.Name}
	cont3 := container{ID: "3", Name: "dedication1", AppName: a3.Name}
	err := s.storage.Apps().Insert(a1, a2, a3)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{a1.Name, a2.Name, a3.Name}}})
	coll := s.storage.Collection(schedulerCollection)
	p := Pool{Name: "pool1", Teams: []string{
		"tsuruteam",
		"nodockerforme",
	}}
	err = coll.Insert(p)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := collection()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, gocheck.IsNil)
	defer contColl.RemoveAll(bson.M{"name": bson.M{"$in": []string{cont1.Name, cont2.Name, cont3.Name}}})
	var scheduler segregatedScheduler
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, gocheck.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	_, err = clusterInstance.Register("http://url1:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	_, err = clusterInstance.Register("http://url2:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	opts := docker.CreateContainerOptions{}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.Address, gocheck.Equals, "http://url0:1234")
	container, err := getContainer(cont1.ID)
	c.Assert(err, gocheck.IsNil)
	c.Assert(container.HostAddr, gocheck.Equals, "")
}

func (s *S) TestSchedulerScheduleFallback(c *gocheck.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	coll := s.storage.Collection(schedulerCollection)
	p := Pool{Name: "pool1"}
	err = coll.Insert(p)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := collection()
	err = contColl.Insert(cont1)
	c.Assert(err, gocheck.IsNil)
	defer contColl.RemoveAll(bson.M{"name": cont1.Name})
	var scheduler segregatedScheduler
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, gocheck.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.Address, gocheck.Equals, "http://url0:1234")
}

func (s *S) TestSchedulerScheduleTeamOwner(c *gocheck.C) {
	a1 := app.App{
		Name:      "impius",
		Teams:     []string{"nodockerforme"},
		TeamOwner: "tsuruteam",
	}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	coll := s.storage.Collection(schedulerCollection)
	p := Pool{Name: "pool1", Teams: []string{"tsuruteam"}}
	err = coll.Insert(p)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := collection()
	err = contColl.Insert(cont1)
	c.Assert(err, gocheck.IsNil)
	defer contColl.RemoveAll(bson.M{"name": cont1.Name})
	var scheduler segregatedScheduler
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, gocheck.IsNil)
	_, err = clusterInstance.Register("http://url0:1234", map[string]string{"pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, a1.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.Address, gocheck.Equals, "http://url0:1234")
}

func (s *S) TestSchedulerNoFallback(c *gocheck.C) {
	app := app.App{Name: "bill", Teams: []string{"jean"}}
	err := s.storage.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app.Name})
	cont1 := container{ID: "1", Name: "bill", AppName: app.Name}
	contColl := collection()
	err = contColl.Insert(cont1)
	c.Assert(err, gocheck.IsNil)
	defer contColl.Remove(bson.M{"name": cont1.Name})
	var scheduler segregatedScheduler
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, gocheck.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, opts, app.Name)
	c.Assert(node.Address, gocheck.Equals, "")
	c.Assert(err, gocheck.Equals, errNoFallback)
}

func (s *S) TestSchedulerNoNodesNoPool(c *gocheck.C) {
	var scheduler segregatedScheduler
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, gocheck.IsNil)
	opts := docker.CreateContainerOptions{}
	node, err := scheduler.Schedule(clusterInstance, opts, "")
	c.Assert(node.Address, gocheck.Equals, "")
	c.Assert(err, gocheck.Equals, errNoFallback)
}

func (s *S) TestSchedulerNoNodesWithFallbackPool(c *gocheck.C) {
	var scheduler segregatedScheduler
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{})
	c.Assert(err, gocheck.IsNil)
	err = scheduler.addPool("mypool")
	c.Assert(err, gocheck.IsNil)
	err = scheduler.addPool("mypool2")
	c.Assert(err, gocheck.IsNil)
	defer scheduler.removePool("mypool")
	defer scheduler.removePool("mypool2")
	opts := docker.CreateContainerOptions{}
	node, err := scheduler.Schedule(clusterInstance, opts, "")
	c.Assert(node.Address, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Matches, "No nodes found with one of the following metadata: pool=mypool, pool=mypool2")
}

func (s *S) TestSchedulerScheduleWithMemoryAwareness(c *gocheck.C) {
	app1 := app.App{Name: "skyrim", Plan: app.Plan{Memory: 60000}}
	err := s.storage.Apps().Insert(app1)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app1.Name})
	app2 := app.App{Name: "oblivion", Plan: app.Plan{Memory: 20000}}
	err = s.storage.Apps().Insert(app2)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().Remove(bson.M{"name": app2.Name})
	segSched := segregatedScheduler{
		maxMemoryRatio:      0.8,
		totalMemoryMetadata: "totalMemory",
	}
	err = segSched.addPool("mypool")
	c.Assert(err, gocheck.IsNil)
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
	c.Assert(err, gocheck.Equals, nil)
	cont1 := container{ID: "pre1", Name: "existingUnit1", AppName: "skyrim", HostAddr: "server1"}
	contColl := collection()
	defer contColl.RemoveAll(bson.M{"appname": "skyrim"})
	defer contColl.RemoveAll(bson.M{"appname": "oblivion"})
	err = contColl.Insert(cont1)
	c.Assert(err, gocheck.Equals, nil)
	for i := 0; i < 5; i++ {
		cont := container{ID: string(i), Name: fmt.Sprintf("unit%d", i), AppName: "oblivion"}
		err := contColl.Insert(cont)
		c.Assert(err, gocheck.IsNil)
		opts := docker.CreateContainerOptions{
			Name: cont.Name,
		}
		node, err := segSched.Schedule(clusterInstance, opts, cont.AppName)
		c.Assert(err, gocheck.IsNil)
		c.Assert(node, gocheck.NotNil)
	}
	n, err := contColl.Find(bson.M{"hostaddr": "server1"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, 2)
	n, err = contColl.Find(bson.M{"hostaddr": "server2"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, 4)
	n, err = contColl.Find(bson.M{"hostaddr": "server1", "appname": "oblivion"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, 1)
	n, err = contColl.Find(bson.M{"hostaddr": "server2", "appname": "oblivion"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, 4)
	cont := container{ID: "post-error", Name: "post-error-1", AppName: "oblivion"}
	err = contColl.Insert(cont)
	c.Assert(err, gocheck.IsNil)
	opts := docker.CreateContainerOptions{
		Name: cont.Name,
	}
	_, err = segSched.Schedule(clusterInstance, opts, cont.AppName)
	c.Assert(err, gocheck.ErrorMatches, "No nodes found with enough memory for container of \"oblivion\": 0.0191MB.")
}

func (s *S) TestChooseNodeDistributesNodesEqually(c *gocheck.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
		{Address: "http://server3:1234"},
		{Address: "http://server4:1234"},
	}
	contColl := collection()
	defer contColl.RemoveAll(bson.M{"appname": "coolapp9"})
	cont1 := container{ID: "pre1", Name: "existingUnit1", AppName: "coolapp9", HostAddr: "server1"}
	err := contColl.Insert(cont1)
	c.Assert(err, gocheck.Equals, nil)
	cont2 := container{ID: "pre2", Name: "existingUnit2", AppName: "coolapp9", HostAddr: "server2"}
	err = contColl.Insert(cont2)
	c.Assert(err, gocheck.Equals, nil)
	numberOfUnits := 38
	unitsPerNode := (numberOfUnits + 2) / 4
	wg := sync.WaitGroup{}
	wg.Add(numberOfUnits)
	for i := 0; i < numberOfUnits; i++ {
		go func(i int) {
			defer wg.Done()
			cont := container{ID: string(i), Name: fmt.Sprintf("unit%d", i), AppName: "coolapp9"}
			err := contColl.Insert(cont)
			c.Assert(err, gocheck.IsNil)
			var s segregatedScheduler
			node, err := s.chooseNode(nodes, cont.Name, "coolapp9")
			c.Assert(err, gocheck.IsNil)
			c.Assert(node, gocheck.NotNil)
		}(i)
	}
	wg.Wait()
	n, err := contColl.Find(bson.M{"hostaddr": "server1"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, unitsPerNode)
	n, err = contColl.Find(bson.M{"hostaddr": "server2"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, unitsPerNode)
	n, err = contColl.Find(bson.M{"hostaddr": "server3"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, unitsPerNode)
	n, err = contColl.Find(bson.M{"hostaddr": "server4"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, unitsPerNode)
}

func (s *S) TestChooseNodeDistributesNodesEquallyDifferentApps(c *gocheck.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
	}
	contColl := collection()
	defer contColl.RemoveAll(bson.M{"appname": "skyrim"})
	defer contColl.RemoveAll(bson.M{"appname": "oblivion"})
	cont1 := container{ID: "pre1", Name: "existingUnit1", AppName: "skyrim", HostAddr: "server1"}
	err := contColl.Insert(cont1)
	c.Assert(err, gocheck.Equals, nil)
	cont2 := container{ID: "pre2", Name: "existingUnit2", AppName: "skyrim", HostAddr: "server1"}
	err = contColl.Insert(cont2)
	c.Assert(err, gocheck.Equals, nil)
	numberOfUnits := 2
	wg := sync.WaitGroup{}
	wg.Add(numberOfUnits)
	for i := 0; i < numberOfUnits; i++ {
		go func(i int) {
			defer wg.Done()
			cont := container{ID: string(i), Name: fmt.Sprintf("unit%d", i), AppName: "oblivion"}
			err := contColl.Insert(cont)
			c.Assert(err, gocheck.IsNil)
			var s segregatedScheduler
			node, err := s.chooseNode(nodes, cont.Name, "oblivion")
			c.Assert(err, gocheck.IsNil)
			c.Assert(node, gocheck.NotNil)
		}(i)
	}
	wg.Wait()
	n, err := contColl.Find(bson.M{"hostaddr": "server1"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, 3)
	n, err = contColl.Find(bson.M{"hostaddr": "server2"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, 1)
	n, err = contColl.Find(bson.M{"hostaddr": "server1", "appname": "oblivion"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, 1)
	n, err = contColl.Find(bson.M{"hostaddr": "server2", "appname": "oblivion"}).Count()
	c.Assert(err, gocheck.Equals, nil)
	c.Check(n, gocheck.Equals, 1)
}

func (s *S) TestAddPool(c *gocheck.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	defer coll.RemoveId("pool1")
	err := seg.addPool("pool1")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestAddPoolWithoutNameShouldBreak(c *gocheck.C) {
	var seg segregatedScheduler
	err := seg.addPool("")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Pool name is required.")
}

func (s *S) TestRemovePool(c *gocheck.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, gocheck.IsNil)
	err = seg.removePool("pool1")
	c.Assert(err, gocheck.IsNil)
	p, err := coll.FindId("pool1").Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(p, gocheck.Equals, 0)
}

func (s *S) TestAddTeamToPool(c *gocheck.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveId(pool.Name)
	err = seg.addTeamsToPool("pool1", []string{"ateam", "test"})
	c.Assert(err, gocheck.IsNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Teams, gocheck.DeepEquals, []string{"ateam", "test"})
}

func (s *S) TestAddTeamToPollWithTeams(c *gocheck.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1", Teams: []string{"test", "ateam"}}
	err := coll.Insert(pool)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveId(pool.Name)
	err = seg.addTeamsToPool(pool.Name, []string{"pteam"})
	c.Assert(err, gocheck.IsNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Teams, gocheck.DeepEquals, []string{"test", "ateam", "pteam"})
}

func (s *S) TestAddTeamToPollShouldNotAcceptDuplicatedTeam(c *gocheck.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1", Teams: []string{"test", "ateam"}}
	err := coll.Insert(pool)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveId(pool.Name)
	err = seg.addTeamsToPool(pool.Name, []string{"ateam"})
	c.Assert(err, gocheck.NotNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Teams, gocheck.DeepEquals, []string{"test", "ateam"})
}

func (s *S) TestRemoveTeamsFromPool(c *gocheck.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1", Teams: []string{"test", "ateam"}}
	err := coll.Insert(pool)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveId(pool.Name)
	err = seg.removeTeamsFromPool(pool.Name, []string{"test"})
	c.Assert(err, gocheck.IsNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Teams, gocheck.DeepEquals, []string{"ateam"})
}

func (s *S) TestAddPoolToSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-pool-add",
		Usage:   "docker-pool-add <pool>",
		Desc:    "Add a pool to cluster",
		MinArgs: 1,
	}
	cmd := addPoolToSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *S) TestAddPoolToTheSchedulerCmd(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"poolTest"}, Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/pool"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := addPoolToSchedulerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRemovePoolFromSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-pool-remove",
		Usage:   "docker-pool-remove <pool> [-y]",
		Desc:    "Remove a pool to cluster",
		MinArgs: 1,
	}
	cmd := removePoolFromSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *S) TestRemovePoolFromTheSchedulerCmd(c *gocheck.C) {
	var buf bytes.Buffer
	context := cmd.Context{Args: []string{"poolTest"}, Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/pool"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := removePoolFromSchedulerCmd{}
	cmd.Flags().Parse(true, []string{"-y"})
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRemovePoolFromTheSchedulerCmdConfirmation(c *gocheck.C) {
	var stdout bytes.Buffer
	context := cmd.Context{
		Args:   []string{"poolX"},
		Stdout: &stdout,
		Stdin:  strings.NewReader("n\n"),
	}
	command := removePoolFromSchedulerCmd{}
	err := command.Run(&context, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "Are you sure you want to remove \"poolX\" pool? (y/n) Abort.\n")
}

func (s *S) TestListPoolsInTheSchedulerCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:  "docker-pool-list",
		Usage: "docker-pool-list",
		Desc:  "List available pools in the cluster",
	}
	cmd := listPoolsInTheSchedulerCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *S) TestListPoolsInTheSchedulerCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	pool := Pool{Name: "pool1", Teams: []string{"tsuruteam", "ateam"}}
	pools := []Pool{pool}
	poolsJson, _ := json.Marshal(pools)
	ctx := cmd.Context{Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: string(poolsJson), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/pool"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := listPoolsInTheSchedulerCmd{}.Run(&ctx, client)
	c.Assert(err, gocheck.IsNil)
	expected := `+-------+------------------+
| Pools | Teams            |
+-------+------------------+
| pool1 | tsuruteam, ateam |
+-------+------------------+
`
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestAddTeamsToPoolCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-pool-teams-add",
		Usage:   "docker-pool-teams-add <pool> <teams>",
		Desc:    "Add team to a pool",
		MinArgs: 2,
	}
	cmd := addTeamsToPoolCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *S) TestAddTeamsToPoolCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	ctx := cmd.Context{Stdout: &buf, Args: []string{"pool1", "team1", "team2"}}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/pool/team"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := addTeamsToPoolCmd{}.Run(&ctx, client)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRemoveTeamsFromPoolCmdInfo(c *gocheck.C) {
	expected := cmd.Info{
		Name:    "docker-pool-teams-remove",
		Usage:   "docker-pool-teams-remove <pool> <teams>",
		Desc:    "Remove team from pool",
		MinArgs: 2,
	}
	cmd := removeTeamsFromPoolCmd{}
	c.Assert(cmd.Info(), gocheck.DeepEquals, &expected)
}

func (s *S) TestRemoveTeamsFromPoolCmdRun(c *gocheck.C) {
	var buf bytes.Buffer
	ctx := cmd.Context{Stdout: &buf, Args: []string{"pool1", "team1"}}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: "", Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/docker/pool/team"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := removeTeamsFromPoolCmd{}.Run(&ctx, client)
	c.Assert(err, gocheck.IsNil)
}
