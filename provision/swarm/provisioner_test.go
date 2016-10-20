// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
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
	// TODO(cezarsa): check TLSConfig loading
}

func (s *S) TestAddNode(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	metadata := map[string]string{"m1": "v1", "m2": "v2", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(srv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, srv.URL())
	c.Assert(node.Metadata(), check.DeepEquals, metadata)
	c.Assert(node.Pool(), check.Equals, "p1")
	c.Assert(node.Status(), check.Equals, "ready")
	coll, err := nodeAddrCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	var nodeAddrs NodeAddrs
	err = coll.FindId(uniqueDocumentID).One(&nodeAddrs)
	c.Assert(err, check.IsNil)
	c.Assert(nodeAddrs.Addresses, check.DeepEquals, []string{srv.URL()})
}

func (s *S) TestAddNodeMultiple(c *check.C) {
	for i := 0; i < 5; i++ {
		srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
		c.Assert(err, check.IsNil)
		metadata := map[string]string{"count": fmt.Sprintf("%d", i), "pool": "p1"}
		opts := provision.AddNodeOptions{
			Address:  srv.URL(),
			Metadata: metadata,
		}
		err = s.p.AddNode(opts)
		c.Assert(err, check.IsNil, check.Commentf("server %d", i))
	}
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 5)
	for i, n := range nodes {
		c.Assert(n.Metadata(), check.DeepEquals, map[string]string{
			"count": fmt.Sprintf("%d", i),
			"pool":  "p1",
		})
	}
}

func (s *S) TestListNodes(c *check.C) {
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
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, srv.URL())
	c.Assert(nodes[0].Metadata(), check.DeepEquals, metadata)
	c.Assert(nodes[0].Pool(), check.DeepEquals, "p1")
	c.Assert(nodes[0].Status(), check.DeepEquals, "ready")
}

func (s *S) TestListNodesEmpty(c *check.C) {
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
}

func (s *S) TestRestart(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
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
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
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
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	err = s.p.Restart(a, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	cli, err := docker.NewClient(srv.URL())
	c.Assert(err, check.IsNil)
	service, err := cli.InspectService("myapp-web")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.ContainerSpec.Labels[labelServiceRestart.String()], check.Equals, "1")
}

func (s *S) TestStopStart(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
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
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err = app.CreateApp(a, s.user)
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

func (s *S) TestAddUnits(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
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
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err = app.CreateApp(a, s.user)
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
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
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
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.ErrorMatches, `units can only be modified after the first deploy`)
}

func (s *S) TestAddUnitsNoProcessWithMultiple(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err = app.CreateApp(a, s.user)
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
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.ErrorMatches, `no process information found deploying image "tsuru/app-myapp"`)
}

func (s *S) TestAddUnitsZeroUnits(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
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
	err = s.p.AddUnits(a, 0, "web", nil)
	c.Assert(err, check.ErrorMatches, `cannot change 0 units`)
}

func (s *S) TestRemoveUnits(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
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
	node, err := s.p.GetNode(srv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, srv.URL())
	c.Assert(node.Metadata(), check.DeepEquals, metadata)
	c.Assert(node.Pool(), check.DeepEquals, "p1")
	c.Assert(node.Status(), check.DeepEquals, "ready")
}

func (s *S) TestGetNodeNotFound(c *check.C) {
	_, err := s.p.GetNode("http://tai.shar.malkier")
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestRemoveNode(c *check.C) {
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
	err = s.p.RemoveNode(provision.RemoveNodeOptions{
		Address: srv.URL(),
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.GetNode(srv.URL())
	c.Assert(errors.Cause(err), check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestRemoveNodeRebalance(c *check.C) {
	var reqs []*http.Request
	srv, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/nodes/") {
			reqs = append(reqs, r)
		}
	})
	c.Assert(err, check.IsNil)
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
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address:  srv.URL(),
		Metadata: map[string]string{"m1": "v2", "m2": "v3"},
	})
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(srv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"m1":   "v2",
		"m2":   "v3",
		"pool": "p1",
	})
}

func (s *S) TestUpdateNodeDisableEnable(c *check.C) {
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
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: srv.URL(),
		Disable: true,
	})
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(srv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"m1":   "v1",
		"pool": "p1",
	})
	c.Assert(node.Status(), check.Equals, "ready (pause)")
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: srv.URL(),
		Enable:  true,
	})
	c.Assert(err, check.IsNil)
	node, err = s.p.GetNode(srv.URL())
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
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	cli, err := chooseDBSwarmNode()
	c.Assert(err, check.IsNil)
	_, err = cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			Annotations: swarm.Annotations{
				Name: "myapp-web",
				Labels: map[string]string{
					labelServiceDeploy.String():     "true",
					labelServiceBuildImage.String(): "app:v1",
				},
			},
		},
	})
	c.Assert(err, check.IsNil)
	conts, err := cli.ListContainers(docker.ListContainersOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 1)
	err = s.p.RegisterUnit(provision.Unit{ID: conts[0].ID}, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	data, err := image.GetImageCustomData("app:v1")
	c.Assert(err, check.IsNil)
	c.Assert(data.Processes, check.DeepEquals, map[string]string{"web": "python myapp.py"})
}

