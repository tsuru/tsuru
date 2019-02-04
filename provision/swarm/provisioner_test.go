// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/swarm"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	dockerTesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/safe"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
)

func (s *S) TestInitialize(c *check.C) {
	config.Set("swarm:swarm-port", 0)
	err := s.p.Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(swarmConfig.swarmPort, check.Equals, 0)
	config.Unset("swarm:swarm-port")
	err = s.p.Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(swarmConfig.swarmPort, check.Equals, 2377)
	config.Unset("swarm:swarm-port")
	err = s.p.Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(swarmConfig.swarmPort, check.Equals, 2377)
}

func (s *S) TestProvision(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	cli, err := newClient(s.clusterSrv.URL(), nil)
	c.Assert(err, check.IsNil)
	nets, err := cli.ListNetworks()
	c.Assert(err, check.IsNil)
	c.Assert(nets, check.HasLen, 1)
	expected := docker.Network{ID: nets[0].ID, Name: "app-myapp-overlay", Driver: "overlay", Containers: map[string]docker.Endpoint{}}
	c.Assert(nets, check.DeepEquals, []docker.Network{expected})
}

func (s *S) TestAddNode(c *check.C) {
	s.addCluster(c)
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	metadata := map[string]string{"m1": "v1", "m2": "v2"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Pool:     "p1",
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(srv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, srv.URL())
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.m1":   "v1",
		"tsuru.m2":   "v2",
		"tsuru.pool": "p1",
	})
	c.Assert(node.Pool(), check.Equals, "p1")
	c.Assert(node.Status(), check.Equals, "ready")
}

func (s *S) TestAddNodeWithPrefix(c *check.C) {
	s.addCluster(c)
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	metadata := map[string]string{"m1": "v1", "tsuru.m2": "v2", "pool": "ignored1", "tsuru.pool": "ignored2"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Pool:     "p1",
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(srv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, srv.URL())
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.m1":   "v1",
		"tsuru.m2":   "v2",
		"tsuru.pool": "p1",
	})
	c.Assert(node.Pool(), check.Equals, "p1")
	c.Assert(node.Status(), check.Equals, "ready")
}

func (s *S) TestAddNodeAlreadyInSwarm(c *check.C) {
	s.addCluster(c)
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	cli, err := docker.NewClient(srv.URL())
	c.Assert(err, check.IsNil)
	err = joinSwarm(s.clusterCli, cli, srv.URL())
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "m2": "v2"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
		Pool:     "pxyz",
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(srv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, srv.URL())
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.m1":   "v1",
		"tsuru.m2":   "v2",
		"tsuru.pool": "pxyz",
	})
	c.Assert(node.Pool(), check.Equals, "pxyz")
	c.Assert(node.Status(), check.Equals, "ready")
}

func (s *S) TestAddNodeMultiple(c *check.C) {
	s.addCluster(c)
	var addrs []string
	for i := 0; i < 5; i++ {
		srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
		c.Assert(err, check.IsNil)
		addrs = append(addrs, srv.URL())
		defer srv.Stop()
		metadata := map[string]string{"count": fmt.Sprintf("%d", i)}
		opts := provision.AddNodeOptions{
			Address:  srv.URL(),
			Pool:     "p1",
			Metadata: metadata,
		}
		err = s.p.AddNode(opts)
		c.Assert(err, check.IsNil, check.Commentf("server %d", i))
	}
	nodes, err := s.p.ListNodes(addrs)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 5)
	for i, n := range nodes {
		c.Assert(n.Metadata(), check.DeepEquals, map[string]string{
			"tsuru.count": fmt.Sprintf("%d", i),
			"tsuru.pool":  "p1",
		})
	}
}

func (s *S) TestAddNodeTLS(c *check.C) {
	s.addTLSCluster(c)
	caPath := tmpFileWith(c, testCA)
	certPath := tmpFileWith(c, testServerCert)
	keyPath := tmpFileWith(c, testServerKey)
	defer os.Remove(certPath)
	defer os.Remove(keyPath)
	defer os.Remove(caPath)
	srv, err := testing.NewTLSServer("127.0.0.1:0", nil, nil, testing.TLSConfig{
		RootCAPath:  caPath,
		CertPath:    certPath,
		CertKeyPath: keyPath,
	})
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	url := strings.Replace(srv.URL(), "http://", "https://", 1)
	metadata := map[string]string{"m1": "v1", "m2": "v2"}
	opts := provision.AddNodeOptions{
		Address:  url,
		Pool:     "p1",
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(url)
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, url)
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.m1":   "v1",
		"tsuru.m2":   "v2",
		"tsuru.pool": "p1",
	})
	c.Assert(node.Pool(), check.Equals, "p1")
	c.Assert(node.Status(), check.Equals, "ready")
}

func (s *S) TestListNodes(c *check.C) {
	s.addCluster(c)
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	metadata := map[string]string{"m1": "v1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Pool:     "p1",
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	nodes, err = s.p.ListNodes([]string{srv.URL()})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, srv.URL())
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"tsuru.m1":   "v1",
		"tsuru.pool": "p1",
	})
	c.Assert(nodes[0].Pool(), check.DeepEquals, "p1")
	c.Assert(nodes[0].Status(), check.DeepEquals, "ready")
}

