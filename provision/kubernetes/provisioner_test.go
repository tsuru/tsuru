// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"io/ioutil"
	"sort"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestListNodes(c *check.C) {
	s.mockfakeNodes(c)
	nodes, err := s.p.ListNodes([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address(), check.Equals, "192.168.99.1")
	c.Assert(nodes[1].Address(), check.Equals, "192.168.99.2")
}

func (s *S) TestListNodesWithoutNodes(c *check.C) {
	nodes, err := s.p.ListNodes([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
}

func (s *S) TestListNodesFilteringByAddress(c *check.C) {
	s.mockfakeNodes(c)
	nodes, err := s.p.ListNodes([]string{"192.168.99.1"})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
}

func (s *S) TestAddNodeCluster(c *check.C) {
	url := "https://clusteraddr"
	opts := provision.AddNodeOptions{
		Address: url,
		Metadata: map[string]string{
			"cluster": "true",
		},
	}
	err := s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	cli, err := getClusterClient()
	c.Assert(err, check.IsNil)
	c.Assert(cli, check.DeepEquals, s.client)
	c.Assert(s.lastConf.Host, check.Equals, url)
}

func (s *S) TestRemoveNode(c *check.C) {
	s.mockfakeNodes(c)
	opts := provision.RemoveNodeOptions{
		Address: "192.168.99.1",
	}
	err := s.p.RemoveNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
}

func (s *S) TestRemoveNodeWithRebalance(c *check.C) {
	s.mockfakeNodes(c)
	_, err := s.client.Core().Pods(tsuruNamespace).Create(&v1.Pod{
		ObjectMeta: v1.ObjectMeta{Name: "p1", Namespace: tsuruNamespace},
	})
	c.Assert(err, check.IsNil)
	evictionCalled := false
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		if action.GetSubresource() == "eviction" {
			evictionCalled = true
			return true, action.(ktesting.CreateAction).GetObject(), nil
		}
		return
	})
	opts := provision.RemoveNodeOptions{
		Address:   "192.168.99.1",
		Rebalance: true,
	}
	err = s.p.RemoveNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(evictionCalled, check.Equals, true)
}

func (s *S) TestUnits(c *check.C) {
	s.mockfakeNodes(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestGetNode(c *check.C) {
	s.mockfakeNodes(c)
	host := "192.168.99.1"
	node, err := s.p.GetNode(host)
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, host)
	node, err = s.p.GetNode("http://doesnotexist.com")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
	c.Assert(node, check.IsNil)
}

func (s *S) TestGetNodeWithoutCluster(c *check.C) {
	_, err := s.p.GetNode("anything")
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestUploadDeploy(c *check.C) {
	srv := s.createDeployReadyServer(c)
	defer srv.Close()
	s.mockfakeNodes(c, srv.URL)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	s.client.PrependReactor("create", "jobs", s.jobWithPodReaction(a, c))
	data := []byte("archivedata")
	archive := ioutil.NopCloser(bytes.NewReader(data))
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	img, err := s.p.UploadDeploy(a, archive, int64(len(data)), false, evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	deps, err := s.client.Extensions().Deployments(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 2)
	var depNames []string
	for _, dep := range deps.Items {
		depNames = append(depNames, dep.Name)
	}
	sort.Strings(depNames)
	c.Assert(depNames, check.DeepEquals, []string{"myapp-web", "myapp-worker"})
}
