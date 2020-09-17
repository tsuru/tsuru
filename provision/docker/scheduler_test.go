// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/autoscale"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/types"
	"github.com/tsuru/tsuru/provision/pool"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func (s *S) TestSchedulerSchedule(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	a2 := app.App{Name: "mirror", Teams: []string{"tsuruteam"}, Pool: "pool1"}
	a3 := app.App{Name: "dedication", Teams: []string{"nodockerforme"}, Pool: "pool1"}
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "impius1", AppName: a1.Name}}
	cont2 := container.Container{Container: types.Container{ID: "2", Name: "mirror1", AppName: a2.Name}}
	cont3 := container.Container{Container: types.Container{ID: "3", Name: "dedication1", AppName: a3.Name}}
	err := s.conn.Apps().Insert(a1, a2, a3)
	c.Assert(err, check.IsNil)
	p := pool.Pool{Name: "pool1"}
	o := pool.AddPoolOptions{Name: p.Name}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(p.Name, []string{
		"tsuruteam",
		"nodockerforme",
	})
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{}, "")
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	server1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server1.Stop()
	server2, err := testing.NewServer("localhost:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server2.Stop()
	err = clusterInstance.Register(cluster.Node{
		Address:  server1.URL(),
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	localURL := strings.Replace(server2.URL(), "127.0.0.1", "localhost", -1)
	err = clusterInstance.Register(cluster.Node{
		Address:  localURL,
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: a1.Name, ProcessName: "web"})
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, server1.URL())
	opts = docker.CreateContainerOptions{Name: cont2.Name}
	node, err = scheduler.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: a2.Name, ProcessName: "web"})
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, localURL)
}

func (s *S) TestSchedulerScheduleFilteringNodes(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	a2 := app.App{Name: "mirror", Teams: []string{"tsuruteam"}, Pool: "pool1"}
	a3 := app.App{Name: "dedication", Teams: []string{"nodockerforme"}, Pool: "pool1"}
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "impius1", AppName: a1.Name}}
	cont2 := container.Container{Container: types.Container{ID: "2", Name: "mirror1", AppName: a2.Name}}
	cont3 := container.Container{Container: types.Container{ID: "3", Name: "dedication1", AppName: a3.Name}}
	err := s.conn.Apps().Insert(a1, a2, a3)
	c.Assert(err, check.IsNil)
	p := pool.Pool{Name: "pool1"}
	o := pool.AddPoolOptions{Name: p.Name}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(p.Name, []string{
		"tsuruteam",
		"nodockerforme",
	})
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{}, "")
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	server1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server1.Stop()
	server2, err := testing.NewServer("localhost:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server2.Stop()
	err = clusterInstance.Register(cluster.Node{
		Address:  server1.URL(),
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	localURL := strings.Replace(server2.URL(), "127.0.0.1", "localhost", -1)
	err = clusterInstance.Register(cluster.Node{
		Address:  localURL,
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	schedOpts := &container.SchedulerOpts{
		AppName:     a1.Name,
		ProcessName: "web",
		FilterNodes: []string{localURL},
	}
	node, err := scheduler.Schedule(clusterInstance, &opts, schedOpts)
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, localURL)
}

func (s *S) TestFilterNodes(c *check.C) {
	tests := []struct {
		nodes    []cluster.Node
		filter   map[string]struct{}
		expected []cluster.Node
	}{
		{
			nodes: []cluster.Node{
				{Address: "n1"},
				{Address: "n3"},
				{Address: "n2"},
				{Address: "n4"},
			},
			filter: map[string]struct{}{
				"n1": {},
				"n2": {},
			},
			expected: []cluster.Node{
				{Address: "n1"},
				{Address: "n2"},
			},
		},
		{
			nodes: []cluster.Node{
				{Address: "n1"},
				{Address: "n3"},
				{Address: "n2"},
				{Address: "n4"},
			},
			filter: nil,
			expected: []cluster.Node{
				{Address: "n1"},
				{Address: "n3"},
				{Address: "n2"},
				{Address: "n4"},
			},
		},
		{
			nodes: []cluster.Node{
				{Address: "n1"},
				{Address: "n3"},
				{Address: "n2"},
				{Address: "n4"},
			},
			filter: map[string]struct{}{
				"n5": {},
			},
			expected: []cluster.Node{},
		},
	}
	for _, tt := range tests {
		newNodes := filterNodes(tt.nodes, tt.filter)
		c.Assert(newNodes, check.DeepEquals, tt.expected)
	}
}

func (s *S) TestSchedulerScheduleChangesContainerName(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "test-default"}
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "impius1", AppName: a1.Name}}
	err := s.conn.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{}, "")
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	server1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server1.Stop()
	err = clusterInstance.Register(cluster.Node{
		Address:  server1.URL(),
		Metadata: map[string]string{"pool": "test-default"},
	})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	node, err := scheduler.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: a1.Name, ProcessName: "web", UpdateName: true})
	c.Assert(err, check.IsNil)
	c.Assert(opts.Name, check.Not(check.Equals), cont1.Name)
	c.Assert(node.Address, check.Equals, server1.URL())
	var dbConts []container.Container
	err = contColl.Find(nil).All(&dbConts)
	c.Assert(err, check.IsNil)
	c.Assert(dbConts, check.HasLen, 1)
	c.Assert(dbConts[0].Name, check.Equals, opts.Name)
	c.Assert(dbConts[0].HostAddr, check.Equals, "127.0.0.1")
}

