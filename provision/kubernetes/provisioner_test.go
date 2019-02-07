// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	"github.com/tsuru/tsuru/provision/kubernetes/testing"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/volume"
	check "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/remotecommand"
)

func (s *S) TestListNodes(c *check.C) {
	s.mock.MockfakeNodes(c)
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
	s.mock.MockfakeNodes(c)
	nodes, err := s.p.ListNodes([]string{"192.168.99.1"})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
}

func (s *S) TestListNodesTimeoutShort(c *check.C) {
	wantedTimeout := 1.0
	config.Set("kubernetes:api-short-timeout", wantedTimeout)
	defer config.Unset("kubernetes")
	block := make(chan bool)
	blackhole := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer func() { close(block); blackhole.Close() }()
	s.mock.MockfakeNodes(c, blackhole.URL)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	t0 := time.Now()
	ClientForConfig = defaultClientForConfig
	_, err = s.p.ListNodes([]string{})
	c.Assert(err, check.ErrorMatches, `(?is).*timeout.*`)
	c.Assert(time.Since(t0) < time.Duration(wantedTimeout*float64(3*time.Second)), check.Equals, true)
}

func (s *S) TestRemoveNode(c *check.C) {
	s.mock.MockfakeNodes(c)
	opts := provision.RemoveNodeOptions{
		Address: "192.168.99.1",
	}
	err := s.p.RemoveNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
}

