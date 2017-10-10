// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"net/url"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

func (s *S) TestNodeAddress(c *check.C) {
	node := kubernetesNodeWrapper{
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
					"pool": "p1",
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
}

func (s *S) TestNodeMetadata(c *check.C) {
	node := kubernetesNodeWrapper{
		node: &apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"pool":  "p1",
					"m1":    "v1",
					"m2.m3": "v2",
				},
				Annotations: map[string]string{
					"a1":    "v3",
					"a2.a3": "v4",
				},
			},
		},
	}
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"pool": "p1",
		"m1":   "v1",
		"a1":   "v3",
	})
	c.Assert(node.ExtraData(), check.DeepEquals, map[string]string{
		"m2.m3": "v2",
		"a2.a3": "v4",
	})
	node.cluster = s.client.clusterClient
	node.cluster.Name = "fakecluster"
	c.Assert(node.ExtraData(), check.DeepEquals, map[string]string{
		"m2.m3":            "v2",
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
	fakeApp, wait, rollback := s.defaultReactions(c)
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
			IP:          "192.168.99.1",
			Status:      "started",
			Address:     &url.URL{Scheme: "http", Host: "192.168.99.1:30000"},
		},
		{
			ID:          "myapp-worker-pod-2-1",
			Name:        "myapp-worker-pod-2-1",
			AppName:     "myapp",
			ProcessName: "worker",
			Type:        "python",
			IP:          "192.168.99.1",
			Status:      "started",
			Address:     &url.URL{Scheme: "http", Host: "192.168.99.1"},
		},
	})
}
