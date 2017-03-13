// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"strconv"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"gopkg.in/check.v1"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/runtime"
	"k8s.io/client-go/pkg/util/intstr"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestServiceManagerDeployService(c *check.C) {
	m := serviceManager{client: s.client}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
			"p2": "cmd2",
		},
	})
	c.Assert(err, check.IsNil)
	err = m.DeployService(a, "p1", servicecommon.ProcessState{}, "myimg")
	c.Assert(err, check.IsNil)
	dep, err := s.client.Extensions().Deployments(tsuruNamespace).Get("myapp-p1")
	c.Assert(err, check.IsNil)
	one := int32(1)
	c.Assert(dep, check.DeepEquals, &extensions.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: tsuruNamespace,
		},
		Spec: extensions.DeploymentSpec{
			Replicas: &one,
			Template: v1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"tsuru.pod":                  strconv.FormatBool(true),
						"tsuru.pod.build":            strconv.FormatBool(false),
						"tsuru.app.name":             a.GetName(),
						"tsuru.app.process":          "p1",
						"tsuru.app.process.replicas": "1",
						"tsuru.app.platform":         a.GetPlatform(),
						"tsuru.node.pool":            a.GetPool(),
						"tsuru.router.name":          "fake",
						"tsuru.router.type":          "fake",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "myapp-p1",
							Image: "myimg",
							Command: []string{
								"/bin/sh",
								"-lc",
								"[ -d /home/application/current ] && cd /home/application/current; curl -fsSL -m15 -XPOST -d\"hostname=$(hostname)\" -o/dev/null -H\"Content-Type:application/x-www-form-urlencoded\" -H\"Authorization:bearer \" http://apps/myapp/units/register && exec cm1",
							},
							Env: []v1.EnvVar{
								{Name: "TSURU_HOST", Value: ""},
								{Name: "port", Value: "8888"},
								{Name: "PORT", Value: "8888"},
							},
						},
					},
				},
			},
		},
	})
	srv, err := s.client.Core().Services(tsuruNamespace).Get("myapp-p1")
	c.Assert(err, check.IsNil)
	c.Assert(srv, check.DeepEquals, &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: tsuruNamespace,
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.app.name":    "myapp",
				"tsuru.app.process": "p1",
			},
			Ports: []v1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(8888),
					TargetPort: intstr.FromInt(8888),
				},
			},
			Type: v1.ServiceTypeNodePort,
		},
	})
}

func (s *S) TestServiceManagerRemoveService(c *check.C) {
	m := serviceManager{client: s.client}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
			"p2": "cmd2",
		},
	})
	c.Assert(err, check.IsNil)
	err = m.DeployService(a, "p1", servicecommon.ProcessState{}, "myimg")
	c.Assert(err, check.IsNil)
	_, err = s.client.Extensions().ReplicaSets(tsuruNamespace).Create(&extensions.ReplicaSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: tsuruNamespace,
			Labels: map[string]string{
				"tsuru.app.name":    a.GetName(),
				"tsuru.app.process": "p1",
			},
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Core().Pods(tsuruNamespace).Create(&v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: tsuruNamespace,
			Labels: map[string]string{
				"tsuru.app.name":    a.GetName(),
				"tsuru.app.process": "p1",
			},
		},
	})
	c.Assert(err, check.IsNil)
	err = m.RemoveService(a, "p1")
	c.Assert(err, check.IsNil)
	deps, err := s.client.Extensions().Deployments(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	srvs, err := s.client.Core().Services(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srvs.Items, check.HasLen, 0)
	pods, err := s.client.Core().Pods(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.Extensions().ReplicaSets(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestServiceManagerRemoveServiceMiddleFailure(c *check.C) {
	m := serviceManager{client: s.client}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
			"p2": "cmd2",
		},
	})
	c.Assert(err, check.IsNil)
	err = m.DeployService(a, "p1", servicecommon.ProcessState{}, "myimg")
	c.Assert(err, check.IsNil)
	s.client.PrependReactor("delete", "deployments", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("my dep err")
	})
	err = m.RemoveService(a, "p1")
	c.Assert(err, check.ErrorMatches, "(?s).*my dep err.*")
	deps, err := s.client.Extensions().Deployments(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	srvs, err := s.client.Core().Services(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srvs.Items, check.HasLen, 0)
}
