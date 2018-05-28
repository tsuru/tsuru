// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestNodeAddress(c *check.C) {
	var node kubernetesNodeWrapper
	c.Assert(node.Address(), check.Equals, "")
	node = kubernetesNodeWrapper{
		node: &apiv1.Node{
			Status: apiv1.NodeStatus{
				Addresses: []apiv1.NodeAddress{
					{
						Type:    apiv1.NodeInternalIP,
						Address: "192.168.99.100",
					},
					{
						Type:    apiv1.NodeExternalIP,
						Address: "200.0.0.1",
					},
				},
			},
		},
	}
	c.Assert(node.Address(), check.Equals, "192.168.99.100")
}

func (s *S) TestNodePool(c *check.C) {
	node := kubernetesNodeWrapper{
		node: &apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"tsuru.io/pool": "p1",
				},
			},
			Status: apiv1.NodeStatus{
				Addresses: []apiv1.NodeAddress{
					{
						Type:    apiv1.NodeInternalIP,
						Address: "192.168.99.100",
					},
				},
			},
		},
	}
	c.Assert(node.Pool(), check.Equals, "p1")
}

func (s *S) TestNodeStatus(c *check.C) {
	node := kubernetesNodeWrapper{
		node: &apiv1.Node{
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: apiv1.ConditionTrue,
					},
				},
			},
		},
	}
	c.Assert(node.Status(), check.Equals, "Ready")
	node = kubernetesNodeWrapper{
		node: &apiv1.Node{
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:    apiv1.NodeReady,
						Status:  apiv1.ConditionFalse,
						Message: "node pending",
					},
				},
			},
		},
	}
	c.Assert(node.Status(), check.Equals, "node pending")
	node = kubernetesNodeWrapper{
		node: &apiv1.Node{},
	}
	c.Assert(node.Status(), check.Equals, "Invalid")
	node = kubernetesNodeWrapper{
		node: &apiv1.Node{
			Spec: apiv1.NodeSpec{
				Taints: []apiv1.Taint{
					{Key: "tsuru.io/disabled", Effect: apiv1.TaintEffectNoSchedule},
				},
			},
		},
	}
	c.Assert(node.Status(), check.Equals, "Disabled")
}

func (s *S) TestNodeMetadata(c *check.C) {
	node := kubernetesNodeWrapper{
		node: &apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"tsuru.io/pool":    "p1",
					"tsuru.io/iaas-id": "i1",
					"m2.m3":            "v2",
				},
				Annotations: map[string]string{
					"a1":          "v3",
					"a2.a3":       "v4",
					"tsuru.io/a4": "v5",
				},
			},
		},
	}
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"tsuru.io/pool":    "p1",
		"tsuru.io/iaas-id": "i1",
		"tsuru.io/a4":      "v5",
	})
	c.Assert(node.MetadataNoPrefix(), check.DeepEquals, map[string]string{
		"pool":    "p1",
		"iaas-id": "i1",
		"a4":      "v5",
	})
	c.Assert(node.ExtraData(), check.DeepEquals, map[string]string{
		"m2.m3": "v2",
		"a1":    "v3",
		"a2.a3": "v4",
	})
	node.cluster = s.clusterClient
	node.cluster.Name = "fakecluster"
	c.Assert(node.ExtraData(), check.DeepEquals, map[string]string{
		"m2.m3":            "v2",
		"a1":               "v3",
		"a2.a3":            "v4",
		"tsuru.io/cluster": "fakecluster",
	})
}

func (s *S) TestNodeProvisioner(c *check.C) {
	node := kubernetesNodeWrapper{
		prov: s.p,
	}
	c.Assert(node.Provisioner(), check.Equals, s.p)
}