func (s *S) TestListNodesEmpty(c *check.C) {
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
}

func (s *S) TestRestart(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.Restart(a, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	err = s.p.Restart(a, "", nil)
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestRestartExisting(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	err = s.p.Restart(a, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	cli, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	service, err := cli.InspectService("myapp-web")
	c.Assert(err, check.IsNil)
	l := provision.LabelSet{Labels: service.Spec.TaskTemplate.ContainerSpec.Labels, Prefix: tsuruLabelPrefix}
	c.Assert(l.Restarts(), check.Equals, 1)
}

func (s *S) TestStopStart(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	err = s.p.Stop(a, "")
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
	err = s.p.Start(a, "")
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
}

func (s *S) TestStopStartSingleProcess(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "python myworker.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "worker", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	err = s.p.Stop(a, "worker")
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units[0].ProcessName, check.Equals, "web")
	err = s.p.Start(a, "worker")
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	procs := []string{units[0].ProcessName, units[1].ProcessName}
	sort.Strings(procs)
	c.Assert(procs, check.DeepEquals, []string{"web", "worker"})
}

func (s *S) TestUnits(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	expected := []provision.Unit{
		{ID: units[0].ID, Name: "", AppName: "myapp", ProcessName: "web", Type: "", IP: "127.0.0.1", Status: "starting", Address: &url.URL{}},
	}
	c.Assert(units, check.DeepEquals, expected)
}

func (s *S) TestUnitsWithShutdownTasks(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	cli, err := newClient(s.clusterSrv.URL(), nil)
	c.Assert(err, check.IsNil)
	tasks, err := cli.ListTasks(docker.ListTasksOptions{})
	c.Assert(err, check.IsNil)
	tasks[0].DesiredState = swarm.TaskStateShutdown
	err = s.clusterSrv.MutateTask(tasks[0].ID, tasks[0])
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestUnitsWithNoNodeIDServiceIDTasks(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	cli, err := newClient(s.clusterSrv.URL(), nil)
	c.Assert(err, check.IsNil)
	tasks, err := cli.ListTasks(docker.ListTasksOptions{})
	c.Assert(err, check.IsNil)
	oldNodeID := tasks[0].NodeID
	tasks[0].NodeID = ""
	err = s.clusterSrv.MutateTask(tasks[0].ID, tasks[0])
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
	tasks[0].NodeID = oldNodeID
	tasks[0].ServiceID = ""
	err = s.clusterSrv.MutateTask(tasks[0].ID, tasks[0])
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestUnitsWithoutSwarmCluster(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	s.mockService.Cluster.OnFindByPool = func(prov, pool string) (*provTypes.Cluster, error) {
		return nil, provTypes.ErrNoCluster
	}
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestRoutableUnits(c *check.C) {
	s.addCluster(c)
	err := s.p.UpdateNode(provision.UpdateNodeOptions{
		Address:  s.clusterSrv.URL(),
		Metadata: map[string]string{"pool": "px"},
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(pool.AddPoolOptions{Name: "px", Public: true, Provisioner: "swarm"})
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1, Pool: "px"}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 10, "web", nil)
	c.Assert(err, check.IsNil)
	addrs, err := s.p.RoutableAddresses(a)
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []url.URL{
		{Scheme: "http", Host: "127.0.0.1:30000"},
	})
}

func (s *S) TestRoutableUnitsNoNodesInPool(c *check.C) {
	s.addCluster(c)
	err := s.p.UpdateNode(provision.UpdateNodeOptions{
		Address:  s.clusterSrv.URL(),
		Metadata: map[string]string{"pool": "py"},
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(pool.AddPoolOptions{Name: "px", Public: true, Provisioner: "swarm"})
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1, Pool: "px"}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 10, "web", nil)
	c.Assert(err, check.IsNil)
	addrs, err := s.p.RoutableAddresses(a)
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []url.URL{})
}

func (s *S) TestAddUnits(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	err = s.p.AddUnits(a, 2, "", nil)
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 5)
}

func (s *S) TestAddUnitsMultipleProcesses(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "python myworker.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	err = s.p.AddUnits(a, 1, "worker", nil)
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
}

func (s *S) TestAddUnitsNoDeploys(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.ErrorMatches, `units can only be modified after the first deploy`)
}

func (s *S) TestAddUnitsNoProcessWithMultiple(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "python myworker.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "", nil)
	c.Assert(err, check.ErrorMatches, `process error: no process name specified and more than one declared in Procfile`)
}

func (s *S) TestAddUnitsNoImage(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.ErrorMatches, `no process information found deploying image "registry.tsuru.io/tsuru/app-myapp"`)
}

func (s *S) TestAddUnitsZeroUnits(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 0, "web", nil)
	c.Assert(err, check.ErrorMatches, `cannot change 0 units`)
}