func (s *S) TestSchedulerScheduleNoName(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	a2 := app.App{Name: "mirror", Teams: []string{"tsuruteam"}, Pool: "pool1"}
	a3 := app.App{Name: "dedication", Teams: []string{"nodockerforme"}, Pool: "pool1"}
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "impius1", AppName: a1.Name}}
	cont2 := container.Container{Container: types.Container{ID: "2", Name: "mirror1", AppName: a2.Name}}
	cont3 := container.Container{Container: types.Container{ID: "3", Name: "dedication1", AppName: a3.Name}}
	err := s.conn.Apps().Insert(a1, a2, a3)
	c.Assert(err, check.IsNil)
	p := pool.Pool{Name: "pool1"}
	o := pool.AddPoolOptions{Name: p.Name}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(p.Name, []string{
		"tsuruteam",
		"nodockerforme",
	})
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3,
	)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{}, "")
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	server1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server1.Stop()
	server2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server2.Stop()
	localURL := strings.Replace(server2.URL(), "127.0.0.1", "localhost", -1)
	err = clusterInstance.Register(cluster.Node{
		Address:  server1.URL(),
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	err = clusterInstance.Register(cluster.Node{
		Address:  localURL,
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{}
	node, err := scheduler.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: a1.Name, ProcessName: "web"})
	c.Assert(err, check.IsNil)
	c.Check(node.Address, check.Equals, server1.URL())
	container, err := s.p.GetContainer(cont1.ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.HostAddr, check.Equals, "")
}

func (s *S) TestSchedulerNoNodes(c *check.C) {
	app := app.App{Name: "bill", Pool: "mypool"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{}, "")
	c.Assert(err, check.IsNil)
	o := pool.AddPoolOptions{Name: "mypool"}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	o = pool.AddPoolOptions{Name: "mypool2"}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{}
	schedOpts := &container.SchedulerOpts{AppName: app.Name, ProcessName: "web"}
	node, err := scheduler.Schedule(clusterInstance, &opts, schedOpts)
	c.Assert(node.Address, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Matches, "error in scheduler: No nodes found with one of the following metadata: pool=mypool")
}

func (s *S) TestSchedulerScheduleWithMemoryAwareness(c *check.C) {
	logBuf := bytes.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(logBuf, false))
	defer log.SetLogger(nil)
	app1 := app.App{Name: "skyrim", Plan: appTypes.Plan{Memory: 60000}, Pool: "mypool"}
	err := s.conn.Apps().Insert(app1)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "oblivion", Plan: appTypes.Plan{Memory: 20000}, Pool: "mypool"}
	err = s.conn.Apps().Insert(app2)
	c.Assert(err, check.IsNil)
	segSched := segregatedScheduler{
		maxMemoryRatio:      0.8,
		TotalMemoryMetadata: "totalMemory",
		provisioner:         s.p,
	}
	o := pool.AddPoolOptions{Name: "mypool"}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	server1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server1.Stop()
	server2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server2.Stop()
	localURL := strings.Replace(server2.URL(), "127.0.0.1", "localhost", -1)
	clusterInstance, err := cluster.New(&segSched, &cluster.MapStorage{}, "",
		cluster.Node{Address: server1.URL(), Metadata: map[string]string{
			"totalMemory": "100000",
			"pool":        "mypool",
		}},
		cluster.Node{Address: localURL, Metadata: map[string]string{
			"totalMemory": "100000",
			"pool":        "mypool",
		}},
	)
	c.Assert(err, check.Equals, nil)
	s.p.cluster = clusterInstance
	cont1 := container.Container{Container: types.Container{ID: "pre1", Name: "existingUnit1", AppName: "skyrim", HostAddr: "127.0.0.1"}}
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	for i := 0; i < 5; i++ {
		id := fmt.Sprint(i)
		cont := container.Container{Container: types.Container{ID: id, Name: fmt.Sprintf("unit%d", i), AppName: "oblivion"}}
		err = contColl.Insert(cont)
		c.Assert(err, check.IsNil)
		opts := docker.CreateContainerOptions{
			Name: cont.Name,
		}
		node, schedErr := segSched.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: cont.AppName, ProcessName: "web"})
		c.Assert(schedErr, check.IsNil)
		c.Assert(node, check.NotNil)
	}
	n, err := contColl.Find(bson.M{"hostaddr": "127.0.0.1"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 2)
	n, err = contColl.Find(bson.M{"hostaddr": "localhost"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 4)
	n, err = contColl.Find(bson.M{"hostaddr": "127.0.0.1", "appname": "oblivion"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 1)
	n, err = contColl.Find(bson.M{"hostaddr": "localhost", "appname": "oblivion"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 4)
	cont := container.Container{Container: types.Container{ID: "post-error", Name: "post-error-1", AppName: "oblivion"}}
	err = contColl.Insert(cont)
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{
		Name: cont.Name,
	}
	node, err := segSched.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: cont.AppName, ProcessName: "web"})
	c.Assert(err, check.ErrorMatches, `.*no nodes found with enough memory for container of "oblivion": 0.0191MB.*`)
	c.Assert(node, check.DeepEquals, cluster.Node{})
}

func (s *S) TestSchedulerScheduleWithMemoryAwarenessWithAutoScale(c *check.C) {
	config.Set("docker:scheduler:total-memory-metadata", "memory")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	rule := autoscale.Rule{
		MetadataFilter: "mypool",
		MaxMemoryRatio: 0.1,
		Enabled:        true,
	}
	err := rule.Update()
	c.Assert(err, check.IsNil)
	autoscale.Initialize()
	defer func() {
		cur, errCfg := autoscale.CurrentConfig()
		c.Assert(errCfg, check.IsNil)
		cur.Shutdown(context.Background())
	}()
	logBuf := bytes.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(logBuf, false))
	defer log.SetLogger(nil)
	app1 := app.App{Name: "skyrim", Plan: appTypes.Plan{Memory: 60000}, Pool: "mypool"}
	err = s.conn.Apps().Insert(app1)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "oblivion", Plan: appTypes.Plan{Memory: 20000}, Pool: "mypool"}
	err = s.conn.Apps().Insert(app2)
	c.Assert(err, check.IsNil)
	segSched := segregatedScheduler{
		maxMemoryRatio:      0.8,
		TotalMemoryMetadata: "totalMemory",
		provisioner:         s.p,
	}
	o := pool.AddPoolOptions{Name: "mypool"}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	server1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server1.Stop()
	server2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server2.Stop()
	localURL := strings.Replace(server2.URL(), "127.0.0.1", "localhost", -1)
	clusterInstance, err := cluster.New(&segSched, &cluster.MapStorage{}, "",
		cluster.Node{Address: server1.URL(), Metadata: map[string]string{
			"totalMemory": "100000",
			"pool":        "mypool",
		}},
		cluster.Node{Address: localURL, Metadata: map[string]string{
			"totalMemory": "100000",
			"pool":        "mypool",
		}},
	)
	c.Assert(err, check.Equals, nil)
	s.p.cluster = clusterInstance
	cont1 := container.Container{Container: types.Container{ID: "pre1", Name: "existingUnit1", AppName: "skyrim", HostAddr: "127.0.0.1"}}
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	for i := 0; i < 5; i++ {
		id := fmt.Sprint(i)
		cont := container.Container{Container: types.Container{ID: id, Name: fmt.Sprintf("unit%d", i), AppName: "oblivion"}}
		err = contColl.Insert(cont)
		c.Assert(err, check.IsNil)
		opts := docker.CreateContainerOptions{
			Name: cont.Name,
		}
		node, schedErr := segSched.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: cont.AppName, ProcessName: "web"})
		c.Assert(schedErr, check.IsNil)
		c.Assert(node, check.NotNil)
	}
	n, err := contColl.Find(bson.M{"hostaddr": "127.0.0.1"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 2)
	n, err = contColl.Find(bson.M{"hostaddr": "localhost"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 4)
	n, err = contColl.Find(bson.M{"hostaddr": "127.0.0.1", "appname": "oblivion"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 1)
	n, err = contColl.Find(bson.M{"hostaddr": "localhost", "appname": "oblivion"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 4)
	cont := container.Container{Container: types.Container{ID: "post-error", Name: "post-error-1", AppName: "oblivion"}}
	err = contColl.Insert(cont)
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{
		Name: cont.Name,
	}
	node, err := segSched.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: cont.AppName, ProcessName: "web"})
	c.Assert(err, check.IsNil)
	c.Assert(node, check.NotNil)
	c.Assert(logBuf.String(), check.Matches, `(?s).*WARNING: no nodes found with enough memory for container of "oblivion": 0.0191MB.*`)
}

func (s *S) TestSchedulerScheduleWithMemoryAwarenessWithAutoScaleDisabledForPool(c *check.C) {
	config.Set("docker:auto-scale:enabled", true)
	defer config.Unset("docker:auto-scale:enabled")
	autoscale.Initialize()
	defer func() {
		cur, err := autoscale.CurrentConfig()
		c.Assert(err, check.IsNil)
		cur.Shutdown(context.Background())
	}()
	rule := autoscale.Rule{MetadataFilter: "mypool", Enabled: false}
	err := rule.Update()
	c.Assert(err, check.IsNil)
	defer autoscale.DeleteRule("mypool")
	logBuf := bytes.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(logBuf, false))
	defer log.SetLogger(nil)
	app1 := app.App{Name: "skyrim", Plan: appTypes.Plan{Memory: 60000}, Pool: "mypool"}
	err = s.conn.Apps().Insert(app1)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "oblivion", Plan: appTypes.Plan{Memory: 20000}, Pool: "mypool"}
	err = s.conn.Apps().Insert(app2)
	c.Assert(err, check.IsNil)
	segSched := segregatedScheduler{
		maxMemoryRatio:      0.8,
		TotalMemoryMetadata: "totalMemory",
		provisioner:         s.p,
	}
	o := pool.AddPoolOptions{Name: "mypool"}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	server1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server1.Stop()
	server2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server2.Stop()
	localURL := strings.Replace(server2.URL(), "127.0.0.1", "localhost", -1)
	clusterInstance, err := cluster.New(&segSched, &cluster.MapStorage{}, "",
		cluster.Node{Address: server1.URL(), Metadata: map[string]string{
			"totalMemory": "100000",
			"pool":        "mypool",
		}},
		cluster.Node{Address: localURL, Metadata: map[string]string{
			"totalMemory": "100000",
			"pool":        "mypool",
		}},
	)
	c.Assert(err, check.Equals, nil)
	s.p.cluster = clusterInstance
	cont1 := container.Container{Container: types.Container{ID: "pre1", Name: "existingUnit1", AppName: "skyrim", HostAddr: "127.0.0.1"}}
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	for i := 0; i < 5; i++ {
		id := fmt.Sprint(i)
		cont := container.Container{Container: types.Container{ID: id, Name: fmt.Sprintf("unit%d", i), AppName: "oblivion"}}
		err = contColl.Insert(cont)
		c.Assert(err, check.IsNil)
		opts := docker.CreateContainerOptions{
			Name: cont.Name,
		}
		node, schedErr := segSched.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: cont.AppName, ProcessName: "web"})
		c.Assert(schedErr, check.IsNil)
		c.Assert(node, check.NotNil)
	}
	n, err := contColl.Find(bson.M{"hostaddr": "127.0.0.1"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 2)
	n, err = contColl.Find(bson.M{"hostaddr": "localhost"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 4)
	n, err = contColl.Find(bson.M{"hostaddr": "127.0.0.1", "appname": "oblivion"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 1)
	n, err = contColl.Find(bson.M{"hostaddr": "localhost", "appname": "oblivion"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 4)
	cont := container.Container{Container: types.Container{ID: "post-error", Name: "post-error-1", AppName: "oblivion"}}
	err = contColl.Insert(cont)
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{
		Name: cont.Name,
	}
	node, err := segSched.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: cont.AppName, ProcessName: "web"})
	c.Assert(err, check.ErrorMatches, `.*no nodes found with enough memory for container of "oblivion": 0.0191MB.*`)
	c.Assert(node, check.DeepEquals, cluster.Node{})
}

func (s *S) TestChooseNodeDistributesNodesEqually(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(10)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
		{Address: "http://server3:1234"},
		{Address: "http://server4:1234"},
	}
	contColl := s.p.Collection()
	defer contColl.Close()
	cont1 := container.Container{Container: types.Container{ID: "pre1", Name: "existingUnit1", AppName: "coolapp9", HostAddr: "server1"}}
	err := contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	cont2 := container.Container{Container: types.Container{ID: "pre2", Name: "existingUnit2", AppName: "coolapp9", HostAddr: "server2"}}
	err = contColl.Insert(cont2)
	c.Assert(err, check.Equals, nil)
	numberOfUnits := 58
	unitsPerNode := (numberOfUnits + 2) / 4
	wg := sync.WaitGroup{}
	wg.Add(numberOfUnits)
	sched := segregatedScheduler{provisioner: s.p}
	for i := 0; i < numberOfUnits; i++ {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprint(i)
			cont := container.Container{Container: types.Container{ID: id, Name: fmt.Sprintf("unit%d", i), AppName: "coolapp9"}}
			insertErr := contColl.Insert(cont)
			c.Assert(insertErr, check.IsNil)
			node, insertErr := sched.chooseNodeToAdd(nodes, cont.Name, "coolapp9", "web")
			c.Assert(insertErr, check.IsNil)
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
	contColl := s.p.Collection()
	defer contColl.Close()
	cont1 := container.Container{Container: types.Container{ID: "pre1", Name: "existingUnit1", AppName: "skyrim", HostAddr: "server1", ProcessName: "web"}}
	err := contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	cont2 := container.Container{Container: types.Container{ID: "pre2", Name: "existingUnit2", AppName: "skyrim", HostAddr: "server1", ProcessName: "web"}}
	err = contColl.Insert(cont2)
	c.Assert(err, check.IsNil)
	cont3 := container.Container{Container: types.Container{ID: "pre3", Name: "existingUnit3", AppName: "skyrim", HostAddr: "server1", ProcessName: "web"}}
	err = contColl.Insert(cont3)
	c.Assert(err, check.IsNil)
	numberOfUnits := 2
	wg := sync.WaitGroup{}
	wg.Add(numberOfUnits)
	sched := segregatedScheduler{provisioner: s.p}
	for i := 0; i < numberOfUnits; i++ {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprint(i)
			cont := container.Container{Container: types.Container{ID: id, Name: fmt.Sprintf("unit%d", i), AppName: "oblivion", ProcessName: "web"}}
			insertErr := contColl.Insert(cont)
			c.Assert(insertErr, check.IsNil)
			node, insertErr := sched.chooseNodeToAdd(nodes, cont.Name, "oblivion", "web")
			c.Assert(insertErr, check.IsNil)
			c.Assert(node, check.NotNil)
		}(i)
	}
	wg.Wait()
	n, err := contColl.Find(bson.M{"hostaddr": "server1"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 4)
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

func (s *S) TestChooseNodeDistributesNodesEquallyDifferentProcesses(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
	}
	contColl := s.p.Collection()
	defer contColl.Close()
	cont1 := container.Container{Container: types.Container{ID: "pre1", Name: "existingUnit1", AppName: "skyrim", HostAddr: "server1", ProcessName: "web"}}
	err := contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	cont2 := container.Container{Container: types.Container{ID: "pre2", Name: "existingUnit2", AppName: "skyrim", HostAddr: "server1", ProcessName: "web"}}
	err = contColl.Insert(cont2)
	c.Assert(err, check.Equals, nil)
	numberOfUnits := 2
	wg := sync.WaitGroup{}
	wg.Add(numberOfUnits)
	sched := segregatedScheduler{provisioner: s.p}
	for i := 0; i < numberOfUnits; i++ {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprint(i)
			cont := container.Container{Container: types.Container{ID: id, Name: fmt.Sprintf("unit%d", i), AppName: "skyrim", ProcessName: "worker"}}
			insertErr := contColl.Insert(cont)
			c.Assert(insertErr, check.IsNil)
			node, insertErr := sched.chooseNodeToAdd(nodes, cont.Name, "skyrim", "worker")
			c.Assert(insertErr, check.IsNil)
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
	n, err = contColl.Find(bson.M{"hostaddr": "server1", "processname": "worker"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 1)
	n, err = contColl.Find(bson.M{"hostaddr": "server2", "processname": "worker"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Check(n, check.Equals, 1)
}

func (s *S) TestChooseNodeDistributesNodesConsideringMetadata(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234", Metadata: map[string]string{
			"region": "a",
		}},
		{Address: "http://server2:1234", Metadata: map[string]string{
			"region": "a",
		}},
		{Address: "http://server3:1234", Metadata: map[string]string{
			"region": "b",
		}},
	}
	var i int
	addUnit := func(app string, process string) {
		i++
		sched := segregatedScheduler{provisioner: s.p}
		contColl := s.p.Collection()
		defer contColl.Close()
		cont := container.Container{Container: types.Container{Name: fmt.Sprintf("unit%d", i), AppName: app, ProcessName: process}}
		err := contColl.Insert(cont)
		c.Assert(err, check.IsNil)
		node, err := sched.chooseNodeToAdd(nodes, cont.Name, app, process)
		c.Assert(err, check.IsNil)
		c.Assert(node, check.Not(check.Equals), "")
	}
	addUnit("anomander", "rake")
	addUnit("anomander", "rake")
	contColl := s.p.Collection()
	defer contColl.Close()
	n1, err := contColl.Find(bson.M{"hostaddr": "server1"}).Count()
	c.Assert(err, check.Equals, nil)
	n2, err := contColl.Find(bson.M{"hostaddr": "server2"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Assert((n1 == 1 && n2 == 0) || (n1 == 0 && n2 == 1), check.Equals, true, check.Commentf("n1: %d, n2: %d", n1, n2))
	n3, err := contColl.Find(bson.M{"hostaddr": "server3"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Assert(n3, check.Equals, 1)
	addUnit("anomander", "rake")
	n1, err = contColl.Find(bson.M{"hostaddr": "server1"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Assert(n1, check.Equals, 1)
	n2, err = contColl.Find(bson.M{"hostaddr": "server2"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Assert(n2, check.Equals, 1)
	n3, err = contColl.Find(bson.M{"hostaddr": "server3"}).Count()
	c.Assert(err, check.Equals, nil)
	c.Assert(n3, check.Equals, 1)
}

func (s *S) TestChooseContainerToBeRemoved(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
	}
	contColl := s.p.Collection()
	defer contColl.Close()
	cont1 := container.Container{
		Container: types.Container{
			ID:          "pre1",
			Name:        "existingUnit1",
			AppName:     "coolapp9",
			HostAddr:    "server1",
			ProcessName: "web",
		},
	}
	err := contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	cont2 := container.Container{
		Container: types.Container{
			ID:          "pre2",
			Name:        "existingUnit2",
			AppName:     "coolapp9",
			HostAddr:    "server2",
			ProcessName: "web",
		},
	}
	err = contColl.Insert(cont2)
	c.Assert(err, check.Equals, nil)
	cont3 := container.Container{
		Container: types.Container{
			ID:          "pre3",
			Name:        "existingUnit1",
			AppName:     "coolapp9",
			HostAddr:    "server1",
			ProcessName: "web",
		},
	}
	err = contColl.Insert(cont3)
	c.Assert(err, check.Equals, nil)
	scheduler := segregatedScheduler{provisioner: s.p}
	containerID, err := scheduler.chooseContainerToRemove(nodes, "coolapp9", "web")
	c.Assert(err, check.IsNil)
	c.Assert(containerID, check.Equals, "pre1")
}

func (s *S) TestAggregateContainersByHostAppProcess(c *check.C) {
	contColl := s.p.Collection()
	defer contColl.Close()
	cont := container.Container{Container: types.Container{ID: "pre1", AppName: "app1", HostAddr: "server1", ProcessName: "web"}}
	err := contColl.Insert(cont)
	c.Assert(err, check.IsNil)
	cont = container.Container{Container: types.Container{ID: "pre2", AppName: "app1", HostAddr: "server1", ProcessName: ""}}
	err = contColl.Insert(cont)
	c.Assert(err, check.IsNil)
	cont = container.Container{Container: types.Container{ID: "pre3", AppName: "app2", HostAddr: "server1", ProcessName: ""}}
	err = contColl.Insert(cont)
	c.Assert(err, check.IsNil)
	cont = container.Container{Container: types.Container{ID: "pre4", AppName: "app1", HostAddr: "server2", ProcessName: ""}}
	err = contColl.Insert(cont)
	c.Assert(err, check.IsNil)
	err = contColl.Insert(map[string]string{"id": "pre5", "appname": "app1", "hostaddr": "server2"})
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	result, err := scheduler.aggregateContainersByHostAppProcess([]string{"server1", "server2"}, "app1", "")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]int{"server1": 1, "server2": 2})
}

func (s *S) TestChooseContainerToBeRemovedMultipleProcesses(c *check.C) {
	nodes := []cluster.Node{
		{Address: "http://server1:1234"},
		{Address: "http://server2:1234"},
	}
	contColl := s.p.Collection()
	defer contColl.Close()
	cont1 := container.Container{Container: types.Container{ID: "pre1", AppName: "coolapp9", HostAddr: "server1", ProcessName: "web"}}
	err := contColl.Insert(cont1)
	c.Assert(err, check.IsNil)
	cont2 := container.Container{Container: types.Container{ID: "pre2", AppName: "coolapp9", HostAddr: "server1", ProcessName: "web"}}
	err = contColl.Insert(cont2)
	c.Assert(err, check.IsNil)
	cont3 := container.Container{Container: types.Container{ID: "pre3", AppName: "coolapp9", HostAddr: "server1", ProcessName: "web"}}
	err = contColl.Insert(cont3)
	c.Assert(err, check.IsNil)
	cont4 := container.Container{Container: types.Container{ID: "pre4", AppName: "coolapp9", HostAddr: "server1", ProcessName: ""}}
	err = contColl.Insert(cont4)
	c.Assert(err, check.IsNil)
	cont5 := container.Container{Container: types.Container{ID: "pre5", AppName: "coolapp9", HostAddr: "server2", ProcessName: ""}}
	err = contColl.Insert(cont5)
	c.Assert(err, check.IsNil)
	err = contColl.Insert(map[string]string{"id": "pre6", "appname": "coolapp9", "hostaddr": "server2"})
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	containerID, err := scheduler.chooseContainerToRemove(nodes, "coolapp9", "")
	c.Assert(err, check.IsNil)
	c.Assert(containerID == "pre5" || containerID == "pre6", check.Equals, true)
}

func (s *S) TestGetContainerPreferablyFromHost(c *check.C) {
	contColl := s.p.Collection()
	defer contColl.Close()
	cont1 := container.Container{
		Container: types.Container{
			ID:          "pre1",
			Name:        "existingUnit1",
			AppName:     "coolapp9",
			HostAddr:    "server1",
			ProcessName: "some",
		},
	}
	err := contColl.Insert(cont1)
	c.Assert(err, check.Equals, nil)
	cont2 := container.Container{
		Container: types.Container{
			ID:          "pre2",
			Name:        "existingUnit2",
			AppName:     "coolapp9",
			HostAddr:    "serverX",
			ProcessName: "some",
		},
	}
	err = contColl.Insert(cont2)
	c.Assert(err, check.Equals, nil)
	scheduler := segregatedScheduler{provisioner: s.p}
	id, err := scheduler.getContainerPreferablyFromHost("server1", "coolapp9", "some")
	c.Assert(err, check.IsNil)
	c.Assert(id, check.Equals, "pre1")
	id, err = scheduler.getContainerPreferablyFromHost("server2", "coolapp9", "some")
	c.Assert(err, check.IsNil)
	c.Assert(id == "pre1" || id == "pre2", check.Equals, true, check.Commentf("id: %s", id))
	_, err = scheduler.getContainerPreferablyFromHost("server1", "coolapp9", "other")
	c.Assert(err, check.ErrorMatches, `Container of app "coolapp9" with process "other" was not found in any servers`)
}

func (s *S) TestGetContainerPreferablyFromHostEmptyProcess(c *check.C) {
	contColl := s.p.Collection()
	defer contColl.Close()
	err := contColl.Insert(map[string]string{"id": "pre1", "name": "unit1", "appname": "coolappX", "hostaddr": "server1"})
	c.Assert(err, check.Equals, nil)
	err = contColl.Insert(map[string]string{"id": "pre2", "name": "unit1", "appname": "coolappX", "hostaddr": "server2", "processname": ""})
	c.Assert(err, check.Equals, nil)
	scheduler := segregatedScheduler{provisioner: s.p}
	id, err := scheduler.getContainerPreferablyFromHost("server1", "coolappX", "")
	c.Assert(err, check.IsNil)
	c.Assert(id, check.Equals, "pre1")
	id, err = scheduler.getContainerPreferablyFromHost("server2", "coolappX", "")
	c.Assert(err, check.IsNil)
	c.Assert(id, check.Equals, "pre2")
	_, err = scheduler.getContainerPreferablyFromHost("server1", "coolappX", "other")
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetRemovableContainer(c *check.C) {
	a1 := app.App{Name: "impius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	cont1 := container.Container{Container: types.Container{ID: "1", Name: "impius1", AppName: a1.Name, ProcessName: "web"}}
	cont2 := container.Container{Container: types.Container{ID: "2", Name: "mirror1", AppName: a1.Name, ProcessName: "worker"}}
	a2 := app.App{Name: "notimpius", Teams: []string{"tsuruteam", "nodockerforme"}, Pool: "pool1"}
	cont3 := container.Container{Container: types.Container{ID: "3", Name: "dedication1", AppName: a2.Name, ProcessName: "web"}}
	cont4 := container.Container{Container: types.Container{ID: "4", Name: "dedication2", AppName: a2.Name, ProcessName: "worker"}}
	err := s.conn.Apps().Insert(a1)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	p := pool.Pool{Name: "pool1"}
	o := pool.AddPoolOptions{Name: p.Name}
	err = pool.AddPool(o)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(p.Name, []string{
		"tsuruteam",
		"nodockerforme",
	})
	c.Assert(err, check.IsNil)
	contColl := s.p.Collection()
	defer contColl.Close()
	err = contColl.Insert(
		cont1, cont2, cont3, cont4,
	)
	c.Assert(err, check.IsNil)
	scheduler := segregatedScheduler{provisioner: s.p}
	clusterInstance, err := cluster.New(&scheduler, &cluster.MapStorage{}, "")
	s.p.cluster = clusterInstance
	c.Assert(err, check.IsNil)
	server1, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server1.Stop()
	server2, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server2.Stop()
	localURL := strings.Replace(server2.URL(), "127.0.0.1", "localhost", -1)
	err = clusterInstance.Register(cluster.Node{
		Address:  server1.URL(),
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	err = clusterInstance.Register(cluster.Node{
		Address:  localURL,
		Metadata: map[string]string{"pool": "pool1"},
	})
	c.Assert(err, check.IsNil)
	opts := docker.CreateContainerOptions{Name: cont1.Name}
	_, err = scheduler.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: a1.Name, ProcessName: cont1.ProcessName})
	c.Assert(err, check.IsNil)
	opts = docker.CreateContainerOptions{Name: cont2.Name}
	_, err = scheduler.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: a1.Name, ProcessName: cont2.ProcessName})
	c.Assert(err, check.IsNil)
	opts = docker.CreateContainerOptions{Name: cont3.Name}
	_, err = scheduler.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: a2.Name, ProcessName: cont3.ProcessName})
	c.Assert(err, check.IsNil)
	opts = docker.CreateContainerOptions{Name: cont4.Name}
	_, err = scheduler.Schedule(clusterInstance, &opts, &container.SchedulerOpts{AppName: a2.Name, ProcessName: cont4.ProcessName})
	c.Assert(err, check.IsNil)
	cont, err := scheduler.GetRemovableContainer(a1.Name, "web")
	c.Assert(err, check.IsNil)
	c.Assert(cont, check.Equals, cont1.ID)
	err = cont1.Remove(s.p.ClusterClient(), s.p.ActionLimiter())
	c.Assert(err, check.IsNil)
	_, err = scheduler.GetRemovableContainer(a1.Name, "web")
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetRemovableContainerWithoutAppOrProcess(c *check.C) {
	scheduler := segregatedScheduler{provisioner: s.p}
	cont, err := scheduler.GetRemovableContainer("", "web")
	c.Assert(cont, check.Equals, "")
	c.Assert(err, check.NotNil)
	cont, err = scheduler.GetRemovableContainer("appname", "")
	c.Assert(cont, check.Equals, "")
	c.Assert(err, check.NotNil)
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
	c.Assert(hosts, check.HasLen, 2)
	c.Assert(hostsMap[hosts[0]], check.Equals, nodes[0].Address)
}

func (s *S) TestChooseContainerToBeRemovedTable(c *check.C) {
	tests := []struct {
		nodes     []cluster.Node
		conts     []container.Container
		app, proc string
		expected  []string
	}{
		{
			nodes: []cluster.Node{
				{Address: "http://server1:1234"},
				{Address: "http://server2:1234"},
			},
			conts: []container.Container{
				{Container: types.Container{AppName: "a1", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a1", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a1", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a2", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a2", HostAddr: "server2", ProcessName: "web"}},
				{Container: types.Container{AppName: "a2", HostAddr: "server2", ProcessName: "web"}},
			},
			app:      "a2",
			proc:     "web",
			expected: []string{"id-4", "id-5"},
		},
		{
			nodes: []cluster.Node{
				{Address: "http://server1:1234"},
				{Address: "http://server2:1234"},
			},
			conts: []container.Container{
				{Container: types.Container{AppName: "a1", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a1", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a1", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a1", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a2", HostAddr: "server2", ProcessName: "web"}},
			},
			app:      "a2",
			proc:     "web",
			expected: []string{"id-4"},
		},
		{
			nodes: []cluster.Node{
				{Address: "http://server1:1234", Metadata: map[string]string{"net": "1"}},
				{Address: "http://server2:1234", Metadata: map[string]string{"net": "2"}},
				{Address: "http://server3:1234", Metadata: map[string]string{"net": "2"}},
			},
			conts: []container.Container{
				{Container: types.Container{AppName: "a1", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a2", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a2", HostAddr: "server1", ProcessName: "web"}},
				{Container: types.Container{AppName: "a1", HostAddr: "server2", ProcessName: "web"}},
				{Container: types.Container{AppName: "a1", HostAddr: "server3", ProcessName: "web"}},
			},
			app:      "a1",
			proc:     "web",
			expected: []string{"id-3", "id-4"},
		},
		{
			nodes: []cluster.Node{
				{Address: "http://server1:1234"},
				{Address: "http://server2:1234"},
				{Address: "http://server3:1234"},
			},
			conts: []container.Container{
				{Container: types.Container{AppName: "a1", HostAddr: "server3", ProcessName: "web"}},
				{Container: types.Container{AppName: "a2", HostAddr: "server4", ProcessName: "web"}},
			},
			app:      "a2",
			proc:     "web",
			expected: []string{"id-1"},
		},
	}
	for i, tt := range tests {
		scheduler := segregatedScheduler{provisioner: s.p}
		contColl := s.p.Collection()
		contColl.RemoveAll(nil)
		for j, cont := range tt.conts {
			cont.ID = fmt.Sprintf("id-%d", j)
			contColl.Insert(cont)
		}
		contColl.Close()
		containerID, err := scheduler.chooseContainerToRemove(tt.nodes, tt.app, tt.proc)
		c.Assert(err, check.IsNil, check.Commentf("test %d", i))
		found := false
		for _, e := range tt.expected {
			if containerID == e {
				found = true
				break
			}
		}
		c.Assert(found, check.Equals, true, check.Commentf("test %d: containerID: %s, expected: %v", i, containerID, tt.expected))
	}
}