func (s *S) TestNodeUnits(c *check.C) {
	s.mock.DefaultHook = func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.FormValue("labelSelector"), check.Equals, "tsuru.io/is-service=true,tsuru.io/app-pool")
		output := `{"items": [
			{"metadata": {"name": "myapp-web-pod-1-1", "labels": {"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-platform": "python"}}, "status": {"phase": "Running"}},
			{"metadata": {"name": "myapp-worker-pod-2-1", "labels": {"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "worker", "tsuru.io/app-platform": "python"}}, "status": {"phase": "Running"}}
		]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(output))
	}
	fakeApp, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	routertest.FakeRouter.Reset()
	a := &app.App{Name: fakeApp.GetName(), TeamOwner: s.team.Name, Platform: fakeApp.GetPlatform()}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
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
	node, err := s.p.GetNode("192.168.99.1")
	c.Assert(err, check.IsNil)
	units, err := node.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{
			ID:          "myapp-web-pod-1-1",
			Name:        "myapp-web-pod-1-1",
			AppName:     "myapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "",
			Status:      "started",
			Address:     &url.URL{Scheme: "http", Host: ":30000"},
		},
		{
			ID:          "myapp-worker-pod-2-1",
			Name:        "myapp-worker-pod-2-1",
			AppName:     "myapp",
			ProcessName: "worker",
			Type:        "python",
			IP:          "",
			Status:      "started",
			Address:     &url.URL{Scheme: "http", Host: ""},
		},
	})
}

func (s *S) TestNodeUnitsUsingPoolNamespaces(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.FormValue("labelSelector"), check.Equals, "tsuru.io/is-service=true,tsuru.io/app-pool")
		output := `{"items": [
			{"metadata": {"name": "myapp-1", "labels": {"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-platform": "python"}}, "status": {"phase": "Running"}},
			{"metadata": {"name": "otherapp-1", "labels": {"tsuru.io/app-name": "otherapp", "tsuru.io/app-process": "web", "tsuru.io/app-platform": "python"}}, "status": {"phase": "Running"}}
		]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(output))
	}))
	defer srv.Close()
	s.mock.MockfakeNodes(c, srv.URL)
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fakeprov"
	provision.Register(p1.Name, func() (provision.Provisioner, error) {
		return p1, nil
	})
	defer provision.Unregister(p1.Name)
	err := pool.AddPool(pool.AddPoolOptions{Name: "pool1", Provisioner: p1.Name})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(pool.AddPoolOptions{Name: "pool2", Provisioner: p1.Name})
	c.Assert(err, check.IsNil)
	app1 := &app.App{Name: "myapp", TeamOwner: s.team.Name, Platform: "python", Pool: "pool1"}
	err = app.CreateApp(app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := &app.App{Name: "otherapp", TeamOwner: s.team.Name, Platform: "python", Pool: "pool2"}
	err = app.CreateApp(app2, s.user)
	c.Assert(err, check.IsNil)
	// TODO: add a second node after fixing kubernetes FakePods: https://github.com/kubernetes/kubernetes/blob/865321c2d69d249d95079b7f8e2ca99f5430d79e/staging/src/k8s.io/client-go/kubernetes/typed/core/v1/fake/fake_pod.go#L67
	numNodes := 1
	for i := 1; i <= numNodes; i++ {
		node, errGet := s.client.CoreV1().Nodes().Get(fmt.Sprintf("n%d", i), metav1.GetOptions{})
		c.Assert(errGet, check.IsNil)
		node.ObjectMeta.Labels["tsuru.io/pool"] = fmt.Sprintf("pool%d", i)
		_, errGet = s.client.CoreV1().Nodes().Update(node)
		c.Assert(errGet, check.IsNil)
	}
	for _, a := range []provision.App{app1, app2} {
		ns := s.client.AppNamespace(a)
		for i := 1; i <= numNodes; i++ {
			_, err = s.client.CoreV1().Pods(ns).Create(&apiv1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-%d", a.GetName(), i),
					Labels: map[string]string{
						"tsuru.io/app-name":     a.GetName(),
						"tsuru.io/app-process":  "web",
						"tsuru.io/app-platform": "python",
						"tsuru.io/is-service":   "true",
					},
				},
				Spec: apiv1.PodSpec{
					NodeName: fmt.Sprintf("n%d", i),
				},
			})
			c.Assert(err, check.IsNil)
		}
	}
	listPodsCalls := 0
	s.client.PrependReactor("list", "pods", func(ktesting.Action) (bool, runtime.Object, error) {
		listPodsCalls++
		return false, nil, nil
	})
	listNodesCalls := 0
	s.client.PrependReactor("list", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
		listNodesCalls++
		return false, nil, nil
	})

	node, err := s.p.GetNode("192.168.99.1")
	c.Assert(err, check.IsNil)
	units, err := node.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{
			ID:          "myapp-1",
			Name:        "myapp-1",
			AppName:     "myapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "",
			Status:      "started",
			Address:     &url.URL{Scheme: "http", Host: ""},
		},
		{
			ID:          "otherapp-1",
			Name:        "otherapp-1",
			AppName:     "otherapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "",
			Status:      "started",
			Address:     &url.URL{Scheme: "http", Host: ""},
		},
	})
}

func (s *S) TestNodeUnitsOnlyFromServices(c *check.C) {
	s.mock.DefaultHook = func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.FormValue("labelSelector"), check.Equals, "tsuru.io/is-service=true,tsuru.io/app-pool")
		output := `{"items": [
			{"metadata": {"name": "myapp-web-pod-1-1", "labels": {"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-platform": "python"}}, "status": {"phase": "Running"}},
			{"metadata": {"name": "myapp-worker-pod-2-1", "labels": {"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "worker", "tsuru.io/app-platform": "python"}}, "status": {"phase": "Running"}}
		]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(output))
	}
	ns := s.client.PoolNamespace("")
	_, err := s.client.CoreV1().Pods(ns).Create(&apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pod-not-tsuru",
			Namespace: ns,
		},
		Spec: apiv1.PodSpec{
			NodeName: "n1",
		},
	})
	c.Assert(err, check.IsNil)
	fakeApp, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.Reset()
	a := &app.App{Name: fakeApp.GetName(), TeamOwner: s.team.Name, Platform: fakeApp.GetPlatform()}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
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
	node, err := s.p.GetNode("192.168.99.1")
	c.Assert(err, check.IsNil)
	units, err := node.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{
			ID:          "myapp-web-pod-1-1",
			Name:        "myapp-web-pod-1-1",
			AppName:     "myapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "",
			Status:      "started",
			Address:     &url.URL{Scheme: "http", Host: ":30000"},
		},
		{
			ID:          "myapp-worker-pod-2-1",
			Name:        "myapp-worker-pod-2-1",
			AppName:     "myapp",
			ProcessName: "worker",
			Type:        "python",
			IP:          "",
			Status:      "started",
			Address:     &url.URL{Scheme: "http", Host: ""},
		},
	})
}