func (s *S) TestAddUnitsWithHealthcheck(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
		"healthcheck": provision.TsuruYamlHealthcheck{
			Path: "/hc",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	cli, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	service, err := cli.InspectService(serviceNameForApp(a, "web"))
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.ContainerSpec.Healthcheck, check.DeepEquals, &container.HealthConfig{
		Test: []string{
			"CMD-SHELL",
			"curl -k -XGET -fsSL http://localhost:8888/hc -o/dev/null -w '%{http_code}' | grep 200",
		},
		Timeout:  60 * time.Second,
		Retries:  1,
		Interval: 3 * time.Second,
	})
}

func (s *S) TestRemoveUnits(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	err = s.p.RemoveUnits(a, 2, "web", nil)
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	err = s.p.RemoveUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
	err = s.p.RemoveUnits(a, 1, "web", nil)
	c.Assert(err, check.ErrorMatches, `cannot have less than 0 units`)
}

func (s *S) TestGetNode(c *check.C) {
	s.addCluster(c)
	node, err := s.p.GetNode(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, s.clusterSrv.URL())
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.pool": "bonehunters",
	})
	c.Assert(node.Pool(), check.DeepEquals, "bonehunters")
	c.Assert(node.Status(), check.DeepEquals, "ready")
}

func (s *S) TestGetNodeNotFound(c *check.C) {
	_, err := s.p.GetNode("http://tai.shar.malkier")
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestRemoveNode(c *check.C) {
	s.addCluster(c)
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	err = s.p.RemoveNode(provision.RemoveNodeOptions{
		Address: srv.URL(),
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.GetNode(srv.URL())
	c.Assert(errors.Cause(err), check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestRemoveLastNodeError(c *check.C) {
	s.addCluster(c)
	err := s.p.RemoveNode(provision.RemoveNodeOptions{
		Address: s.clusterSrv.URL(),
	})
	c.Assert(err, check.ErrorMatches, `cannot remove last node from swarm, remove the cluster from tsuru to remove it`)
}

func (s *S) TestRemoveNodeRebalance(c *check.C) {
	s.addCluster(c)
	var reqs []*http.Request
	hook := func(r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/nodes/") {
			reqs = append(reqs, r)
		}
	}
	s.clusterSrv.SetHook(hook)
	srv, err := testing.NewServer("127.0.0.1:0", nil, hook)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	reqs = nil
	err = s.p.RemoveNode(provision.RemoveNodeOptions{
		Address:   srv.URL(),
		Rebalance: true,
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.GetNode(srv.URL())
	c.Assert(errors.Cause(err), check.Equals, provision.ErrNodeNotFound)
	c.Assert(reqs, check.HasLen, 2)
	c.Assert(reqs[0].Method, check.Equals, "POST")
	c.Assert(reqs[1].Method, check.Equals, "DELETE")
}

func (s *S) TestRemoveNodeNotFound(c *check.C) {
	err := s.p.RemoveNode(provision.RemoveNodeOptions{
		Address: "localhost:1000",
	})
	c.Assert(errors.Cause(err), check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestUpdateNode(c *check.C) {
	s.addCluster(c)
	err := s.p.UpdateNode(provision.UpdateNodeOptions{
		Address:  s.clusterSrv.URL(),
		Metadata: map[string]string{"m1": "v2", "m2": "v3"},
	})
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.m1":   "v2",
		"tsuru.m2":   "v3",
		"tsuru.pool": "bonehunters",
	})
}

func (s *S) TestUpdateNodeNewPool(c *check.C) {
	s.addCluster(c)
	err := s.p.UpdateNode(provision.UpdateNodeOptions{
		Address:  s.clusterSrv.URL(),
		Pool:     "pxyz",
		Metadata: map[string]string{"m1": "v2", "m2": "v3"},
	})
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Pool(), check.Equals, "pxyz")
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.m1":   "v2",
		"tsuru.m2":   "v3",
		"tsuru.pool": "pxyz",
	})
}

func (s *S) TestUpdateNodeNoPreviousMetadata(c *check.C) {
	clusterSrv, err := dockerTesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer clusterSrv.Stop()
	clust := &provTypes.Cluster{
		Addresses:   []string{clusterSrv.URL()},
		Default:     true,
		Name:        "c1",
		Provisioner: provisionerName,
	}
	s.mockService.Cluster.OnFindByProvisioner = func(prov string) ([]provTypes.Cluster, error) {
		return []provTypes.Cluster{*clust}, nil
	}
	s.mockService.Cluster.OnFindByPool = func(prov, pool string) (*provTypes.Cluster, error) {
		return clust, nil
	}
	prov, err := provision.Get(clust.Provisioner)
	c.Assert(err, check.IsNil)
	if clusterProv, ok := prov.(cluster.ClusteredProvisioner); ok {
		err = clusterProv.InitializeCluster(clust)
		c.Assert(err, check.IsNil)
	}
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address:  "http://127.0.0.1:2375",
		Metadata: map[string]string{"m1": "v2", "m2": "v3"},
	})
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode("http://127.0.0.1:2375")
	c.Assert(err, check.IsNil)
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.m1": "v2",
		"tsuru.m2": "v3",
	})
}

