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
	"strings"
	"time"

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

func (s *S) TestGetNode(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
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

func (s *S) TestRegisterUnit(c *check.C) {
	c.Fatal("aaaargh")
}

func (s *S) TestArchiveDeploy(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
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
