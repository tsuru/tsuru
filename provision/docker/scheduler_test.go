// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"sync"
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
	p := Pool{Name: "pool1", Nodes: []string{
		"http://url0:1234",
		"http://url1:1234",
		"http://url2:1234",
	}, Teams: []string{
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
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(opts, a1.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.ID, gocheck.Equals, "http://url0:1234")
	opts = docker.CreateContainerOptions{Name: cont2.Name}
	node, err = scheduler.Schedule(opts, a2.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.ID, gocheck.Equals, "http://url1:1234")
	opts = docker.CreateContainerOptions{Name: cont3.Name}
	node, err = scheduler.Schedule(opts, a3.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.ID, gocheck.Equals, "http://url2:1234")
}

func (s *S) TestSchedulerScheduleFallback(c *gocheck.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}}
	cont1 := container{ID: "1", Name: "impius1", AppName: a1.Name}
	err := s.storage.Apps().Insert(a1)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": a1.Name})
	coll := s.storage.Collection(schedulerCollection)
	p := Pool{Name: "pool1", Nodes: []string{"http://url0:1234"}}
	err = coll.Insert(p)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := collection()
	err = contColl.Insert(cont1)
	c.Assert(err, gocheck.IsNil)
	defer contColl.RemoveAll(bson.M{"name": cont1.Name})
	var scheduler segregatedScheduler
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(opts, a1.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.ID, gocheck.Equals, "http://url0:1234")
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
	p := Pool{Name: "pool1", Nodes: []string{"http://url0:1234"}, Teams: []string{"tsuruteam"}}
	err = coll.Insert(p)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": p.Name})
	contColl := collection()
	err = contColl.Insert(cont1)
	c.Assert(err, gocheck.IsNil)
	defer contColl.RemoveAll(bson.M{"name": cont1.Name})
	var scheduler segregatedScheduler
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(opts, a1.Name)
	c.Assert(err, gocheck.IsNil)
	c.Check(node.ID, gocheck.Equals, "http://url0:1234")
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
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(opts, app.Name)
	c.Assert(node.ID, gocheck.Equals, "")
	c.Assert(err, gocheck.Equals, errNoFallback)
}

func (s *S) TestSchedulerNoNodes(c *gocheck.C) {
	var scheduler segregatedScheduler
	opts := docker.CreateContainerOptions{}
	node, err := scheduler.Schedule(opts, "")
	c.Assert(node.ID, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestSchedulerNodes(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	newNodes := []string{
		"http://localhost:8080",
		"http://localhost:8081",
		"http://localhost:8082",
	}
	pool := Pool{Name: "teste", Nodes: newNodes}
	err := coll.Insert(pool)
	c.Assert(err, gocheck.IsNil)
	defer coll.Remove(pool)
	expected := []cluster.Node{
		{ID: "http://localhost:8080", Address: "http://localhost:8080"},
		{ID: "http://localhost:8081", Address: "http://localhost:8081"},
		{ID: "http://localhost:8082", Address: "http://localhost:8082"},
	}
	var scheduler segregatedScheduler
	nodes, err := scheduler.Nodes()
	c.Assert(err, gocheck.IsNil)
	c.Assert(nodes, gocheck.DeepEquals, expected)
}

func (s *S) TestSchedulerNodesForOptions(c *gocheck.C) {
	a1 := app.App{Name: "egwene", Teams: []string{"team1"}}
	a2 := app.App{Name: "nynaeve", Teams: []string{"team1", "team2"}, TeamOwner: "team2"}
	a3 := app.App{Name: "moiraine", Teams: []string{"team3"}}
	err := s.storage.Apps().Insert(a1, a2, a3)
	c.Assert(err, gocheck.IsNil)
	defer s.storage.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{a1.Name, a2.Name, a3.Name}}})
	coll := s.storage.Collection(schedulerCollection)
	pool1 := Pool{Name: "pool1", Nodes: []string{"http://localhost:8080"}, Teams: []string{"team1"}}
	pool2 := Pool{Name: "pool2", Nodes: []string{"http://localhost:8081"}, Teams: []string{"team2", "team1"}}
	pool3 := Pool{Name: "pool3", Nodes: []string{"http://localhost:8082"}, Teams: []string{"team3"}}
	err = coll.Insert(pool1, pool2, pool3)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": bson.M{"$in": []string{pool1.Name, pool2.Name, pool3.Name}}})
	var scheduler segregatedScheduler
	nodes, err := scheduler.NodesForOptions("egwene")
	c.Assert(err, gocheck.IsNil)
	c.Assert(nodes, gocheck.DeepEquals, []cluster.Node{{ID: "http://localhost:8080", Address: "http://localhost:8080"}})
	nodes, err = scheduler.NodesForOptions("nynaeve")
	c.Assert(err, gocheck.IsNil)
	c.Assert(nodes, gocheck.DeepEquals, []cluster.Node{{ID: "http://localhost:8081", Address: "http://localhost:8081"}})
	nodes, err = scheduler.NodesForOptions("moiraine")
	c.Assert(err, gocheck.IsNil)
	c.Assert(nodes, gocheck.DeepEquals, []cluster.Node{{ID: "http://localhost:8082", Address: "http://localhost:8082"}})
}