func (s *S) TestUpdateNodeDisableEnable(c *check.C) {
	s.addCluster(c)
	err := s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: s.clusterSrv.URL(),
		Disable: true,
	})
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.pool": "bonehunters",
	})
	c.Assert(node.Status(), check.Equals, "ready (pause)")
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: s.clusterSrv.URL(),
		Enable:  true,
	})
	c.Assert(err, check.IsNil)
	node, err = s.p.GetNode(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Status(), check.Equals, "ready")
}

func (s *S) TestUpdateNodeNotFound(c *check.C) {
	err := s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: "localhost:1000",
	})
	c.Assert(errors.Cause(err), check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestRegisterUnit(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	cli, err := clusterForPool(a.GetPool())
	c.Assert(err, check.IsNil)
	set, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			IsDeploy:    true,
			BuildImage:  "app:v1",
			Provisioner: provisionerName,
			Prefix:      tsuruLabelPrefix,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: &swarm.ContainerSpec{
					Labels: set.ToLabels(),
				},
			},
			Annotations: swarm.Annotations{
				Name:   "myapp-web",
				Labels: set.ToLabels(),
			},
		},
	})
	c.Assert(err, check.IsNil)
	tasks, err := cli.ListTasks(docker.ListTasksOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(tasks, check.HasLen, 1)
	err = s.p.RegisterUnit(a, tasks[0].Status.ContainerStatus.ContainerID, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	data, err := image.GetImageMetaData("app:v1")
	c.Assert(err, check.IsNil)
	c.Assert(data.Processes, check.DeepEquals, map[string][]string{"web": {"python myapp.py"}})
}

func (s *S) TestRegisterUnitNotBuild(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	cli, err := clusterForPool(a.GetPool())
	c.Assert(err, check.IsNil)
	set, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			BuildImage:  "notset:v1",
			Provisioner: provisionerName,
			Prefix:      tsuruLabelPrefix,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: &swarm.ContainerSpec{
					Labels: set.ToLabels(),
				},
			},
			Annotations: swarm.Annotations{
				Name:   "myapp-web",
				Labels: set.ToLabels(),
			},
		},
	})
	c.Assert(err, check.IsNil)
	conts, err := cli.ListContainers(docker.ListContainersOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 1)
	err = s.p.RegisterUnit(a, conts[0].ID, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	data, err := image.GetImageMetaData("notset:v1")
	c.Assert(err, check.IsNil)
	c.Assert(data, check.DeepEquals, image.ImageMetadata{})
}

func (s *S) TestRegisterUnitNoImageLabel(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	cli, err := clusterForPool(a.GetPool())
	c.Assert(err, check.IsNil)
	set, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Provisioner: provisionerName,
			IsDeploy:    true,
			Prefix:      tsuruLabelPrefix,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: &swarm.ContainerSpec{
					Labels: set.ToLabels(),
				},
			},
			Annotations: swarm.Annotations{
				Name:   "myapp-web",
				Labels: set.ToLabels(),
			},
		},
	})
	c.Assert(err, check.IsNil)
	tasks, err := cli.ListTasks(docker.ListTasksOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(tasks, check.HasLen, 1)
	err = s.p.RegisterUnit(a, tasks[0].Status.ContainerStatus.ContainerID, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.ErrorMatches, `invalid build image label for build task: .*`)
}

func (s *S) TestRegisterUnitWaitsContainerID(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	cli, err := clusterForPool(a.GetPool())
	c.Assert(err, check.IsNil)
	set, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			IsDeploy:    true,
			BuildImage:  "app:v1",
			Provisioner: provisionerName,
			Prefix:      tsuruLabelPrefix,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: &swarm.ContainerSpec{
					Labels: set.ToLabels(),
				},
			},
			Annotations: swarm.Annotations{
				Name:   "myapp-web",
				Labels: set.ToLabels(),
			},
		},
	})
	c.Assert(err, check.IsNil)
	tasks, err := cli.ListTasks(docker.ListTasksOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(tasks, check.HasLen, 1)
	task := tasks[0]
	contID := task.Status.ContainerStatus.ContainerID
	task.Status.ContainerStatus.ContainerID = ""
	s.clusterSrv.MutateTask(task.ID, task)
	go func() {
		time.Sleep(time.Second)
		task.Status.ContainerStatus = &swarm.ContainerStatus{
			ContainerID: contID,
		}
		s.clusterSrv.MutateTask(task.ID, task)
	}()
	err = s.p.RegisterUnit(a, contID, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	data, err := image.GetImageMetaData("app:v1")
	c.Assert(err, check.IsNil)
	c.Assert(data.Processes, check.DeepEquals, map[string][]string{"web": {"python myapp.py"}})
}

func (s *S) TestDeploy(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	attached := s.attachRegister(c, s.clusterSrv, true, a)
	tags := []string{}
	s.clusterSrv.CustomHandler("/images/.*/push", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.Path, check.Equals, "/images/registry.tsuru.io/tsuru/app-myapp/push")
		tags = append(tags, r.URL.Query().Get("tag"))
		s.clusterSrv.DefaultHandler().ServeHTTP(w, r)
	}))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	builderImgID := "registry.tsuru.io/tsuru/app-myapp:v1-builder"
	pullOpts := docker.PullImageOptions{
		Repository: "tsuru/app-myapp",
		Tag:        "v1-builder",
	}
	cli, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	err = cli.PullImage(pullOpts, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	imgID, err := s.p.Deploy(a, builderImgID, evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(<-attached, check.Equals, true)
	c.Assert(tags, check.DeepEquals, []string{"v1", "latest"})
	c.Assert(imgID, check.Equals, "registry.tsuru.io/tsuru/app-myapp:v1")
	dbImg, err := image.AppCurrentImageName(a.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(dbImg, check.Equals, "registry.tsuru.io/tsuru/app-myapp:v1")
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{ID: units[0].ID, AppName: a.Name, Type: "whitespace", ProcessName: "web", IP: "127.0.0.1", Status: "starting", Address: &url.URL{}},
	})
	task, err := cli.InspectTask(units[0].ID)
	c.Assert(err, check.IsNil)
	cont, err := cli.InspectContainer(task.Status.ContainerStatus.ContainerID)
	c.Assert(err, check.IsNil)
	c.Assert(cont.Config.Entrypoint, check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		fmt.Sprintf(
			"[ -d /home/application/current ] && cd /home/application/current; %s && exec python myapp.py",
			extraRegisterCmds(a),
		),
	})
}