func (s *S) TestRemoveNodeNotFound(c *check.C) {
	s.mock.MockfakeNodes(c)
	opts := provision.RemoveNodeOptions{
		Address: "192.168.99.99",
	}
	err := s.p.RemoveNode(opts)
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestRemoveNodeWithRebalance(c *check.C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.FormValue("labelSelector"), check.Equals, "tsuru.io/app-pool")
		output := `{"items": [
			{"metadata": {"name": "myapp-web-pod-1-1", "labels": {"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-platform": "python"}}, "status": {"phase": "Running"}},
			{"metadata": {"name": "myapp-worker-pod-2-1", "labels": {"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "worker", "tsuru.io/app-platform": "python"}}, "status": {"phase": "Running"}}
		]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(output))
	}))
	defer srv.Close()
	s.mock.MockfakeNodes(c, srv.URL)
	ns := s.client.PoolNamespace("")
	_, err := s.client.CoreV1().Pods(ns).Create(&apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: ns},
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

func (s *S) TestAddNode(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p1",
		Metadata: map[string]string{
			"m1": "v1",
		},
	})
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "my-node-addr")
	c.Assert(nodes[0].Pool(), check.Equals, "p1")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool": "p1",
		"tsuru.io/m1":   "v1",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Labels, check.DeepEquals, map[string]string{
		"tsuru.io/pool": "p1",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Annotations, check.DeepEquals, map[string]string{
		"tsuru.io/m1": "v1",
	})
}

func (s *S) TestAddNodePrefixed(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p1",
		Metadata: map[string]string{
			"tsuru.io/m1":                "v1",
			"m2":                         "v2",
			"pool":                       "p2", // ignored
			"tsuru.io/pool":              "p3", // ignored
			"tsuru.io/extra-labels":      "k1=v1,k2=v2",
			"tsuru.io/extra-annotations": "k3=v3,k4=v4",
		},
	})
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "my-node-addr")
	c.Assert(nodes[0].Pool(), check.Equals, "p1")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool":              "p1",
		"tsuru.io/m1":                "v1",
		"tsuru.io/m2":                "v2",
		"tsuru.io/extra-labels":      "k1=v1,k2=v2",
		"tsuru.io/extra-annotations": "k3=v3,k4=v4",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Labels, check.DeepEquals, map[string]string{
		"tsuru.io/pool": "p1",
		"k1":            "v1",
		"k2":            "v2",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Annotations, check.DeepEquals, map[string]string{
		"tsuru.io/m1":                "v1",
		"tsuru.io/m2":                "v2",
		"k3":                         "v3",
		"k4":                         "v4",
		"tsuru.io/extra-labels":      "k1=v1,k2=v2",
		"tsuru.io/extra-annotations": "k3=v3,k4=v4",
	})
}

func (s *S) TestAddNodeExisting(c *check.C) {
	s.mock.MockfakeNodes(c)
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "n1",
		Pool:    "Pxyz",
		Metadata: map[string]string{
			"m1": "v1",
		},
	})
	c.Assert(err, check.IsNil)
	n, err := s.p.GetNode("n1")
	c.Assert(err, check.IsNil)
	c.Assert(n.Address(), check.Equals, "192.168.99.1")
	c.Assert(n.Pool(), check.Equals, "Pxyz")
	c.Assert(n.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool": "Pxyz",
		"tsuru.io/m1":   "v1",
	})
}

func (s *S) TestAddNodeIaaSID(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p1",
		IaaSID:  "id-1",
		Metadata: map[string]string{
			"m1": "v1",
		},
	})
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "my-node-addr")
	c.Assert(nodes[0].Pool(), check.Equals, "p1")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool":    "p1",
		"tsuru.io/iaas-id": "id-1",
		"tsuru.io/m1":      "v1",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Labels, check.DeepEquals, map[string]string{
		"tsuru.io/pool":    "p1",
		"tsuru.io/iaas-id": "id-1",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Annotations, check.DeepEquals, map[string]string{
		"tsuru.io/m1": "v1",
	})
}

func (s *S) TestUpdateNode(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p1",
		Metadata: map[string]string{
			"m1": "v1",
		},
	})
	c.Assert(err, check.IsNil)
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: "my-node-addr",
		Pool:    "p2",
		Metadata: map[string]string{
			"m1": "",
			"m2": "v2",
		},
	})
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "my-node-addr")
	c.Assert(nodes[0].Pool(), check.Equals, "p2")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool": "p2",
		"tsuru.io/m2":   "v2",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Labels, check.DeepEquals, map[string]string{
		"tsuru.io/pool": "p2",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Annotations, check.DeepEquals, map[string]string{
		"tsuru.io/m2": "v2",
	})
}

func (s *S) TestUpdateNodeWithIaaSID(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p1",
		IaaSID:  "id-1",
		Metadata: map[string]string{
			"m1": "v1",
		},
	})
	c.Assert(err, check.IsNil)
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: "my-node-addr",
		Pool:    "p2",
		Metadata: map[string]string{
			"m1": "",
			"m2": "v2",
		},
	})
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].IaaSID(), check.Equals, "id-1")
	c.Assert(nodes[0].Address(), check.Equals, "my-node-addr")
	c.Assert(nodes[0].Pool(), check.Equals, "p2")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool":    "p2",
		"tsuru.io/iaas-id": "id-1",
		"tsuru.io/m2":      "v2",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Labels, check.DeepEquals, map[string]string{
		"tsuru.io/pool":    "p2",
		"tsuru.io/iaas-id": "id-1",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Annotations, check.DeepEquals, map[string]string{
		"tsuru.io/m2": "v2",
	})
	// Try updating by adding the same node with other IaasID, should be ignored.
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p2",
		IaaSID:  "bogus",
	})
	c.Assert(err, check.IsNil)
	nodes, err = s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].IaaSID(), check.Equals, "id-1")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool":    "p2",
		"tsuru.io/iaas-id": "id-1",
		"tsuru.io/m2":      "v2",
	})
}

func (s *S) TestUpdateNodeWithIaaSIDPreviousEmpty(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p1",
		Metadata: map[string]string{
			"m1": "v1",
		},
	})
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p2",
		IaaSID:  "valid-iaas-id",
	})
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].IaaSID(), check.Equals, "valid-iaas-id")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool":    "p2",
		"tsuru.io/iaas-id": "valid-iaas-id",
		"tsuru.io/m1":      "v1",
	})
}

func (s *S) TestUpdateNodeNoPool(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p1",
		Metadata: map[string]string{
			"m1": "v1",
		},
	})
	c.Assert(err, check.IsNil)
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: "my-node-addr",
		Metadata: map[string]string{
			"m1": "",
			"m2": "v2",
		},
	})
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "my-node-addr")
	c.Assert(nodes[0].Pool(), check.Equals, "p1")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool": "p1",
		"tsuru.io/m2":   "v2",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Labels, check.DeepEquals, map[string]string{
		"tsuru.io/pool": "p1",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Annotations, check.DeepEquals, map[string]string{
		"tsuru.io/m2": "v2",
	})
}

func (s *S) TestUpdateNodeRemoveInProgressTaint(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p1",
		Metadata: map[string]string{
			"m1": "v1",
		},
	})
	c.Assert(err, check.IsNil)
	n1, err := s.client.CoreV1().Nodes().Get("my-node-addr", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	n1.Spec.Taints = append(n1.Spec.Taints, apiv1.Taint{
		Key:    tsuruInProgressTaint,
		Value:  "true",
		Effect: apiv1.TaintEffectNoSchedule,
	})
	_, err = s.client.CoreV1().Nodes().Update(n1)
	c.Assert(err, check.IsNil)
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: "my-node-addr",
		Pool:    "p2",
		Metadata: map[string]string{
			"m1": "",
			"m2": "v2",
		},
	})
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "my-node-addr")
	c.Assert(nodes[0].Pool(), check.Equals, "p2")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool": "p2",
		"tsuru.io/m2":   "v2",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Labels, check.DeepEquals, map[string]string{
		"tsuru.io/pool": "p2",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Annotations, check.DeepEquals, map[string]string{
		"tsuru.io/m2": "v2",
	})
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Spec.Taints, check.DeepEquals, []apiv1.Taint{})
}

func (s *S) TestUpdateNodeToggleDisableTaint(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "my-node-addr",
		Pool:    "p1",
	})
	c.Assert(err, check.IsNil)
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: "my-node-addr",
		Disable: true,
	})
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "my-node-addr")
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Spec.Taints, check.DeepEquals, []apiv1.Taint{
		{Key: "tsuru.io/disabled", Effect: apiv1.TaintEffectNoSchedule},
	})
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: "my-node-addr",
		Disable: true,
	})
	c.Assert(err, check.IsNil)
	nodes, err = s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "my-node-addr")
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Spec.Taints, check.DeepEquals, []apiv1.Taint{
		{Key: "tsuru.io/disabled", Effect: apiv1.TaintEffectNoSchedule},
	})
	err = s.p.UpdateNode(provision.UpdateNodeOptions{
		Address: "my-node-addr",
		Enable:  true,
	})
	c.Assert(err, check.IsNil)
	nodes, err = s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "my-node-addr")
	c.Assert(nodes[0].(*kubernetesNodeWrapper).node.Spec.Taints, check.DeepEquals, []apiv1.Taint{})
}

func (s *S) TestUnits(c *check.C) {
	_, err := s.client.CoreV1().Pods("default").Create(&apiv1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "non-app-pod",
	}})
	c.Assert(err, check.IsNil)
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "myworker",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.Start(a, "")
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 2)
	sort.Slice(units, func(i, j int) bool {
		return units[i].ProcessName < units[j].ProcessName
	})
	for i, u := range units {
		splittedName := strings.Split(u.ID, "-")
		c.Assert(splittedName, check.HasLen, 5)
		c.Assert(splittedName[0], check.Equals, "myapp")
		units[i].ID = ""
		units[i].Name = ""
	}
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{
			AppName:     "myapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "192.168.99.1",
			Status:      "started",
			Address:     &url.URL{Scheme: "http", Host: "192.168.99.1:30000"},
		},
		{
			AppName:     "myapp",
			ProcessName: "worker",
			Type:        "python",
			IP:          "192.168.99.1",
			Status:      "started",
			Address:     &url.URL{Scheme: "http", Host: "192.168.99.1:30000"},
		},
	})
}

func (s *S) TestUnitsMultipleAppsNodes(c *check.C) {
	a1 := provisiontest.NewFakeAppWithPool("myapp", "python", "pool1", 0)
	a2 := provisiontest.NewFakeAppWithPool("otherapp", "python", "pool2", 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/pods" {
			s.mock.ListPodsHandler(c)(w, r)
			return
		}
	}))
	s.mock.MockfakeNodes(c, srv.URL)
	for _, a := range []provision.App{a1, a2} {
		for i := 1; i <= 2; i++ {
			_, err := s.client.CoreV1().Pods("default").Create(&apiv1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-%d", a.GetName(), i),
					Labels: map[string]string{
						"tsuru.io/app-name":     a.GetName(),
						"tsuru.io/app-process":  "web",
						"tsuru.io/app-platform": "python",
					},
					Namespace: "default",
				},
				Spec: apiv1.PodSpec{
					NodeName: fmt.Sprintf("n%d", i),
				},
			})
			c.Assert(err, check.IsNil)
		}
	}
	listNodesCalls := 0
	s.client.PrependReactor("list", "nodes", func(action ktesting.Action) (bool, runtime.Object, error) {
		listNodesCalls++
		return testing.RunReactionsAfter(&s.client.Fake, action)
	})
	units, err := s.p.Units(a1, a2)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 4)
	c.Assert(listNodesCalls, check.Equals, 1)
	sort.Slice(units, func(i, j int) bool {
		return units[i].ID < units[j].ID
	})
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{
			ID:          "myapp-1",
			Name:        "myapp-1",
			AppName:     "myapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "192.168.99.1",
			Status:      "",
			Address:     &url.URL{Scheme: "http", Host: "192.168.99.1"},
		},
		{
			ID:          "myapp-2",
			Name:        "myapp-2",
			AppName:     "myapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "192.168.99.2",
			Status:      "",
			Address:     &url.URL{Scheme: "http", Host: "192.168.99.2"},
		},
		{
			ID:          "otherapp-1",
			Name:        "otherapp-1",
			AppName:     "otherapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "192.168.99.1",
			Status:      "",
			Address:     &url.URL{Scheme: "http", Host: "192.168.99.1"},
		},
		{
			ID:          "otherapp-2",
			Name:        "otherapp-2",
			AppName:     "otherapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "192.168.99.2",
			Status:      "",
			Address:     &url.URL{Scheme: "http", Host: "192.168.99.2"},
		},
	})
}

func (s *S) TestUnitsSkipTerminating(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "myworker",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.Start(a, "")
	c.Assert(err, check.IsNil)
	wait()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	podlist, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(podlist.Items), check.Equals, 2)
	for _, p := range podlist.Items {
		if p.Labels["tsuru.io/app-process"] == "worker" {
			deadline := int64(10)
			p.Spec.ActiveDeadlineSeconds = &deadline
			_, err = s.client.CoreV1().Pods("default").Update(&p)
			c.Assert(err, check.IsNil)
		}
	}
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 1)
	c.Assert(units[0].ProcessName, check.DeepEquals, "web")
}

func (s *S) TestUnitsSkipEvicted(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "myworker",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.Start(a, "")
	c.Assert(err, check.IsNil)
	wait()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	podlist, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(podlist.Items), check.Equals, 2)
	for _, p := range podlist.Items {
		if p.Labels["tsuru.io/app-process"] == "worker" {
			p.Status.Phase = apiv1.PodFailed
			p.Status.Reason = "Evicted"
			_, err = s.client.CoreV1().Pods("default").Update(&p)
			c.Assert(err, check.IsNil)
		}
	}
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 1)
	c.Assert(units[0].ProcessName, check.DeepEquals, "web")
}

func (s *S) TestUnitsEmpty(c *check.C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.FormValue("labelSelector"), check.Equals, "tsuru.io/app-name in (myapp)")
		output := `{"items": []}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(output))
	}))
	defer srv.Close()
	s.mock.MockfakeNodes(c, srv.URL)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestUnitsNoApps(c *check.C) {
	s.mock.MockfakeNodes(c)
	units, err := s.p.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestGetNode(c *check.C) {
	s.mock.MockfakeNodes(c)
	host := "192.168.99.1"
	node, err := s.p.GetNode(host)
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, host)
	node, err = s.p.GetNode("n1")
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

func (s *S) TestRegisterUnit(c *check.C) {
	s.mock.DefaultHook = func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.FormValue("labelSelector"), check.Equals, "tsuru.io/app-name in (myapp)")
		output := `{"items": [
		{"metadata": {"name": "myapp-web-pod-1-1", "labels": {"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-platform": "python"}}, "status": {"phase": "Running"}}
	]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(output))
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	err = s.p.RegisterUnit(a, units[0].ID, nil)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
}

func (s *S) TestRegisterUnitDeployUnit(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := createDeployPod(context.Background(), createPodParams{
		client:            s.clusterClient,
		app:               a,
		sourceImage:       "myimg",
		destinationImages: []string{"destimg"},
	})
	c.Assert(err, check.IsNil)
	meta, err := image.GetImageMetaData("destimg")
	c.Assert(err, check.IsNil)
	c.Assert(meta, check.DeepEquals, image.ImageMetadata{
		Name:            "destimg",
		CustomData:      map[string]interface{}{},
		LegacyProcesses: map[string]string{},
		Processes: map[string][]string{
			// Processes from RegisterUnit call in suite_test.go as deploy pod
			// reaction.
			"web":    {"python myapp.py"},
			"worker": {"python myworker.py"},
		},
		ExposedPorts: []string{},
	})
}

func (s *S) TestAddUnits(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
}

func (s *S) TestAddUnitsNotProvisionedRecreateAppCRD(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Destroy(a)
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
	a.Deploys = 1
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestRemoveUnits(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	err = s.p.RemoveUnits(a, 2, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestRestart(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	id := units[0].ID
	err = s.p.Restart(a, "", nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units[0].ID, check.Not(check.Equals), id)
}

func (s *S) TestRestartNotProvisionedRecreateAppCRD(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Destroy(a)
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
	a.Deploys = 1
	err = s.p.Restart(a, "", nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestStopStart(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	err = s.p.Stop(a, "")
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
	err = s.p.Start(a, "")
	c.Assert(err, check.IsNil)
	wait()
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestProvisionerDestroy(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	wait()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	err = s.p.Destroy(a)
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1().Deployments(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	services, err := s.client.CoreV1().Services(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(services.Items, check.HasLen, 0)
	serviceAccounts, err := s.client.CoreV1().ServiceAccounts(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(serviceAccounts.Items, check.HasLen, 0)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 0)
}

func (s *S) TestProvisionerRoutableAddresses(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil)
	wait()
	addrs, err := s.p.RoutableAddresses(a)
	c.Assert(err, check.IsNil)
	sort.Slice(addrs, func(i, j int) bool {
		return addrs[i].Host < addrs[j].Host
	})
	c.Assert(addrs, check.DeepEquals, []url.URL{
		{
			Scheme: "http",
			Host:   "192.168.99.1:30000",
		},
		{
			Scheme: "http",
			Host:   "192.168.99.2:30000",
		},
	})
}

func (s *S) TestProvisionerRoutableAddressesRouterAddressLocal(c *check.C) {
	s.clusterClient.CustomData = map[string]string{
		routerAddressLocalKey: "true",
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil)
	wait()
	addrs, err := s.p.RoutableAddresses(a)
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []url.URL{
		{
			Scheme: "http",
			Host:   "192.168.99.1:30000",
		},
	})
}

func (s *S) TestDeploy(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	img, err := s.p.Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1().Deployments(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	c.Assert(deps.Items[0].Name, check.Equals, "myapp-web")
	containers := deps.Items[0].Spec.Template.Spec.Containers
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].Command[len(containers[0].Command)-3:], check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; curl -sSL -m15 -XPOST -d\"hostname=$(hostname)\" -o/dev/null -H\"Content-Type:application/x-www-form-urlencoded\" -H\"Authorization:bearer \" http://apps/myapp/units/register || true && exec run mycmd arg1",
	})
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(units, check.HasLen, 1)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(len(appList.Items), check.Equals, 1)
	c.Assert(appList.Items[0].Spec, check.DeepEquals, tsuruv1.AppSpec{
		NamespaceName:      "default",
		ServiceAccountName: "app-myapp",
		Deployments:        map[string][]string{"web": {"myapp-web"}},
		Services:           map[string][]string{"web": {"myapp-web", "myapp-web-units"}},
	})
}

func (s *S) TestDeployCreatesAppCR(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Destroy(a)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
}

func (s *S) TestDeployWithPoolNamespaces(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	var counter int32
	s.client.PrependReactor("create", "namespaces", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		new := atomic.AddInt32(&counter, 1)
		ns, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Namespace)
		c.Assert(ok, check.Equals, true)
		if new == 2 {
			c.Assert(ns.ObjectMeta.Name, check.Equals, "tsuru-test-default")
		} else {
			c.Assert(ns.ObjectMeta.Name, check.Equals, s.client.Namespace())
		}
		return testing.RunReactionsAfter(&s.client.Fake, action)
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	img, err := s.p.Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	c.Assert(atomic.LoadInt32(&counter), check.Equals, int32(3))
	appList, err := s.client.TsuruV1().Apps("tsuru").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(len(appList.Items), check.Equals, 1)
	c.Assert(appList.Items[0].Spec, check.DeepEquals, tsuruv1.AppSpec{
		NamespaceName:      "tsuru-test-default",
		ServiceAccountName: "app-myapp",
		Deployments:        map[string][]string{"web": {"myapp-web"}},
		Services:           map[string][]string{"web": {"myapp-web", "myapp-web-units"}},
	})
}

func (s *S) TestDeployWithCustomConfig(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
		"kubernetes": map[string]interface{}{
			"groups": map[string]interface{}{
				"pod1": map[string]interface{}{
					"web": map[string]interface{}{
						"ports": []interface{}{
							map[string]interface{}{
								"name": "my-port",
								"port": 9000,
							},
							map[string]interface{}{
								"protocol":    "tcp",
								"target_port": 8080,
							},
						},
					},
				},
				"pod2": map[string]interface{}{
					"proc2": map[string]interface{}{},
				},
				"pod3": map[string]interface{}{
					"proc3": map[string]interface{}{
						"ports": []interface{}{
							map[string]interface{}{
								"protocol":    "udp",
								"port":        9000,
								"target_port": 9001,
							},
						},
					},
				},
			},
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	img, err := s.p.Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1().Deployments(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	c.Assert(deps.Items[0].Name, check.Equals, "myapp-web")
	containers := deps.Items[0].Spec.Template.Spec.Containers
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].Command[len(containers[0].Command)-3:], check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; curl -sSL -m15 -XPOST -d\"hostname=$(hostname)\" -o/dev/null -H\"Content-Type:application/x-www-form-urlencoded\" -H\"Authorization:bearer \" http://apps/myapp/units/register || true && exec run mycmd arg1",
	})
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(units, check.HasLen, 1)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(len(appList.Items), check.Equals, 1)
	c.Assert(appList.Items[0].Spec, check.DeepEquals, tsuruv1.AppSpec{
		NamespaceName:      "default",
		ServiceAccountName: "app-myapp",
		Deployments:        map[string][]string{"web": {"myapp-web"}},
		Services:           map[string][]string{"web": {"myapp-web", "myapp-web-units"}},
		Configs: &provTypes.TsuruYamlKubernetesConfig{
			Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
				"pod1": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"web": {
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
							{Name: "my-port", Protocol: "TCP", Port: 9000, TargetPort: 9000},
							{Name: "http-default-2", Protocol: "TCP", Port: 8080, TargetPort: 8080},
						},
					},
				},
				"pod2": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"proc2": {
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{},
					},
				},
				"pod3": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"proc3": {
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
							{Name: "udp-default-1", Protocol: "UDP", Port: 9000, TargetPort: 9001},
						},
					},
				},
			},
		},
	})
}

func (s *S) TestDeployBuilderImageCancel(c *check.C) {
	srv, wg := s.mock.CreateDeployReadyServer(c)
	s.mock.MockfakeNodes(c, srv.URL)
	defer srv.Close()
	defer wg.Wait()
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	deploy := make(chan struct{})
	attach := make(chan struct{})
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		deploy <- struct{}{}
		<-attach
		return testing.RunReactionsAfter(&s.client.Fake, action)
	})
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(ok, check.Equals, true)
		pod.Status.Phase = apiv1.PodRunning
		testing.UpdatePodContainerStatus(pod, true)
		return testing.RunReactionsAfter(&s.client.Fake, action)
	})
	evt, err := event.New(&event.Opts{
		Target:        event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:          permission.PermAppDeploy,
		Owner:         s.token,
		Allowed:       event.Allowed(permission.PermAppDeploy),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents),
		Cancelable:    true,
	})
	c.Assert(err, check.IsNil)
	go func(evt event.Event) {
		img, errDeploy := s.p.Deploy(a, "tsuru/app-myapp:v1-builder", &evt)
		c.Check(errDeploy, check.ErrorMatches, `canceled after .*`)
		c.Check(img, check.Equals, "")
		deploy <- struct{}{}
	}(*evt)
	select {
	case <-deploy:
	case <-time.After(time.Second * 15):
		c.Fatal("timeout waiting for deploy to start")
	}
	err = evt.TryCancel("because I want.", "admin@admin.com")
	attach <- struct{}{}
	c.Assert(err, check.IsNil)
	select {
	case <-deploy:
	case <-time.After(time.Second * 15):
		c.Fatal("timeout waiting for cancelation")
	}
}

func (s *S) TestRollback(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	deployEvt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	img, err := s.p.Deploy(a, "tsuru/app-myapp:v1", deployEvt)
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	customData = map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg2",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v2", customData)
	c.Assert(err, check.IsNil)
	img, err = s.p.Deploy(a, "tsuru/app-myapp:v2", deployEvt)
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/app-myapp:v2")
	deployEvt.Done(err)
	c.Assert(err, check.IsNil)
	rollbackEvt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeployRollback,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeployRollback),
	})
	c.Assert(err, check.IsNil)
	img, err = s.p.Rollback(a, "tsuru/app-myapp:v1", rollbackEvt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1().Deployments(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	c.Assert(deps.Items[0].Name, check.Equals, "myapp-web")
	containers := deps.Items[0].Spec.Template.Spec.Containers
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].Command[len(containers[0].Command)-3:], check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; curl -sSL -m15 -XPOST -d\"hostname=$(hostname)\" -o/dev/null -H\"Content-Type:application/x-www-form-urlencoded\" -H\"Authorization:bearer \" http://apps/myapp/units/register || true && exec run mycmd arg1",
	})
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestDeployBuilderImageWithRegistryAuth(c *check.C) {
	config.Set("docker:registry", "registry.example.com")
	defer config.Unset("docker:registry")
	config.Set("docker:registry-auth:username", "user")
	defer config.Unset("docker:registry-auth:username")
	config.Set("docker:registry-auth:password", "pwd")
	defer config.Unset("docker:registry-auth:password")
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		containers := pod.Spec.Containers
		if containers[0].Name == "myapp-v1-deploy" {
			c.Assert(containers, check.HasLen, 2)
			sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
			cmds := cleanCmds(containers[0].Command[2])
			c.Assert(cmds, check.Equals, `end() { touch /tmp/intercontainer/done; }
trap end EXIT
mkdir -p $(dirname /dev/null) && cat >/dev/null && tsuru_unit_agent   myapp deploy-only`)
			c.Assert(containers[0].Env, check.DeepEquals, []apiv1.EnvVar{
				{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
				{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "registry.example.com/tsuru/app-myapp:v1,registry.example.com/tsuru/app-myapp:latest"},
				{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: "user"},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: "pwd"},
				{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: "registry.example.com"},
				{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/dev/null"},
				{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
				{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "false"},
			})
		}
		return testing.RunReactionsAfter(&s.client.Fake, action)
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	img, err := s.p.Deploy(a, "registry.example.com/tsuru/app-myapp:v1-builder", evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "registry.example.com/tsuru/app-myapp:v1")
}

func (s *S) TestUpgradeNodeContainer(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	s.mock.MockfakeNodes(c)
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
		HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			Binds:         []string{"/xyz:/abc:ro"},
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	c2 := c1
	c2.Config.Env = []string{"e1=v1"}
	err = nodecontainer.AddNewContainer("p1", &c2)
	c.Assert(err, check.IsNil)
	c3 := c1
	err = nodecontainer.AddNewContainer("p2", &c3)
	c.Assert(err, check.IsNil)
	buf := &bytes.Buffer{}
	err = s.p.UpgradeNodeContainer("bs", "", buf)
	c.Assert(err, check.IsNil)

	daemons, err := s.client.AppsV1().DaemonSets(s.client.PoolNamespace("")).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 1)
	daemons, err = s.client.AppsV1().DaemonSets(s.client.PoolNamespace("p1")).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 1)
	daemons, err = s.client.AppsV1().DaemonSets(s.client.PoolNamespace("p2")).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 1)
}

func (s *S) TestRemoveNodeContainer(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	s.mock.MockfakeNodes(c)
	ns := s.client.PoolNamespace("p1")
	_, err := s.client.AppsV1().DaemonSets(ns).Create(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-container-bs-pool-p1",
			Namespace: ns,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.CoreV1().Pods(ns).Create(&apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-container-bs-pool-p1-xyz",
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":            "true",
				"tsuru.io/is-node-container":   "true",
				"tsuru.io/provisioner":         provisionerName,
				"tsuru.io/node-container-name": "bs",
				"tsuru.io/node-container-pool": "p1",
			},
		},
	})
	c.Assert(err, check.IsNil)
	err = s.p.RemoveNodeContainer("bs", "p1", ioutil.Discard)
	c.Assert(err, check.IsNil)
	daemons, err := s.client.AppsV1().DaemonSets(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 0)
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
}

func (s *S) TestExecuteCommandWithStdin(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	s.mock.HandleSize = true
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdin:  conn,
		Stdout: conn,
		Stderr: conn,
		Width:  99,
		Height: 42,
		Term:   "xterm",
		Units:  []string{"myapp-web-pod-1-1"},
		Cmds:   []string{"mycmd", "arg1"},
	})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	rollback()
	c.Assert(s.mock.Stream["myapp-web"].Stdin, check.Equals, "echo test")
	var sz remotecommand.TerminalSize
	err = json.Unmarshal([]byte(s.mock.Stream["myapp-web"].Resize), &sz)
	c.Assert(err, check.IsNil)
	c.Assert(sz, check.DeepEquals, remotecommand.TerminalSize{Width: 99, Height: 42})
	c.Assert(s.mock.Stream["myapp-web"].Urls, check.HasLen, 1)
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec")
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Query()["command"], check.DeepEquals, []string{"/usr/bin/env", "TERM=xterm", "mycmd", "arg1"})
}

func (s *S) TestExecuteCommandWithStdinNoSize(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdin:  conn,
		Stdout: conn,
		Stderr: conn,
		Term:   "xterm",
		Units:  []string{"myapp-web-pod-1-1"},
		Cmds:   []string{"mycmd", "arg1"},
	})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	rollback()
	c.Assert(s.mock.Stream["myapp-web"].Stdin, check.Equals, "echo test")
	c.Assert(s.mock.Stream["myapp-web"].Urls, check.HasLen, 1)
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec")
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Query()["command"], check.DeepEquals, []string{"/usr/bin/env", "TERM=xterm", "mycmd", "arg1"})
}

func (s *S) TestExecuteCommandWithStdinNoUnits(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 2, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	s.mock.HandleSize = true
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdin:  conn,
		Stdout: conn,
		Stderr: conn,
		Width:  99,
		Height: 42,
		Term:   "xterm",
	})
	rollback()
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(s.mock.Stream["myapp-isolated-run"].Stdin, check.Equals, "echo test")
	var sz remotecommand.TerminalSize
	err = json.Unmarshal([]byte(s.mock.Stream["myapp-isolated-run"].Resize), &sz)
	c.Assert(err, check.IsNil)
	c.Assert(sz, check.DeepEquals, remotecommand.TerminalSize{Width: 99, Height: 42})
	c.Assert(s.mock.Stream["myapp-isolated-run"].Urls, check.HasLen, 1)
	c.Assert(s.mock.Stream["myapp-isolated-run"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-isolated-run/attach")
}

func (s *S) TestExecuteCommandUnitNotFound(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	buf := bytes.NewBuffer(nil)
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: buf,
		Width:  99,
		Height: 42,
		Units:  []string{"invalid-unit"},
	})
	c.Assert(err, check.DeepEquals, &provision.UnitNotFoundError{ID: "invalid-unit"})
}

func (s *S) TestExecuteCommand(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 2, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Units:  []string{"myapp-web-pod-1-1", "myapp-web-pod-2-2"},
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	rollback()
	c.Assert(stdout.String(), check.Equals, "stdout datastdout data")
	c.Assert(stderr.String(), check.Equals, "stderr datastderr data")
	c.Assert(s.mock.Stream["myapp-web"].Urls, check.HasLen, 2)
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec")
	c.Assert(s.mock.Stream["myapp-web"].Urls[1].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-web-pod-2-2/exec")
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Query()["command"], check.DeepEquals, []string{"mycmd", "arg1", "arg2"})
	c.Assert(s.mock.Stream["myapp-web"].Urls[1].Query()["command"], check.DeepEquals, []string{"mycmd", "arg1", "arg2"})
}

func (s *S) TestExecuteCommandSingleUnit(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 2, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Units:  []string{"myapp-web-pod-1-1"},
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	rollback()
	c.Assert(stdout.String(), check.Equals, "stdout data")
	c.Assert(stderr.String(), check.Equals, "stderr data")
	c.Assert(s.mock.Stream["myapp-web"].Urls, check.HasLen, 1)
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec")
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Query()["command"], check.DeepEquals, []string{"mycmd", "arg1", "arg2"})
}

func (s *S) TestExecuteCommandNoUnits(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(stdout.String(), check.Equals, "stdout data")
	c.Assert(stderr.String(), check.Equals, "stderr data")
	c.Assert(s.mock.Stream["myapp-isolated-run"].Urls, check.HasLen, 1)
	c.Assert(s.mock.Stream["myapp-isolated-run"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-isolated-run/attach")
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	account, err := s.client.CoreV1().ServiceAccounts(ns).Get("app-myapp", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(account, check.DeepEquals, &apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-myapp",
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/provisioner": "kubernetes",
			},
		},
	})
}

func (s *S) TestExecuteCommandNoUnitsPodFailed(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(ok, check.Equals, true)
		pod.Status.Phase = apiv1.PodFailed
		return testing.RunReactionsAfter(&s.client.Fake, action)
	})
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	c.Assert(err, check.ErrorMatches, `(?s)invalid pod phase "Failed".*`)
}

func (s *S) TestStartupMessage(c *check.C) {
	s.mock.MockfakeNodes(c)
	msg, err := s.p.StartupMessage()
	c.Assert(err, check.IsNil)
	c.Assert(msg, check.Equals, `Kubernetes provisioner on cluster "c1" - https://clusteraddr:
    Kubernetes node: 192.168.99.1
    Kubernetes node: 192.168.99.2
`)
	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return nil, nil
	}
	msg, err = s.p.StartupMessage()
	c.Assert(err, check.IsNil)
	c.Assert(msg, check.Equals, "")
}

func (s *S) TestSleepStart(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	imgName := "myapp:v1"
	err := image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	wait()
	err = s.p.Sleep(a, "")
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
	err = s.p.Start(a, "")
	c.Assert(err, check.IsNil)
	wait()
	units, err = s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestGetKubeConfig(c *check.C) {
	config.Set("kubernetes:deploy-sidecar-image", "img1")
	config.Set("kubernetes:deploy-inspect-image", "img2")
	config.Set("kubernetes:api-timeout", 10)
	config.Set("kubernetes:api-short-timeout", 0.5)
	config.Set("kubernetes:pod-ready-timeout", 6)
	config.Set("kubernetes:pod-running-timeout", 2*60)
	config.Set("kubernetes:deployment-progress-timeout", 3*60)
	config.Set("kubernetes:attach-after-finish-timeout", 5)
	config.Set("kubernetes:headless-service-port", 8889)
	defer config.Unset("kubernetes")
	kubeConf := getKubeConfig()
	c.Assert(kubeConf, check.DeepEquals, kubernetesConfig{
		DeploySidecarImage:                  "img1",
		DeployInspectImage:                  "img2",
		APITimeout:                          10 * time.Second,
		APIShortTimeout:                     500 * time.Millisecond,
		PodReadyTimeout:                     6 * time.Second,
		PodRunningTimeout:                   2 * time.Minute,
		DeploymentProgressTimeout:           3 * time.Minute,
		AttachTimeoutAfterContainerFinished: 5 * time.Second,
		HeadlessServicePort:                 8889,
	})
}

func (s *S) TestGetKubeConfigDefaults(c *check.C) {
	config.Unset("kubernetes")
	kubeConf := getKubeConfig()
	c.Assert(kubeConf, check.DeepEquals, kubernetesConfig{
		DeploySidecarImage:                  "tsuru/deploy-agent:0.8.1",
		DeployInspectImage:                  "tsuru/deploy-agent:0.8.1",
		APITimeout:                          60 * time.Second,
		APIShortTimeout:                     5 * time.Second,
		PodReadyTimeout:                     time.Minute,
		PodRunningTimeout:                   10 * time.Minute,
		DeploymentProgressTimeout:           10 * time.Minute,
		AttachTimeoutAfterContainerFinished: time.Minute,
		HeadlessServicePort:                 8888,
	})
}

func (s *S) TestProvisionerProvision(c *check.C) {
	_, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	crdList, err := s.client.ApiextensionsV1beta1().CustomResourceDefinitions().List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(crdList.Items[0].GetName(), check.DeepEquals, "apps.tsuru.io")
	appList, err := s.client.TsuruV1().Apps("tsuru").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 1)
	c.Assert(appList.Items[0].GetName(), check.DeepEquals, a.GetName())
	c.Assert(appList.Items[0].Spec.NamespaceName, check.DeepEquals, "default")
}

func (s *S) TestProvisionerUpdateApp(c *check.C) {
	err := pool.AddPool(pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	img, err := s.p.Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	sList, err := s.client.CoreV1().Services("tsuru-test-pool-2").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 2)
	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), "test-pool-2", 0)
	buf := new(bytes.Buffer)
	var recreatedPods bool
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(pod.Spec.NodeSelector, check.DeepEquals, map[string]string{
			"tsuru.io/pool": newApp.GetPool(),
		})
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-pool"], check.Equals, newApp.GetPool())
		recreatedPods = true
		return true, nil, nil
	})
	s.client.PrependReactor("create", "services", func(action ktesting.Action) (bool, runtime.Object, error) {
		srv := action.(ktesting.CreateAction).GetObject().(*apiv1.Service)
		srv.Spec.Ports = []apiv1.ServicePort{
			{
				NodePort: int32(30002),
			},
		}
		return testing.RunReactionsAfter(&s.client.Fake, action)
	})
	_, err = s.client.CoreV1().Nodes().Create(&apiv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-test-pool-2",
			Labels: map[string]string{"tsuru.io/pool": "test-pool-2"},
		},
		Status: apiv1.NodeStatus{
			Addresses: []apiv1.NodeAddress{{Type: apiv1.NodeInternalIP, Address: "192.168.100.1"}},
		},
	})
	c.Assert(err, check.IsNil)
	err = rebuild.Initialize(func(appName string) (rebuild.RebuildApp, error) {
		return &app.App{
			Name:    appName,
			Pool:    "test-pool-2",
			Routers: a.GetRouters(),
		}, nil
	})
	c.Assert(err, check.IsNil)
	defer rebuild.Shutdown(context.Background())
	err = s.p.UpdateApp(a, newApp, buf)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(buf.String(), "Done updating units"), check.Equals, true)
	c.Assert(recreatedPods, check.Equals, true)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 1)
	c.Assert(appList.Items[0].GetName(), check.DeepEquals, a.GetName())
	c.Assert(appList.Items[0].Spec.NamespaceName, check.DeepEquals, "tsuru-test-pool-2")
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-pool-2").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 2)
	raddrs, err := routertest.FakeRouter.Routes(a.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(raddrs, check.DeepEquals, []*url.URL{
		{
			Scheme: "http",
			Host:   "192.168.100.1:30002",
		},
	})
}

func (s *S) TestProvisionerUpdateAppWithVolumeSameClusterAndNamespace(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	err := pool.AddPool(pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	config.Set("kubernetes:use-pool-namespaces", false)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	img, err := s.p.Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	sList, err := s.client.CoreV1().Services("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 2)
	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), "test-pool-2", 0)
	buf := new(bytes.Buffer)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(pod.Spec.NodeSelector, check.DeepEquals, map[string]string{
			"tsuru.io/pool": newApp.GetPool(),
		})
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-pool"], check.Equals, newApp.GetPool())
		return true, nil, nil
	})
	v := volume.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan:      volume.VolumePlan{Name: "p1"},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = v.Create()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	err = s.p.UpdateApp(a, newApp, buf)
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerUpdateAppWithVolumeSameClusterOtherNamespace(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	err := pool.AddPool(pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	img, err := s.p.Deploy(a, "tsuru/app-myapp:v1", evt)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	sList, err := s.client.CoreV1().Services("tsuru-test-pool-2").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 2)
	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), "test-pool-2", 0)
	buf := new(bytes.Buffer)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(pod.Spec.NodeSelector, check.DeepEquals, map[string]string{
			"tsuru.io/pool": newApp.GetPool(),
		})
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-pool"], check.Equals, newApp.GetPool())
		return true, nil, nil
	})
	v := volume.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan:      volume.VolumePlan{Name: "p1"},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = v.Create()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	err = s.p.UpdateApp(a, newApp, buf)
	c.Assert(err, check.ErrorMatches, "can't change the pool of an app with binded volumes")
}

func (s *S) TestProvisionerUpdateAppWithVolumeOtherCluster(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	client1, client2 := s.prepareMultiCluster(c)
	s.client = client1
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	pool2 := client2.GetCluster().Pools[0]
	err := pool.AddPool(pool.AddPoolOptions{
		Name:        pool2,
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	s.client = client2
	s.mock = testing.NewKubeMock(s.client, s.p, s.factory)
	s.mock.IgnorePool = true
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	v := volume.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan:      volume.VolumePlan{Name: "p1"},
		Pool:      a.Pool,
		TeamOwner: "admin",
	}
	err = v.Create()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt1", false)
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt2", false)
	c.Assert(err, check.IsNil)
	_, _, err = createVolumesForApp(client1.ClusterInterface.(*ClusterClient), a)
	c.Assert(err, check.IsNil)

	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), pool2, 0)
	pvcs, err := client1.CoreV1().PersistentVolumeClaims("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 1)

	err = s.p.UpdateApp(a, newApp, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	// Check if old volume was removed
	pvcs, err = client1.CoreV1().PersistentVolumeClaims("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 0)
	// Check if new volume was created
	pvcs, err = client2.CoreV1().PersistentVolumeClaims("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 1)
}

func (s *S) TestProvisionerUpdateAppWithVolumeWithTwoBindsOtherCluster(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	client1, client2 := s.prepareMultiCluster(c)
	s.client = client1
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	pool2 := client2.GetCluster().Pools[0]
	err := pool.AddPool(pool.AddPoolOptions{
		Name:        pool2,
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	s.client = client2
	s.mock = testing.NewKubeMock(s.client, s.p, s.factory)
	s.mock.IgnorePool = true
	s.mock.IgnoreAppName = true
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	v := volume.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan:      volume.VolumePlan{Name: "p1"},
		Pool:      a.Pool,
		TeamOwner: "admin",
	}
	err = v.Create()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	_, _, err = createVolumesForApp(client1.ClusterInterface.(*ClusterClient), a)
	c.Assert(err, check.IsNil)
	a2 := provisiontest.NewFakeApp("myapp2", "python", 0)
	err = s.p.Provision(a2)
	c.Assert(err, check.IsNil)
	client1.TsuruClientset.PrependReactor("create", "apps", s.mock.AppReaction(a2, c))
	err = v.BindApp(a2.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	_, _, err = createVolumesForApp(client1.ClusterInterface.(*ClusterClient), a2)
	c.Assert(err, check.IsNil)

	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", customData)
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	pvcs, err := client1.CoreV1().PersistentVolumeClaims("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 1)

	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), pool2, 0)
	err = s.p.UpdateApp(a, newApp, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	// Check if old volume was not removed
	pvcs, err = client1.CoreV1().PersistentVolumeClaims("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 1)
	// Check if new volume was created
	pvcs, err = client2.CoreV1().PersistentVolumeClaims("default").List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 1)
}

func (s *S) TestProvisionerInitialize(c *check.C) {
	clusterControllers = map[string]*routerController{}
	_, ok := clusterControllers[s.clusterClient.Name]
	c.Assert(ok, check.Equals, false)
	err := s.p.Initialize()
	c.Assert(err, check.IsNil)
	_, ok = clusterControllers[s.clusterClient.Name]
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestProvisionerInitializeNoClusters(c *check.C) {
	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return nil, provTypes.ErrNoCluster
	}
	err := s.p.Initialize()
	c.Assert(err, check.IsNil)
}

func (s *S) TestEnvsForAppDefaultPort(c *check.C) {
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg:v2", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python proc1.py",
		},
	})
	c.Assert(err, check.IsNil)
	fa := provisiontest.NewFakeApp("myapp", "java", 1)
	fa.SetEnv(bind.EnvVar{Name: "e1", Value: "v1"})

	envs := EnvsForApp(fa, "web", "myimg:v2", false)
	c.Assert(envs, check.DeepEquals, []bind.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "web"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "port", Value: "8888"},
		{Name: "PORT", Value: "8888"},
		{Name: "PORT_web", Value: "8888"},
	})
}

func (s *S) TestEnvsForAppCustomPorts(c *check.C) {
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg:v2", map[string]interface{}{
		"processes": map[string]interface{}{
			"proc1": "python proc1.py",
			"proc2": "python proc2.py",
			"proc3": "python proc3.py",
			"proc4": "python proc4.py",
			"proc5": "python worker.py",
			"proc6": "python proc6.py",
		},
		"kubernetes": provTypes.TsuruYamlKubernetesConfig{
			Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
				"mypod1": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"proc1": {
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
							{TargetPort: 8080},
							{Port: 9000},
						},
					},
				},
				"mypod2": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"proc2": {
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
							{TargetPort: 8000},
						},
					},
				},
				"mypod3": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"proc3": {
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
							{Port: 8000, TargetPort: 8080},
						},
					},
				},
				"mypod5": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"proc5": {},
					"proc6": {
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
							{Port: 8000},
						},
					},
				},
				"mypod6": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"proc6": {},
				},
			},
		},
	})
	c.Assert(err, check.IsNil)
	fa := provisiontest.NewFakeApp("myapp", "java", 1)
	fa.SetEnv(bind.EnvVar{Name: "e1", Value: "v1"})

	envs := EnvsForApp(fa, "proc1", "myimg:v2", false)
	c.Assert(envs, check.DeepEquals, []bind.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc1"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "PORT_proc1", Value: "8080,9000"},
	})

	envs = EnvsForApp(fa, "proc2", "myimg:v2", false)
	c.Assert(envs, check.DeepEquals, []bind.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc2"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "PORT_proc2", Value: "8000"},
	})

	envs = EnvsForApp(fa, "proc3", "myimg:v2", false)
	c.Assert(envs, check.DeepEquals, []bind.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc3"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "PORT_proc3", Value: "8080"},
	})

	envs = EnvsForApp(fa, "proc4", "myimg:v2", false)
	c.Assert(envs, check.DeepEquals, []bind.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc4"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "port", Value: "8888"},
		{Name: "PORT", Value: "8888"},
		{Name: "PORT_proc4", Value: "8888"},
	})

	envs = EnvsForApp(fa, "proc5", "myimg:v2", false)
	c.Assert(envs, check.DeepEquals, []bind.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc5"},
		{Name: "TSURU_HOST", Value: ""},
	})

	envs = EnvsForApp(fa, "proc6", "myimg:v2", false)
	c.Assert(envs, check.DeepEquals, []bind.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc6"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "port", Value: "8888"},
		{Name: "PORT", Value: "8888"},
	})
}