func (s *S) TestSchedulerGetNode(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1", Nodes: []string{
		"http://localhost:8080",
	}}
	err := coll.Insert(pool)
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": pool.Name})
	var scheduler segregatedScheduler
	nd, err := scheduler.GetNode(pool.Name, pool.Nodes[0])
	c.Assert(err, gocheck.IsNil)
	c.Assert(nd, gocheck.Equals, "http://localhost:8080")
	_, err = scheduler.GetNode(pool.Name, "http://localhost:8082")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestAddNodeToScheduler(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	coll.Insert(Pool{Name: "pool1"})
	nd := cluster.Node{ID: "http://localhost:8080", Address: "http://localhost:8080"}
	var scheduler segregatedScheduler
	err := scheduler.Register(map[string]string{"ID": nd.ID, "address": nd.Address, "pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": "pool1"})
	var p Pool
	err = coll.Find(bson.M{"_id": "pool1"}).One(&p)
	c.Assert(err, gocheck.IsNil)
	n := p.Nodes[0]
	c.Check(n, gocheck.Equals, nd.ID)
	c.Check(n, gocheck.Equals, "http://localhost:8080")
}

func (s *S) TestAddNodeDuplicated(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	err := coll.Insert(Pool{Name: "pool1"})
	c.Assert(err, gocheck.IsNil)
	nd := cluster.Node{ID: "server0", Address: "http://localhost:8080"}
	var scheduler segregatedScheduler
	err = scheduler.Register(map[string]string{"ID": nd.ID, "address": nd.Address, "pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": "pool1"})
	err = scheduler.Register(map[string]string{"ID": nd.ID, "address": nd.Address, "pool": "pool2"})
	c.Assert(err, gocheck.Equals, errNodeAlreadyRegister)
}

func (s *S) TestAddNodeWithoutPoolNameError(c *gocheck.C) {
	var scheduler segregatedScheduler
	err := scheduler.Register(map[string]string{"address": "http://localhost:1234"})
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Pool name is required.")
}

func (s *S) TestRemoveNodeFromScheduler(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	err := coll.Insert(Pool{Name: "pool1"})
	c.Assert(err, gocheck.IsNil)
	nd := cluster.Node{ID: "server0", Address: "http://localhost:8080"}
	var scheduler segregatedScheduler
	err = scheduler.Register(map[string]string{"ID": nd.ID, "address": nd.Address, "pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": "pool1"})
	err = scheduler.Unregister(map[string]string{"address": nd.Address, "pool": "pool1"})
	c.Assert(err, gocheck.IsNil)
	n, err := coll.Find(bson.M{"_id": "server0"}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
}

func (s *S) TesteRemoveUnknownNodeFromScheduler(c *gocheck.C) {
	var scheduler segregatedScheduler
	err := scheduler.Unregister(map[string]string{"ID": "server0"})
	c.Assert(err, gocheck.Equals, errNodeNotFound)
}

func (s *S) TestListNodesInTheScheduler(c *gocheck.C) {
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, gocheck.IsNil)
	var scheduler segregatedScheduler
	err = scheduler.Register(map[string]string{"ID": "server0", "address": "http://localhost:8080", "pool": pool.Name})
	c.Assert(err, gocheck.IsNil)
	err = scheduler.Register(map[string]string{"ID": "server1", "address": "http://localhost:9090", "pool": pool.Name})
	c.Assert(err, gocheck.IsNil)
	err = scheduler.Register(map[string]string{"ID": "server2", "address": "http://localhost:9091", "pool": pool.Name})
	c.Assert(err, gocheck.IsNil)
	defer coll.RemoveAll(bson.M{"_id": pool.Name})
	nodes, err := listNodesInTheScheduler()
	c.Assert(err, gocheck.IsNil)
	expected := []string{
		"http://localhost:8080",
		"http://localhost:9090",
		"http://localhost:9091",
	}
	c.Assert(nodes, gocheck.DeepEquals, expected)
}

func (s *S) TestChooseNodeDistributesNodesEqually(c *gocheck.C) {
	nodes := []string{
		"http://server1:1234",
		"http://server2:1234",
		"http://server3:1234",
		"http://server4:1234",
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
	nodes := []string{
		"http://server1:1234",
		"http://server2:1234",
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

func (s *S) TestRemovePoolDontRemovePoolWithNodes(c *gocheck.C) {
	var seg segregatedScheduler
	coll := s.storage.Collection(schedulerCollection)
	pool := Pool{Name: "pool1", Nodes: []string{"test:1234"}}
	err := coll.Insert(pool)
	defer coll.RemoveId(pool.Name)
	c.Assert(err, gocheck.IsNil)
	err = seg.removePool("pool1")
	c.Assert(err, gocheck.NotNil)
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
			return req.URL.Path == "/pool"
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
		Usage:   "docker-pool-remove <pool>",
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
			return req.URL.Path == "/pool"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	cmd := removePoolFromSchedulerCmd{}
	err := cmd.Run(&context, client)
	c.Assert(err, gocheck.IsNil)
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
	pool := Pool{Name: "pool1", Nodes: []string{"url:1234", "url:2345"}, Teams: []string{"tsuruteam", "ateam"}}
	pools := []Pool{pool}
	poolsJson, _ := json.Marshal(pools)
	ctx := cmd.Context{Stdout: &buf}
	trans := &testing.ConditionalTransport{
		Transport: testing.Transport{Message: string(poolsJson), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/pool"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := listPoolsInTheSchedulerCmd{}.Run(&ctx, client)
	c.Assert(err, gocheck.IsNil)
	expected := `+-------+--------------------+------------------+
| Pools | Nodes              | Teams            |
+-------+--------------------+------------------+
| pool1 | url:1234, url:2345 | tsuruteam, ateam |
+-------+--------------------+------------------+
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
			return req.URL.Path == "/pool/team"
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
			return req.URL.Path == "/pool/team"
		},
	}
	manager := cmd.Manager{}
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, &manager)
	err := removeTeamsFromPoolCmd{}.Run(&ctx, client)
	c.Assert(err, gocheck.IsNil)
}