func (s *S) TestDeployImageID(c *check.C) {
	s.addCluster(c)
	cli, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": []string{"/bin/sh", "-c", "python test.py"},
		},
	}
	err = image.SaveImageCustomData("registry.tsuru.io/tsuru/app-"+a.Name+":v1", customData)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	builderImgID := "registry.tsuru.io/tsuru/app-myapp:v1"
	pullOpts := docker.PullImageOptions{
		Repository: "tsuru/app-myapp",
		Tag:        "v1",
	}
	err = cli.PullImage(pullOpts, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	deployedImg, err := s.p.Deploy(a, builderImgID, evt)
	c.Assert(err, check.IsNil)
	c.Assert(deployedImg, check.Equals, "registry.tsuru.io/tsuru/app-myapp:v1")
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{ID: units[0].ID, AppName: a.Name, ProcessName: "web", IP: "127.0.0.1", Status: "starting", Address: &url.URL{}},
	})
	dbImg, err := image.AppCurrentImageName(a.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(dbImg, check.Equals, "registry.tsuru.io/tsuru/app-myapp:v1")
	task, err := cli.InspectTask(units[0].ID)
	c.Assert(err, check.IsNil)
	cont, err := cli.InspectContainer(task.Status.ContainerStatus.ContainerID)
	c.Assert(err, check.IsNil)
	c.Assert(cont.Config.Entrypoint, check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		fmt.Sprintf(
			"[ -d /home/application/current ] && cd /home/application/current; %s && exec $0 \"$@\"",
			extraRegisterCmds(a),
		),
		"/bin/sh", "-c", "python test.py",
	})
}

func (s *S) TestDestroy(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	err = s.p.Destroy(a)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestDestroyServiceNotFound(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = s.p.Destroy(a)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestExecuteCommandWithStdinToUnit(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	client, _ := docker.NewClient(s.clusterSrv.URL())
	task, err := client.InspectTask(units[0].ID)
	c.Assert(err, check.IsNil)
	task.DesiredState = swarm.TaskStateRunning
	err = s.clusterSrv.MutateTask(task.ID, *task)
	c.Assert(err, check.IsNil)
	var urls struct {
		items []url.URL
		sync.Mutex
	}
	s.clusterSrv.PrepareExec("*", func() {
		time.Sleep(time.Second)
	})
	s.clusterSrv.SetHook(func(r *http.Request) {
		urls.Lock()
		urls.items = append(urls.items, *r.URL)
		urls.Unlock()
	})
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: conn,
		Stdin:  conn,
		Stderr: conn,
		Width:  140,
		Height: 38,
		Term:   "xterm",
		Units:  []string{units[0].ID},
		Cmds:   []string{"cmd1", "arg1"},
	})
	c.Assert(err, check.IsNil)
	urls.Lock()
	resizeURL := urls.items[len(urls.items)-2]
	urls.Unlock()
	execResizeRegexp := regexp.MustCompile(`^.*/exec/(.*)/resize$`)
	matches := execResizeRegexp.FindStringSubmatch(resizeURL.Path)
	c.Assert(matches, check.HasLen, 2)
	c.Assert(resizeURL.Query().Get("w"), check.Equals, "140")
	c.Assert(resizeURL.Query().Get("h"), check.Equals, "38")
	exec, err := client.InspectExec(matches[1])
	c.Assert(err, check.IsNil)
	cmd := append([]string{exec.ProcessConfig.EntryPoint}, exec.ProcessConfig.Arguments...)
	c.Assert(cmd, check.DeepEquals, []string{"/usr/bin/env", "TERM=xterm", "cmd1", "arg1"})
}