func (s *S) TestRegisterUnitNotBuild(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	cli, err := chooseDBSwarmNode()
	c.Assert(err, check.IsNil)
	_, err = cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			Annotations: swarm.Annotations{
				Name: "myapp-web",
				Labels: map[string]string{
					labelServiceBuildImage.String(): "notset:v1",
				},
			},
		},
	})
	c.Assert(err, check.IsNil)
	conts, err := cli.ListContainers(docker.ListContainersOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 1)
	err = s.p.RegisterUnit(provision.Unit{ID: conts[0].ID}, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	data, err := image.GetImageCustomData("notset:v1")
	c.Assert(err, check.IsNil)
	c.Assert(data, check.DeepEquals, image.ImageMetadata{})
}

func (s *S) TestRegisterUnitNoImageLabel(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	cli, err := chooseDBSwarmNode()
	c.Assert(err, check.IsNil)
	_, err = cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			Annotations: swarm.Annotations{
				Name: "myapp-web",
				Labels: map[string]string{
					labelServiceDeploy.String(): "true",
				},
			},
		},
	})
	c.Assert(err, check.IsNil)
	conts, err := cli.ListContainers(docker.ListContainersOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(conts, check.HasLen, 1)
	err = s.p.RegisterUnit(provision.Unit{ID: conts[0].ID}, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.ErrorMatches, `invalid build image label for build service: .*`)
}

func (s *S) TestArchiveDeploy(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	attached := s.attachRegister(c, srv, true)
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	imgID, err := s.p.ArchiveDeploy(a, "http://server/myfile.tgz", evt)
	c.Assert(err, check.IsNil)
	c.Assert(<-attached, check.Equals, true)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v1")
	dbImg, err := image.AppCurrentImageName(a.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(dbImg, check.Equals, "tsuru/app-myapp:v1")
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{ID: units[0].ID, AppName: a.Name, Type: "whitespace", ProcessName: "web", Ip: "127.0.0.1", Status: "started", Address: &url.URL{Scheme: "http", Host: "127.0.0.1:0"}},
	})
	cli, err := docker.NewClient(srv.URL())
	c.Assert(err, check.IsNil)
	cont, err := cli.InspectContainer(units[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(cont.Config.Entrypoint, check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; exec python myapp.py",
	})
}

func (s *S) TestDeployServiceBind(c *check.C) {
	c.Skip("no support for service bind in the provisioner just yet")
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	attached := s.attachRegister(c, srv, true)
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	var serviceBodies []string
	rollback := s.addServiceInstance(c, a.Name, nil, func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadAll(r.Body)
		serviceBodies = append(serviceBodies, string(data))
		w.WriteHeader(http.StatusOK)
	})
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	imgID, err := s.p.ArchiveDeploy(a, "http://server/myfile.tgz", evt)
	c.Assert(err, check.IsNil)
	c.Assert(<-attached, check.Equals, true)
	c.Assert(imgID, check.Equals, "tsuru/app-myapp:v1")
	dbImg, err := image.AppCurrentImageName(a.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(dbImg, check.Equals, "tsuru/app-myapp:v1")
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{ID: units[0].ID, AppName: a.Name, Type: "whitespace", ProcessName: "web", Ip: "127.0.0.1", Status: "started", Address: &url.URL{Scheme: "http", Host: "127.0.0.1:0"}},
	})
	cli, err := docker.NewClient(srv.URL())
	c.Assert(err, check.IsNil)
	cont, err := cli.InspectContainer(units[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(cont.Config.Entrypoint, check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; exec python myapp.py",
	})
	c.Assert(serviceBodies, check.HasLen, 1)
	c.Assert(serviceBodies[0], check.Matches, ".*unit-host="+units[0].Ip)
}

func (s *S) TestImageDeploy(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	attached := s.attachRegister(c, srv, false)
	imageName := "myimg:v1"
	srv.CustomHandler(fmt.Sprintf("/images/%s/json", imageName), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := docker.Image{
			Config: &docker.Config{
				Entrypoint:   []string{"/bin/sh", "-c", "python test.py"},
				ExposedPorts: map[docker.Port]struct{}{"80/tcp": {}},
			},
		}
		j, _ := json.Marshal(response)
		w.Write(j)
	}))
	cli, err := docker.NewClient(srv.URL())
	c.Assert(err, check.IsNil)
	err = cli.PullImage(docker.PullImageOptions{
		Repository: "myimg",
		Tag:        "v1",
	}, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	deployedImg, err := s.p.ImageDeploy(a, imageName, evt)
	c.Assert(err, check.IsNil)
	c.Assert(<-attached, check.Equals, true)
	c.Assert(deployedImg, check.Equals, "tsuru/app-myapp:v1")
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{ID: units[0].ID, AppName: a.Name, ProcessName: "web", Ip: "127.0.0.1", Status: "started", Address: &url.URL{Scheme: "http", Host: "127.0.0.1:0"}},
	})
	dbImg, err := image.AppCurrentImageName(a.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(dbImg, check.Equals, "tsuru/app-myapp:v1")
	cont, err := cli.InspectContainer(units[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(cont.Config.Entrypoint, check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; exec /bin/sh \"-c\" \"python test.py\"",
	})
}

func (s *S) TestDestroy(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
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
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	err = s.p.Destroy(a)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) attachRegister(c *check.C, srv *testing.DockerServer, register bool) <-chan bool {
	chAttached := make(chan bool, 1)
	srv.CustomHandler("/containers", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) == 4 && parts[3] == "attach" {
			if register {
				err := s.p.RegisterUnit(provision.Unit{ID: parts[2]}, map[string]interface{}{
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
	}))
	return chAttached
}