func (s *S) TestExecuteCommandWithStdinNoUnit(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 2, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	client, _ := docker.NewClient(s.clusterSrv.URL())
	task, err := client.InspectTask(units[1].ID)
	c.Assert(err, check.IsNil)
	task.DesiredState = swarm.TaskStateRunning
	err = s.clusterSrv.MutateTask(task.ID, *task)
	c.Assert(err, check.IsNil)
	var urls struct {
		items []url.URL
		sync.Mutex
	}
	hook := func(r *http.Request) {
		urls.Lock()
		urls.items = append(urls.items, *r.URL)
		urls.Unlock()
	}
	s.clusterSrv.SetHook(hook)
	attached := s.attachRegisterHook(c, s.clusterSrv, false, a, hook)
	var service *swarm.Service
	s.clusterSrv.CustomHandler("/services/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.clusterSrv.DefaultHandler().ServeHTTP(w, r)
		service, err = client.InspectService("myapp-isolated-run")
		c.Assert(err, check.IsNil)
	}))
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: conn,
		Stdin:  conn,
		Stderr: conn,
		Width:  140,
		Height: 38,
		Term:   "xterm",
		Cmds:   []string{"cmd1", "arg1"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(<-attached, check.Equals, true)
	_, err = client.InspectService("myapp-isolated-run")
	c.Assert(err, check.DeepEquals, &docker.NoSuchService{ID: "myapp-isolated-run"})
	l := provision.LabelSet{Labels: service.Spec.Labels, Prefix: tsuruLabelPrefix}
	c.Assert(l.IsIsolatedRun(), check.Equals, true)
	urls.Lock()
	resizeURL := urls.items[3]
	urls.Unlock()
	execResizeRegexp := regexp.MustCompile(`^.*/containers/(.*)/resize$`)
	matches := execResizeRegexp.FindStringSubmatch(resizeURL.Path)
	c.Assert(matches, check.HasLen, 2)
	c.Assert(resizeURL.Query().Get("w"), check.Equals, "140")
	c.Assert(resizeURL.Query().Get("h"), check.Equals, "38")
	c.Assert(matches[1], check.Not(check.Equals), task.Status.ContainerStatus.ContainerID)
	c.Assert(service.Spec.TaskTemplate.ContainerSpec.Command, check.DeepEquals, []string{"/usr/bin/env", "TERM=xterm", "cmd1", "arg1"})
}

func (s *S) TestExecuteCommand(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	client, _ := docker.NewClient(s.clusterSrv.URL())
	task, err := client.InspectTask(units[0].ID)
	c.Assert(err, check.IsNil)
	task.DesiredState = swarm.TaskStateRunning
	err = s.clusterSrv.MutateTask(task.ID, *task)
	c.Assert(err, check.IsNil)
	task, err = client.InspectTask(units[2].ID)
	c.Assert(err, check.IsNil)
	task.DesiredState = swarm.TaskStateRunning
	err = s.clusterSrv.MutateTask(task.ID, *task)
	c.Assert(err, check.IsNil)
	var executed int
	s.clusterSrv.SetHook(func(r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/exec") {
			s.clusterSrv.PrepareExec("*", func() {
				executed++
			})
		}
	})
	var stdout, stderr bytes.Buffer
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: &stdout,
		Stderr: &stderr,
		Units:  []string{units[0].ID, units[2].ID},
		Cmds:   []string{"ls", "-l"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(executed, check.Equals, 2)
}

func (s *S) TestExecuteCommandSingleUnit(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	client, err := clusterForPool(a.GetPool())
	c.Assert(err, check.IsNil)
	task, err := client.InspectTask(units[0].ID)
	c.Assert(err, check.IsNil)
	task.DesiredState = swarm.TaskStateRunning
	err = s.clusterSrv.MutateTask(task.ID, *task)
	c.Assert(err, check.IsNil)
	task, err = client.InspectTask(units[2].ID)
	c.Assert(err, check.IsNil)
	task.DesiredState = swarm.TaskStateRunning
	err = s.clusterSrv.MutateTask(task.ID, *task)
	c.Assert(err, check.IsNil)
	var executed int
	s.clusterSrv.SetHook(func(r *http.Request) {
		s.clusterSrv.PrepareExec("*", func() {
			executed++
		})
	})
	var stdout, stderr bytes.Buffer
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: &stdout,
		Stderr: &stderr,
		Units:  []string{units[0].ID},
		Cmds:   []string{"ls", "-l"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(executed, check.Equals, 1)
}

func (s *S) TestExecuteCommandNoUnits(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	attached := s.attachRegister(c, s.clusterSrv, false, a)
	var stdout, stderr bytes.Buffer
	var service *swarm.Service
	client, _ := docker.NewClient(s.clusterSrv.URL())
	s.clusterSrv.CustomHandler("/services/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.clusterSrv.DefaultHandler().ServeHTTP(w, r)
		service, err = client.InspectService("myapp-isolated-run")
		c.Assert(err, check.IsNil)
	}))
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: &stdout,
		Stderr: &stderr,
		Cmds:   []string{"ls", "-l"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(<-attached, check.Equals, true)
	_, err = client.InspectService("myapp-isolated-run")
	c.Assert(err, check.DeepEquals, &docker.NoSuchService{ID: "myapp-isolated-run"})
	l := provision.LabelSet{Labels: service.Spec.Labels, Prefix: tsuruLabelPrefix}
	c.Assert(l.IsIsolatedRun(), check.Equals, true)
}

func (s *S) TestUpgradeNodeContainerCreatesBaseService(c *check.C) {
	s.addCluster(c)
	client, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err = nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	err = s.p.UpgradeNodeContainer("bs", "", ioutil.Discard)
	c.Assert(err, check.IsNil)
	service, err := client.InspectService("node-container-bs-all")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.Placement.Constraints, check.DeepEquals, []string(nil))
}

func (s *S) TestUpgradeNodeContainerCreatesLimitedService(c *check.C) {
	s.addCluster(c)
	client, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err = nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	err = s.p.UpgradeNodeContainer("bs", "", ioutil.Discard)
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &c1)
	c.Assert(err, check.IsNil)
	err = s.p.UpgradeNodeContainer("bs", "p1", ioutil.Discard)
	c.Assert(err, check.IsNil)
	err = s.p.UpgradeNodeContainer("bs", "p1", ioutil.Discard)
	c.Assert(err, check.IsNil)
	service, err := client.InspectService("node-container-bs-all")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.Placement.Constraints, check.DeepEquals, []string{"node.labels.tsuru.pool != p1"})
	service, err = client.InspectService("node-container-bs-p1")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.Placement.Constraints, check.DeepEquals, []string{"node.labels.tsuru.pool == p1"})
}

func (s *S) TestUpgradeNodeContainerBaseUpgradesSpecifics(c *check.C) {
	s.addCluster(c)
	client, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err = nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &c1)
	c.Assert(err, check.IsNil)
	err = s.p.UpgradeNodeContainer("bs", "", ioutil.Discard)
	c.Assert(err, check.IsNil)
	service, err := client.InspectService("node-container-bs-all")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.Placement.Constraints, check.DeepEquals, []string{"node.labels.tsuru.pool != p1"})
	service, err = client.InspectService("node-container-bs-p1")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.Placement.Constraints, check.DeepEquals, []string{"node.labels.tsuru.pool == p1"})
}

func (s *S) TestUpgradeNodeContainerUpdatesExistingService(c *check.C) {
	s.addCluster(c)
	client, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err = nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	err = s.p.UpgradeNodeContainer("bs", "", ioutil.Discard)
	c.Assert(err, check.IsNil)
	c1.Config.Image = "bs:v2"
	err = nodecontainer.UpdateContainer("", &c1)
	c.Assert(err, check.IsNil)
	err = s.p.UpgradeNodeContainer("bs", "", ioutil.Discard)
	c.Assert(err, check.IsNil)
	service, err := client.InspectService("node-container-bs-all")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.ContainerSpec.Image, check.Equals, "bs:v2")
}

func (s *S) TestRemoveNodeContainerRemovesService(c *check.C) {
	s.addCluster(c)
	client, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err = nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	err = s.p.UpgradeNodeContainer("bs", "", ioutil.Discard)
	c.Assert(err, check.IsNil)
	services, err := client.ListServices(docker.ListServicesOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(services), check.Equals, 1)
	err = s.p.RemoveNodeContainer("bs", "", ioutil.Discard)
	c.Assert(err, check.IsNil)
	services, err = client.ListServices(docker.ListServicesOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(services), check.Equals, 0)
}

func (s *S) TestNodeForNodeData(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	cli, err := newClient(s.clusterSrv.URL(), nil)
	c.Assert(err, check.IsNil)
	conts, err := cli.ListContainers(docker.ListContainersOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 1)
	data := provision.NodeStatusData{
		Units: []provision.UnitStatusData{
			{ID: conts[0].ID},
		},
	}
	node, err := s.p.NodeForNodeData(data)
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, s.clusterSrv.URL())
	data = provision.NodeStatusData{
		Addrs: []string{s.clusterSrv.URL()},
	}
	node, err = s.p.NodeForNodeData(data)
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, s.clusterSrv.URL())
	data = provision.NodeStatusData{
		Units: []provision.UnitStatusData{
			{ID: "invalidid"},
		},
	}
	_, err = s.p.NodeForNodeData(data)
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestNodeForNodeDataNoCluster(c *check.C) {
	data := provision.NodeStatusData{
		Units: []provision.UnitStatusData{
			{ID: "invalidid"},
		},
	}
	_, err := s.p.NodeForNodeData(data)
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}

func (s *S) attachRegister(c *check.C, srv *testing.DockerServer, register bool, a provision.App) <-chan bool {
	return s.attachRegisterHook(c, srv, register, a, nil)
}

func (s *S) attachRegisterHook(c *check.C, srv *testing.DockerServer, register bool, a provision.App, hook func(r *http.Request)) <-chan bool {
	chAttached := make(chan bool, 1)
	srv.CustomHandler("/containers", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) == 4 && parts[3] == "attach" {
			if register {
				err := s.p.RegisterUnit(a, parts[2], map[string]interface{}{
					"processes": map[string]interface{}{
						"web": "python myapp.py",
					},
				})
				c.Assert(err, check.IsNil)
			}
			srv.MutateContainer(parts[2], docker.State{StartedAt: time.Now(), Running: false})
			chAttached <- true
		}
		srv.DefaultHandler().ServeHTTP(w, r)
		if hook != nil {
			hook(r)
		}
	}))
	return chAttached
}

func (s *S) TestSleepStart(c *check.C) {
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	err = s.p.Sleep(a, "")
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
	err = s.p.Start(a, "")
	c.Assert(err, check.IsNil)
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
}

func (s *S) TestCleanImage(c *check.C) {
	s.addCluster(c)
	var urls []string
	for i := 0; i < 5; i++ {
		srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
		c.Assert(err, check.IsNil)
		defer srv.Stop()
		urls = append(urls, srv.URL())
		metadata := map[string]string{"count": fmt.Sprintf("%d", i), "pool": "p1"}
		opts := provision.AddNodeOptions{
			Address:  srv.URL(),
			Metadata: metadata,
		}
		err = s.p.AddNode(opts)
		c.Assert(err, check.IsNil, check.Commentf("server %d", i))
		cli, err := docker.NewClient(srv.URL())
		c.Assert(err, check.IsNil)
		err = cli.PullImage(docker.PullImageOptions{
			Repository: "myimg",
			Tag:        "v1",
		}, docker.AuthConfiguration{})
		c.Assert(err, check.IsNil)
		err = cli.PullImage(docker.PullImageOptions{
			Repository: "myimg",
			Tag:        "v2",
		}, docker.AuthConfiguration{})
		c.Assert(err, check.IsNil)
		imgs, err := cli.ListImages(docker.ListImagesOptions{All: true})
		c.Assert(err, check.IsNil)
		c.Assert(imgs, check.HasLen, 2)
	}
	imageName := "myimg:v1"
	err := s.p.CleanImage("teste", imageName)
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(urls)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 5)
	for _, n := range nodes {
		cli, err := newClient(n.Address(), nil)
		c.Assert(err, check.IsNil)
		imgs, err := cli.ListImages(docker.ListImagesOptions{All: true})
		c.Assert(err, check.IsNil)
		c.Assert(imgs, check.HasLen, 1)
	}
}

func (s *S) TestDeleteVolume(c *check.C) {
	s.addCluster(c)
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	err = s.p.AddNode(provision.AddNodeOptions{Address: srv.URL()})
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	client2, err := docker.NewClient(srv.URL())
	c.Assert(err, check.IsNil)
	_, err = client.CreateVolume(docker.CreateVolumeOptions{Name: "myvol"})
	c.Assert(err, check.IsNil)
	_, err = client2.CreateVolume(docker.CreateVolumeOptions{Name: "myvol"})
	c.Assert(err, check.IsNil)
	vols, err := client.ListVolumes(docker.ListVolumesOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(vols), check.Equals, 1)
	err = s.p.DeleteVolume("myvol", "pool")
	c.Assert(err, check.IsNil)
	vols, err = client.ListVolumes(docker.ListVolumesOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(vols), check.Equals, 0)
	vols, err = client2.ListVolumes(docker.ListVolumesOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(vols), check.Equals, 0)
	err = s.p.DeleteVolume("myvol", "pool")
	c.Assert(err, check.IsNil)
}

func (s *S) TestIsVolumeProvisioned(c *check.C) {
	s.addCluster(c)
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	isProv, err := s.p.IsVolumeProvisioned("myvol", "pool")
	c.Assert(err, check.IsNil)
	c.Assert(isProv, check.Equals, false)
	err = s.p.AddNode(provision.AddNodeOptions{Address: srv.URL()})
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	client2, err := docker.NewClient(srv.URL())
	c.Assert(err, check.IsNil)
	_, err = client.CreateVolume(docker.CreateVolumeOptions{Name: "myvol"})
	c.Assert(err, check.IsNil)
	_, err = client2.CreateVolume(docker.CreateVolumeOptions{Name: "myvol"})
	c.Assert(err, check.IsNil)
	isProv, err = s.p.IsVolumeProvisioned("myvol", "pool")
	c.Assert(err, check.IsNil)
	c.Assert(isProv, check.Equals, true)
}

func (s *S) TestInitializeCluster(c *check.C) {
	clusterSrv, err := dockerTesting.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer clusterSrv.Stop()
	clust := &provTypes.Cluster{
		Addresses:   []string{clusterSrv.URL()},
		Default:     true,
		Name:        "c1",
		Provisioner: provisionerName,
	}
	err = s.p.InitializeCluster(clust)
	c.Assert(err, check.IsNil)
	cli, err := newClusterClient(clust)
	c.Assert(err, check.IsNil)
	nodes, err := cli.ListNodes(docker.ListNodesOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
}
