// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/safe"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/volume"
	check "gopkg.in/check.v1"
	"k8s.io/api/apps/v1beta2"
	apiv1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	ktesting "k8s.io/client-go/testing"
)

func cleanCmds(cmds string) string {
	result := []string{}
	trimpattern := regexp.MustCompile("^[\t ]*(.*?)[\t ]*$")
	for _, cmd := range strings.Split(cmds, "\n") {
		cleanCmd := trimpattern.ReplaceAllString(cmd, "$1")
		if len(cleanCmd) > 0 {
			result = append(result, cleanCmd)
		}
	}
	return strings.Join(result, "\n")
}

func (s *S) TestServiceManagerDeployService(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg:v2", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
			"p2": "cmd2",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg:v2", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	one := int32(1)
	ten := int32(10)
	maxSurge := intstr.FromString("100%")
	maxUnavailable := intstr.FromInt(0)
	expectedUID := int64(1000)
	depLabels := map[string]string{
		"tsuru.io/is-tsuru":             "true",
		"tsuru.io/is-service":           "true",
		"tsuru.io/is-build":             "false",
		"tsuru.io/is-stopped":           "false",
		"tsuru.io/is-deploy":            "false",
		"tsuru.io/is-isolated-run":      "false",
		"tsuru.io/app-name":             "myapp",
		"tsuru.io/app-process":          "p1",
		"tsuru.io/app-process-replicas": "1",
		"tsuru.io/app-platform":         "",
		"tsuru.io/app-pool":             "test-default",
		"tsuru.io/provisioner":          "kubernetes",
		"tsuru.io/builder":              "",
		"app":                           "myapp-p1",
		"version":                       "v2",
	}
	podLabels := make(map[string]string)
	for k, v := range depLabels {
		if k == "tsuru.io/app-process-replicas" {
			continue
		}
		podLabels[k] = v
	}
	annotations := map[string]string{
		"tsuru.io/router-type": "fake",
		"tsuru.io/router-name": "fake",
	}
	nsName, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	c.Assert(dep, check.DeepEquals, &v1beta2.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "myapp-p1",
			Namespace:   nsName,
			Labels:      depLabels,
			Annotations: annotations,
		},
		Status: v1beta2.DeploymentStatus{
			UpdatedReplicas: 1,
			Replicas:        1,
		},
		Spec: v1beta2.DeploymentSpec{
			Strategy: v1beta2.DeploymentStrategy{
				Type: v1beta2.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &v1beta2.RollingUpdateDeployment{
					MaxSurge:       &maxSurge,
					MaxUnavailable: &maxUnavailable,
				},
			},
			Replicas:             &one,
			RevisionHistoryLimit: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"tsuru.io/app-name":        "myapp",
					"tsuru.io/app-process":     "p1",
					"tsuru.io/is-build":        "false",
					"tsuru.io/is-isolated-run": "false",
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: annotations,
				},
				Spec: apiv1.PodSpec{
					ServiceAccountName: "app-myapp",
					SecurityContext: &apiv1.PodSecurityContext{
						RunAsUser: &expectedUID,
					},
					NodeSelector: map[string]string{
						"tsuru.io/pool": "test-default",
					},
					RestartPolicy: "Always",
					Subdomain:     "myapp-p1-units",
					Containers: []apiv1.Container{
						{
							Name:  "myapp-p1",
							Image: "myimg:v2",
							Command: []string{
								"/bin/sh",
								"-lc",
								"[ -d /home/application/current ] && cd /home/application/current; curl -sSL -m15 -XPOST -d\"hostname=$(hostname)\" -o/dev/null -H\"Content-Type:application/x-www-form-urlencoded\" -H\"Authorization:bearer \" http://apps/myapp/units/register || true && exec cm1",
							},
							Env: []apiv1.EnvVar{
								{Name: "TSURU_SERVICES", Value: "{}"},
								{Name: "TSURU_PROCESSNAME", Value: "p1"},
								{Name: "TSURU_HOST", Value: ""},
								{Name: "port", Value: "8888"},
								{Name: "PORT", Value: "8888"},
							},
							Resources: apiv1.ResourceRequirements{
								Limits:   apiv1.ResourceList{},
								Requests: apiv1.ResourceList{},
							},
							Ports: []apiv1.ContainerPort{
								{ContainerPort: 8888},
							},
						},
					},
				},
			},
		},
	})
	srv, err := s.client.CoreV1().Services(nsName).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srv, check.DeepEquals, &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: nsName,
			Labels: map[string]string{
				"app":                           "myapp-p1",
				"tsuru.io/is-tsuru":             "true",
				"tsuru.io/is-service":           "true",
				"tsuru.io/is-build":             "false",
				"tsuru.io/is-stopped":           "false",
				"tsuru.io/is-deploy":            "false",
				"tsuru.io/is-isolated-run":      "false",
				"tsuru.io/app-name":             "myapp",
				"tsuru.io/app-process":          "p1",
				"tsuru.io/app-process-replicas": "1",
				"tsuru.io/app-platform":         "",
				"tsuru.io/app-pool":             "test-default",
				"tsuru.io/provisioner":          "kubernetes",
				"tsuru.io/builder":              "",
			},
			Annotations: map[string]string{
				"tsuru.io/router-type": "fake",
				"tsuru.io/router-name": "fake",
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":        "myapp",
				"tsuru.io/app-process":     "p1",
				"tsuru.io/is-build":        "false",
				"tsuru.io/is-isolated-run": "false",
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(8888),
					TargetPort: intstr.FromInt(8888),
					Name:       "http-default",
				},
			},
			Type:                  apiv1.ServiceTypeNodePort,
			ExternalTrafficPolicy: apiv1.ServiceExternalTrafficPolicyTypeCluster,
		},
	})
	srvHeadless, err := s.client.CoreV1().Services(nsName).Get("myapp-p1-units", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srvHeadless, check.DeepEquals, &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-units",
			Namespace: nsName,
			Labels: map[string]string{
				"app":                           "myapp-p1",
				"tsuru.io/is-tsuru":             "true",
				"tsuru.io/is-service":           "true",
				"tsuru.io/is-build":             "false",
				"tsuru.io/is-stopped":           "false",
				"tsuru.io/is-deploy":            "false",
				"tsuru.io/is-isolated-run":      "false",
				"tsuru.io/is-headless-service":  "true",
				"tsuru.io/app-name":             "myapp",
				"tsuru.io/app-process":          "p1",
				"tsuru.io/app-process-replicas": "1",
				"tsuru.io/app-platform":         "",
				"tsuru.io/app-pool":             "test-default",
				"tsuru.io/provisioner":          "kubernetes",
				"tsuru.io/builder":              "",
			},
			Annotations: map[string]string{
				"tsuru.io/router-type": "fake",
				"tsuru.io/router-name": "fake",
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":        "myapp",
				"tsuru.io/app-process":     "p1",
				"tsuru.io/is-build":        "false",
				"tsuru.io/is-isolated-run": "false",
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(8888),
					TargetPort: intstr.FromInt(8888),
					Name:       "http-headless",
				},
			},
			ClusterIP: "None",
			Type:      apiv1.ServiceTypeClusterIP,
		},
	})
	account, err := s.client.CoreV1().ServiceAccounts(nsName).Get("app-myapp", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(account, check.DeepEquals, &apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-myapp",
			Namespace: nsName,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/provisioner": "kubernetes",
			},
		},
	})
}

func (s *S) TestServiceManagerDeployServiceWithPoolNamespaces(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	var counter int32
	s.client.PrependReactor("create", "namespaces", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		new := atomic.AddInt32(&counter, 1)
		ns, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Namespace)
		c.Assert(ok, check.Equals, true)
		if new == 2 {
			c.Assert(ns.ObjectMeta.Name, check.Equals, "tsuru-test-default")
		} else if new < 2 {
			c.Assert(ns.ObjectMeta.Name, check.Equals, s.client.Namespace())
		}
		return false, nil, nil
	})
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	processes := map[string]interface{}{
		"p1": "cmd1",
		"p2": "cmd2",
		"p3": "cmd3",
	}
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": processes,
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	c.Assert(atomic.LoadInt32(&counter), check.Equals, int32(len(processes)+1))
}

func (s *S) TestServiceManagerDeployServiceCustomPort(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgData := image.ImageMetadata{
		Name:        "myimg",
		ExposedPort: "7777/tcp",
		Processes:   map[string][]string{"p1": {"cmd1"}},
	}
	err = imgData.Save()
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	nsName, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	srv, err := s.client.CoreV1().Services(nsName).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srv, check.DeepEquals, &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: nsName,
			Labels: map[string]string{
				"app":                           "myapp-p1",
				"tsuru.io/is-tsuru":             "true",
				"tsuru.io/is-service":           "true",
				"tsuru.io/is-build":             "false",
				"tsuru.io/is-stopped":           "false",
				"tsuru.io/is-deploy":            "false",
				"tsuru.io/is-isolated-run":      "false",
				"tsuru.io/app-name":             "myapp",
				"tsuru.io/app-process":          "p1",
				"tsuru.io/app-process-replicas": "1",
				"tsuru.io/app-platform":         "",
				"tsuru.io/app-pool":             "test-default",
				"tsuru.io/provisioner":          "kubernetes",
				"tsuru.io/builder":              "",
			},
			Annotations: map[string]string{
				"tsuru.io/router-type": "fake",
				"tsuru.io/router-name": "fake",
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":        "myapp",
				"tsuru.io/app-process":     "p1",
				"tsuru.io/is-build":        "false",
				"tsuru.io/is-isolated-run": "false",
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(8888),
					TargetPort: intstr.FromInt(7777),
					Name:       "http-default",
				},
			},
			Type:                  apiv1.ServiceTypeNodePort,
			ExternalTrafficPolicy: apiv1.ServiceExternalTrafficPolicyTypeCluster,
		},
	})
}

func (s *S) TestServiceManagerDeployServiceCustomHeadlessPort(c *check.C) {
	config.Set("kubernetes:headless-service-port", 8889)
	defer config.Unset("kubernetes:headless-service-port")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgData := image.ImageMetadata{
		Name:      "myimg",
		Processes: map[string][]string{"p1": {"cmd1"}},
	}
	err = imgData.Save()
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	nsName, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	srv, err := s.client.CoreV1().Services(nsName).Get("myapp-p1-units", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srv, check.DeepEquals, &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-units",
			Namespace: nsName,
			Labels: map[string]string{
				"app":                           "myapp-p1",
				"tsuru.io/is-tsuru":             "true",
				"tsuru.io/is-service":           "true",
				"tsuru.io/is-build":             "false",
				"tsuru.io/is-stopped":           "false",
				"tsuru.io/is-deploy":            "false",
				"tsuru.io/is-isolated-run":      "false",
				"tsuru.io/is-headless-service":  "true",
				"tsuru.io/app-name":             "myapp",
				"tsuru.io/app-process":          "p1",
				"tsuru.io/app-process-replicas": "1",
				"tsuru.io/app-platform":         "",
				"tsuru.io/app-pool":             "test-default",
				"tsuru.io/provisioner":          "kubernetes",
				"tsuru.io/builder":              "",
			},
			Annotations: map[string]string{
				"tsuru.io/router-type": "fake",
				"tsuru.io/router-name": "fake",
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":        "myapp",
				"tsuru.io/app-process":     "p1",
				"tsuru.io/is-build":        "false",
				"tsuru.io/is-isolated-run": "false",
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(8889),
					TargetPort: intstr.FromInt(8888),
					Name:       "http-headless",
				},
			},
			ClusterIP: "None",
			Type:      apiv1.ServiceTypeClusterIP,
		},
	})
}

func (s *S) TestServiceManagerDeployServiceUpdateStates(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
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
	tests := []struct {
		states []servicecommon.ProcessState
		fn     func(dep *v1beta2.Deployment)
	}{
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 1},
			},
			fn: func(dep *v1beta2.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(2))
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(0))
				ls := labelSetFromMeta(&dep.ObjectMeta)
				c.Assert(ls.AppReplicas(), check.Equals, 3)
				c.Assert(ls.IsStopped(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				ls := labelSetFromMeta(&dep.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Start: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(3))
				ls := labelSetFromMeta(&dep.ObjectMeta)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {Start: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				ls := labelSetFromMeta(&dep.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Restart: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(3))
				ls := labelSetFromMeta(&dep.ObjectMeta)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {Restart: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				ls := labelSetFromMeta(&dep.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {},
			},
			fn: func(dep *v1beta2.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(0))
				ls := labelSetFromMeta(&dep.ObjectMeta)
				c.Assert(ls.AppReplicas(), check.Equals, 3)
				c.Assert(ls.IsStopped(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {},
			},
			fn: func(dep *v1beta2.Deployment) {
				ls := labelSetFromMeta(&dep.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Restart: true}, {Restart: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(1))
				ls := labelSetFromMeta(&dep.ObjectMeta)
				c.Assert(ls.Restarts(), check.Equals, 2)
			},
		},
	}
	for _, tt := range tests {
		for _, s := range tt.states {
			err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
				"p1": s,
			}, nil)
			c.Assert(err, check.IsNil)
			waitDep()
		}
		var dep *v1beta2.Deployment
		nsName, err := s.client.AppNamespace(a)
		c.Assert(err, check.IsNil)
		dep, err = s.client.Clientset.AppsV1beta2().Deployments(nsName).Get("myapp-p1", metav1.GetOptions{})
		c.Assert(err, check.IsNil)
		waitDep()
		tt.fn(dep)
		err = cleanupDeployment(s.clusterClient, a, "p1")
		c.Assert(err, check.IsNil)
		err = cleanupDeployment(s.clusterClient, a, "p2")
		c.Assert(err, check.IsNil)
	}
}

func (s *S) TestServiceManagerDeployServiceWithHC(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	tests := []struct {
		hc                provision.TsuruYamlHealthcheck
		expectedLiveness  *apiv1.Probe
		expectedReadiness *apiv1.Probe
		expectedLifecycle *apiv1.Lifecycle
	}{
		{},
		{
			hc: provision.TsuruYamlHealthcheck{
				Path:           "/hc",
				TimeoutSeconds: 10,
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    3,
				FailureThreshold: 0,
				TimeoutSeconds:   10,
				Handler: apiv1.Handler{
					Exec: &apiv1.ExecAction{
						Command: []string{
							"sh", "-c",
							"if [ ! -f /tmp/onetimeprobesuccessful ]; then curl -ksSf -XGET -o /dev/null http://localhost:8888/hc && touch /tmp/onetimeprobesuccessful; fi",
						},
					},
				},
			},
		},
		{
			hc: provision.TsuruYamlHealthcheck{
				Path:            "/hc",
				Scheme:          "https",
				AllowedFailures: 2,
				Method:          "POST",
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    3,
				FailureThreshold: 2,
				TimeoutSeconds:   60,
				Handler: apiv1.Handler{
					Exec: &apiv1.ExecAction{
						Command: []string{
							"sh", "-c",
							"if [ ! -f /tmp/onetimeprobesuccessful ]; then curl -ksSf -XPOST -o /dev/null https://localhost:8888/hc && touch /tmp/onetimeprobesuccessful; fi",
						},
					},
				},
			},
		},
		{
			hc: provision.TsuruYamlHealthcheck{
				Path:        "/hc",
				Scheme:      "https",
				UseInRouter: true,
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    10,
				FailureThreshold: 3,
				TimeoutSeconds:   60,
				Handler: apiv1.Handler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:   "/hc",
						Port:   intstr.FromInt(8888),
						Scheme: apiv1.URISchemeHTTPS,
					},
				},
			},
		},
		{
			hc: provision.TsuruYamlHealthcheck{
				Path:            "/hc",
				Scheme:          "https",
				UseInRouter:     true,
				IntervalSeconds: 9,
				TimeoutSeconds:  2,
				AllowedFailures: 4,
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    9,
				FailureThreshold: 4,
				TimeoutSeconds:   2,
				Handler: apiv1.Handler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:   "/hc",
						Port:   intstr.FromInt(8888),
						Scheme: apiv1.URISchemeHTTPS,
					},
				},
			},
		},
		{
			hc: provision.TsuruYamlHealthcheck{
				Path:            "/hc",
				Scheme:          "https",
				UseInRouter:     true,
				IntervalSeconds: 9,
				TimeoutSeconds:  2,
				AllowedFailures: 4,
				ForceRestart:    true,
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    9,
				FailureThreshold: 4,
				TimeoutSeconds:   2,
				Handler: apiv1.Handler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:   "/hc",
						Port:   intstr.FromInt(8888),
						Scheme: apiv1.URISchemeHTTPS,
					},
				},
			},
			expectedLiveness: &apiv1.Probe{
				PeriodSeconds:    9,
				FailureThreshold: 4,
				TimeoutSeconds:   2,
				Handler: apiv1.Handler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:   "/hc",
						Port:   intstr.FromInt(8888),
						Scheme: apiv1.URISchemeHTTPS,
					},
				},
			},
		},
	}
	for i, tt := range tests {
		img := fmt.Sprintf("myimg-%d", i)
		err = image.SaveImageCustomData(img, map[string]interface{}{
			"processes": map[string]interface{}{
				"web": "cm1",
				"p2":  "cmd2",
			},
			"healthcheck": tt.hc,
		})
		c.Assert(err, check.IsNil)
		err = servicecommon.RunServicePipeline(&m, a, img, servicecommon.ProcessSpec{
			"web": servicecommon.ProcessState{Start: true},
			"p2":  servicecommon.ProcessState{Start: true},
		}, nil)
		c.Assert(err, check.IsNil)
		waitDep()
		nsName, err := s.client.AppNamespace(a)
		c.Assert(err, check.IsNil)
		dep, err := s.client.Clientset.AppsV1beta2().Deployments(nsName).Get("myapp-web", metav1.GetOptions{})
		c.Assert(err, check.IsNil)
		c.Assert(dep.Spec.Template.Spec.Containers[0].ReadinessProbe, check.DeepEquals, tt.expectedReadiness)
		c.Assert(dep.Spec.Template.Spec.Containers[0].LivenessProbe, check.DeepEquals, tt.expectedLiveness)
		c.Assert(dep.Spec.Template.Spec.Containers[0].Lifecycle, check.DeepEquals, tt.expectedLifecycle)
		dep, err = s.client.Clientset.AppsV1beta2().Deployments(nsName).Get("myapp-p2", metav1.GetOptions{})
		c.Assert(err, check.IsNil)
		c.Assert(dep.Spec.Template.Spec.Containers[0].ReadinessProbe, check.IsNil)
		c.Assert(dep.Spec.Template.Spec.Containers[0].LivenessProbe, check.IsNil)
		c.Assert(dep.Spec.Template.Spec.Containers[0].Lifecycle, check.IsNil)
	}
}

func (s *S) TestServiceManagerDeployServiceWithRestartHooks(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "proc1",
			"p2":  "proc2",
		},
		"hooks": provision.TsuruYamlHooks{
			Restart: provision.TsuruYamlRestartHooks{
				Before: []string{"before cmd1", "before cmd2"},
				After:  []string{"after cmd1", "after cmd2"},
			},
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
		"p2":  servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedLifecycle := &apiv1.Lifecycle{
		PostStart: &apiv1.Handler{
			Exec: &apiv1.ExecAction{
				Command: []string{"sh", "-c", "after cmd1 && after cmd2"},
			},
		},
	}
	c.Assert(dep.Spec.Template.Spec.Containers[0].Lifecycle, check.DeepEquals, expectedLifecycle)
	cmd := dep.Spec.Template.Spec.Containers[0].Command
	c.Assert(cmd, check.HasLen, 3)
	c.Assert(cmd[2], check.Matches, `.*before cmd1 && before cmd2 && exec proc1$`)
	dep, err = s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-p2", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep.Spec.Template.Spec.Containers[0].Lifecycle, check.DeepEquals, expectedLifecycle)
	cmd = dep.Spec.Template.Spec.Containers[0].Command
	c.Assert(cmd, check.HasLen, 3)
	c.Assert(cmd[2], check.Matches, `.*before cmd1 && before cmd2 && exec proc2$`)
}

func (s *S) TestServiceManagerDeployServiceWithRegistryAuth(c *check.C) {
	config.Set("docker:registry", "myreg.com")
	config.Set("docker:registry-auth:username", "user")
	config.Set("docker:registry-auth:password", "pass")
	defer config.Unset("docker:registry")
	defer config.Unset("docker:registry-auth")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myreg.com/myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "cmd1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myreg.com/myimg", servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep.Spec.Template.Spec.ImagePullSecrets, check.DeepEquals, []apiv1.LocalObjectReference{
		{Name: "registry-myreg.com"},
	})
	secrets, err := s.client.CoreV1().Secrets(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(secrets.Items, check.DeepEquals, []apiv1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "registry-myreg.com",
				Namespace: "default",
			},
			Data: map[string][]byte{
				".dockerconfigjson": []byte(`{"auths":{"myreg.com":{"username":"user","password":"pass","auth":"dXNlcjpwYXNz"}}}`),
			},
			Type: "kubernetes.io/dockerconfigjson",
		},
	})
}

func (s *S) TestServiceManagerDeployServiceProgressMessages(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	fakeWatcher := watch.NewFakeWithChanSize(2, false)
	fakeWatcher.Add(&apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: "myapp-web.1",
		},
		InvolvedObject: apiv1.ObjectReference{
			Name: "pod-name-1",
		},
		Source: apiv1.EventSource{
			Component: "c1",
		},
		Message: "msg1",
	})
	fakeWatcher.Add(&apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: "myapp-web.2",
		},
		InvolvedObject: apiv1.ObjectReference{
			Name: "pod-name-1",
		},
		Source: apiv1.EventSource{
			Component: "c1",
			Host:      "n1",
		},
		Message: "msg2",
	})
	watchCalled := make(chan struct{})
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		dep := obj.(*v1beta2.Deployment)
		dep.Status.UnavailableReplicas = 1
		depCopy := *dep
		go func() {
			<-watchCalled
			time.Sleep(time.Second)
			depCopy.Status.UnavailableReplicas = 0
			s.client.AppsV1beta2().Deployments(ns).Update(&depCopy)
		}()
		return false, nil, nil
	})
	s.client.PrependWatchReactor("events", func(action ktesting.Action) (handled bool, ret watch.Interface, err error) {
		close(watchCalled)
		return true, fakeWatcher, nil
	})
	buf := bytes.NewBuffer(nil)
	m := serviceManager{client: s.clusterClient, writer: buf}
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "cmd1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	_, err = s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Matches, `(?s).* ---> 1 of 1 new units created.*? ---> 0 of 1 new units ready.*? ---> 1 of 1 new units ready.*? ---> Done updating units.*`)
	c.Assert(buf.String(), check.Matches, `(?s).*  ---> pod-name-1 - msg1 \[c1\].*?  ---> pod-name-1 - msg2 \[c1, n1\].*`)
}

func (s *S) TestServiceManagerDeployServiceFirstDeployDeleteDeploymentOnRollback(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	buf := bytes.NewBuffer(nil)
	m := serviceManager{client: s.clusterClient, writer: buf}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:        event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:          permission.PermAppDeploy,
		Owner:         s.token,
		Allowed:       event.Allowed(permission.PermAppDeploy),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents),
		Cancelable:    true,
	})
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "cmd1",
		},
	})
	c.Assert(err, check.IsNil)
	var deleteCalled bool
	s.client.PrependReactor("delete", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		name := action.(ktesting.DeleteAction).GetName()
		c.Assert(name, check.DeepEquals, "myapp-web")
		deleteCalled = true
		return false, nil, nil
	})
	deployCreated := make(chan struct{})
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if dep, ok := obj.(*v1beta2.Deployment); ok {
			rev, _ := strconv.Atoi(dep.Annotations[replicaDepRevision])
			rev++
			dep.Annotations[replicaDepRevision] = strconv.Itoa(rev)
			dep.Status.UnavailableReplicas = 1
			deployCreated <- struct{}{}
		}
		return false, nil, nil
	})
	go func(evt event.Event) {
		<-deployCreated
		errCancel := evt.TryCancel("Because i want.", "admin@admin.com")
		c.Assert(errCancel, check.IsNil)
	}(*evt)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	}, evt)
	c.Assert(err, check.DeepEquals, provision.ErrUnitStartup{Err: context.Canceled})
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	_, err = s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-web", metav1.GetOptions{})
	c.Assert(k8sErrors.IsNotFound(err), check.DeepEquals, true)
	c.Assert(buf.String(), check.Matches, `(?s).* ---> 1 of 1 new units created.*? ---> 0 of 1 new units ready.*? DELETING CREATED DEPLOYMENT AFTER FAILURE .*? ---> context canceled <---.*`)
	c.Assert(deleteCalled, check.DeepEquals, true)
}

func (s *S) TestServiceManagerDeployServiceCancelRollback(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	buf := bytes.NewBuffer(nil)
	m := serviceManager{client: s.clusterClient, writer: buf}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:        event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:          permission.PermAppDeploy,
		Owner:         s.token,
		Allowed:       event.Allowed(permission.PermAppDeploy),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents),
		Cancelable:    true,
	})
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "cmd1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	}, evt)
	c.Assert(err, check.IsNil)
	waitDep()
	deployCreated := make(chan struct{})
	s.client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if dep, ok := obj.(*v1beta2.Deployment); ok {
			rev, _ := strconv.Atoi(dep.Annotations[replicaDepRevision])
			rev++
			dep.Annotations[replicaDepRevision] = strconv.Itoa(rev)
			dep.Status.UnavailableReplicas = 1
			deployCreated <- struct{}{}
		}
		return false, nil, nil
	})
	go func(evt event.Event) {
		<-deployCreated
		errCancel := evt.TryCancel("Because i want.", "admin@admin.com")
		c.Assert(errCancel, check.IsNil)
	}(*evt)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	}, evt)
	c.Assert(err, check.DeepEquals, provision.ErrUnitStartup{Err: context.Canceled})
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	_, err = s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Matches, `(?s).* ---> 1 of 1 new units created.*? ---> 0 of 1 new units ready.*? ROLLING BACK AFTER FAILURE .*? ---> context canceled <---.*`)
}

func (s *S) TestServiceManagerDeployServiceWithNodeContainers(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep, check.NotNil)
	daemon, err := s.client.Clientset.AppsV1beta2().DaemonSets(ns).Get("node-container-bs-all", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemon, check.NotNil)
}

func (s *S) TestServiceManagerDeployServiceWithHCInvalidMethod(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "cm1",
			"p2":  "cmd2",
		},
		"healthcheck": provision.TsuruYamlHealthcheck{
			Path:        "/hc",
			Method:      "POST",
			UseInRouter: true,
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.ErrorMatches, "healthcheck: only GET method is supported in kubernetes provisioner with use_in_router set")
}

func (s *S) TestServiceManagerDeployServiceWithUID(c *check.C) {
	config.Set("docker:uid", 1001)
	defer config.Unset("docker:uid")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedUID := int64(1001)
	c.Assert(dep.Spec.Template.Spec.SecurityContext, check.DeepEquals, &apiv1.PodSecurityContext{
		RunAsUser: &expectedUID,
	})
}

func (s *S) TestServiceManagerDeployServiceWithResourceRequirements(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	a.Plan = appTypes.Plan{Memory: 1024}
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedMemory := resource.NewQuantity(1024, resource.BinarySI)
	c.Assert(dep.Spec.Template.Spec.Containers[0].Resources, check.DeepEquals, apiv1.ResourceRequirements{
		Limits: apiv1.ResourceList{
			apiv1.ResourceMemory: *expectedMemory,
		},
		Requests: apiv1.ResourceList{
			apiv1.ResourceMemory: *expectedMemory,
		},
	})
}

func (s *S) TestServiceManagerDeployServiceWithClusterWideOvercommitFactor(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	s.clusterClient.CustomData[overcommitClusterKey] = "3"
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	a.Plan = appTypes.Plan{Memory: 1024}
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedMemory := resource.NewQuantity(1024, resource.BinarySI)
	expectedMemoryRequest := resource.NewQuantity(341, resource.BinarySI)
	c.Assert(dep.Spec.Template.Spec.Containers[0].Resources, check.DeepEquals, apiv1.ResourceRequirements{
		Limits: apiv1.ResourceList{
			apiv1.ResourceMemory: *expectedMemory,
		},
		Requests: apiv1.ResourceList{
			apiv1.ResourceMemory: *expectedMemoryRequest,
		},
	})
}

func (s *S) TestServiceManagerDeployServiceWithClusterPoolOvercommitFactor(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	s.clusterClient.CustomData[overcommitClusterKey] = "3"
	s.clusterClient.CustomData["test-default:"+overcommitClusterKey] = "2"
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	a.Plan = appTypes.Plan{Memory: 1024}
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedMemory := resource.NewQuantity(1024, resource.BinarySI)
	expectedMemoryRequest := resource.NewQuantity(512, resource.BinarySI)
	c.Assert(dep.Spec.Template.Spec.Containers[0].Resources, check.DeepEquals, apiv1.ResourceRequirements{
		Limits: apiv1.ResourceList{
			apiv1.ResourceMemory: *expectedMemory,
		},
		Requests: apiv1.ResourceList{
			apiv1.ResourceMemory: *expectedMemoryRequest,
		},
	})
}

func (s *S) TestCreateBuildPodContainers(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	err = createBuildPod(context.Background(), createPodParams{
		client:            s.clusterClient,
		app:               a,
		sourceImage:       "myimg",
		destinationImages: []string{"destimg"},
		inputFile:         "/home/application/archive.tar.gz",
	})
	c.Assert(err, check.IsNil)
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	pods, err := s.client.Core().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	containers := pods.Items[0].Spec.Containers
	c.Assert(containers, check.HasLen, 2)
	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
	runAsUser := int64(1000)
	c.Assert(containers[0], check.DeepEquals, apiv1.Container{
		Name:  "committer-cont",
		Image: "tsuru/deploy-agent:0.6.0",
		VolumeMounts: []apiv1.VolumeMount{
			{Name: "dockersock", MountPath: dockerSockPath},
			{Name: "intercontainer", MountPath: buildIntercontainerPath},
		},
		Stdin:     true,
		StdinOnce: true,
		Command: []string{
			"sh", "-ec", `
				end() { touch /tmp/intercontainer/done; }
				trap end EXIT
				mkdir -p $(dirname /home/application/archive.tar.gz) && cat >/home/application/archive.tar.gz && tsuru_unit_agent   myapp "/var/lib/tsuru/deploy archive file:///home/application/archive.tar.gz" build
			`,
		},
		Env: []apiv1.EnvVar{
			{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
			{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "destimg"},
			{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
			{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
			{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/home/application/archive.tar.gz"},
			{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
			{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "false"},
		},
	})
	c.Assert(containers[1], check.DeepEquals, apiv1.Container{
		Name:    "myapp-v1-build",
		Image:   "myimg",
		Command: []string{"/bin/sh", "-ec", `while [ ! -f /tmp/intercontainer/done ]; do sleep 5; done`},
		Env:     []apiv1.EnvVar{{Name: "TSURU_HOST", Value: ""}},
		SecurityContext: &apiv1.SecurityContext{
			RunAsUser: &runAsUser,
		},
		VolumeMounts: []apiv1.VolumeMount{
			{Name: "intercontainer", MountPath: buildIntercontainerPath},
		},
	})
}

func (s *S) TestCreateDeployPodContainers(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	err = createDeployPod(context.Background(), createPodParams{
		client:            s.clusterClient,
		app:               a,
		sourceImage:       "myimg",
		destinationImages: []string{"destimg"},
		inputFile:         "/dev/null",
	})
	c.Assert(err, check.IsNil)
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	containers := pods.Items[0].Spec.Containers
	pods.Items[0].Spec.Containers = nil
	pods.Items[0].Status = apiv1.PodStatus{}
	c.Assert(pods.Items[0], check.DeepEquals, apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-v1-deploy",
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-deploy":            "false",
				"tsuru.io/is-stopped":           "false",
				"tsuru.io/is-tsuru":             "true",
				"tsuru.io/app-name":             "myapp",
				"tsuru.io/is-isolated-run":      "false",
				"tsuru.io/builder":              "",
				"tsuru.io/app-process":          "",
				"tsuru.io/is-build":             "true",
				"tsuru.io/app-platform":         "python",
				"tsuru.io/is-service":           "true",
				"tsuru.io/app-process-replicas": "0",
				"tsuru.io/app-pool":             "test-default",
				"tsuru.io/provisioner":          "kubernetes",
			},
			Annotations: map[string]string{
				"tsuru.io/build-image": "destimg",
				"tsuru.io/router-name": "fake",
				"tsuru.io/router-type": "fake",
			},
		},
		Spec: apiv1.PodSpec{
			ServiceAccountName: "app-myapp",
			NodeName:           "n1",
			NodeSelector:       map[string]string{"tsuru.io/pool": "test-default"},
			Volumes: []apiv1.Volume{
				{
					Name: "dockersock",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: dockerSockPath,
						},
					},
				},
				{
					Name: "intercontainer",
					VolumeSource: apiv1.VolumeSource{
						EmptyDir: &apiv1.EmptyDirVolumeSource{},
					},
				},
			},
			RestartPolicy: apiv1.RestartPolicyNever,
		},
	})
	c.Assert(containers, check.HasLen, 2)
	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
	runAsUser := int64(1000)
	c.Assert(containers, check.DeepEquals, []apiv1.Container{
		{
			Name:  "committer-cont",
			Image: "tsuru/deploy-agent:0.6.0",
			VolumeMounts: []apiv1.VolumeMount{
				{Name: "dockersock", MountPath: dockerSockPath},
				{Name: "intercontainer", MountPath: buildIntercontainerPath},
			},
			Stdin:     true,
			StdinOnce: true,
			Command: []string{
				"sh", "-ec", `
				end() { touch /tmp/intercontainer/done; }
				trap end EXIT
				mkdir -p $(dirname /dev/null) && cat >/dev/null && tsuru_unit_agent   myapp deploy-only
			`,
			},
			Env: []apiv1.EnvVar{
				{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
				{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "destimg"},
				{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
				{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
				{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/dev/null"},
				{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
				{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "false"},
			}},
		{
			Name:    "myapp-v1-deploy",
			Image:   "myimg",
			Command: []string{"/bin/sh", "-ec", `while [ ! -f /tmp/intercontainer/done ]; do sleep 5; done`},
			Env:     []apiv1.EnvVar{{Name: "TSURU_HOST", Value: ""}},
			SecurityContext: &apiv1.SecurityContext{
				RunAsUser: &runAsUser,
			},
			VolumeMounts: []apiv1.VolumeMount{
				{Name: "intercontainer", MountPath: buildIntercontainerPath},
			},
		},
	})
}

func (s *S) TestCreateDeployPodContainersWithRegistryAuth(c *check.C) {
	config.Set("docker:registry", "registry.example.com")
	defer config.Unset("docker:registry")
	config.Set("docker:registry-auth:username", "user")
	defer config.Unset("docker:registry-auth:username")
	config.Set("docker:registry-auth:password", "pwd")
	defer config.Unset("docker:registry-auth:password")
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	err = createDeployPod(context.Background(), createPodParams{
		client:            s.clusterClient,
		app:               a,
		sourceImage:       "myimg",
		destinationImages: []string{"registry.example.com/destimg"},
		inputFile:         "/dev/null",
	})
	c.Assert(err, check.IsNil)
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	containers := pods.Items[0].Spec.Containers
	pods.Items[0].Spec.Containers = nil
	pods.Items[0].Status = apiv1.PodStatus{}
	c.Assert(pods.Items[0], check.DeepEquals, apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-v1-deploy",
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-deploy":            "false",
				"tsuru.io/is-stopped":           "false",
				"tsuru.io/is-tsuru":             "true",
				"tsuru.io/app-name":             "myapp",
				"tsuru.io/is-isolated-run":      "false",
				"tsuru.io/builder":              "",
				"tsuru.io/app-process":          "",
				"tsuru.io/is-build":             "true",
				"tsuru.io/app-platform":         "python",
				"tsuru.io/is-service":           "true",
				"tsuru.io/app-process-replicas": "0",
				"tsuru.io/app-pool":             "test-default",
				"tsuru.io/provisioner":          "kubernetes",
			},
			Annotations: map[string]string{
				"tsuru.io/build-image": "registry.example.com/destimg",
				"tsuru.io/router-name": "fake",
				"tsuru.io/router-type": "fake",
			},
		},
		Spec: apiv1.PodSpec{
			ServiceAccountName: "app-myapp",
			NodeName:           "n1",
			NodeSelector:       map[string]string{"tsuru.io/pool": "test-default"},
			Volumes: []apiv1.Volume{
				{
					Name: "dockersock",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: dockerSockPath,
						},
					},
				},
				{
					Name: "intercontainer",
					VolumeSource: apiv1.VolumeSource{
						EmptyDir: &apiv1.EmptyDirVolumeSource{},
					},
				},
			},
			RestartPolicy: apiv1.RestartPolicyNever,
		},
	})
	c.Assert(containers, check.HasLen, 2)
	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
	c.Assert(containers[0].Command, check.HasLen, 3)
	c.Assert(containers[0].Command[:2], check.DeepEquals, []string{"sh", "-ec"})
	c.Assert(containers[0].Env, check.DeepEquals, []apiv1.EnvVar{
		{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
		{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "registry.example.com/destimg"},
		{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
		{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: "user"},
		{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: "pwd"},
		{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: "registry.example.com"},
		{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/dev/null"},
		{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
		{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "false"},
	})
	cmds := cleanCmds(containers[0].Command[2])
	c.Assert(cmds, check.Equals, `end() { touch /tmp/intercontainer/done; }
trap end EXIT
mkdir -p $(dirname /dev/null) && cat >/dev/null && tsuru_unit_agent   myapp deploy-only`)
}

func (s *S) TestCreateImageBuildPodContainer(c *check.C) {
	_, rollback := s.mock.NoAppReactions(c)
	defer rollback()
	err := createImageBuildPod(context.Background(), createPodParams{
		client:            s.clusterClient,
		podName:           "myplatform-image-build",
		destinationImages: []string{"destimg"},
		inputFile:         "/data/context.tar.gz",
	})
	c.Assert(err, check.IsNil)
	pods, err := s.client.Core().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	containers := pods.Items[0].Spec.Containers
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].Command, check.HasLen, 3)
	c.Assert(containers[0].Command[:2], check.DeepEquals, []string{"sh", "-ec"})
	c.Assert(containers[0].Env, check.DeepEquals, []apiv1.EnvVar{
		{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
		{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "destimg"},
		{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
		{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
		{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
		{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
		{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/data/context.tar.gz"},
		{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
		{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "true"},
	})
	c.Assert(containers[0].Image, check.DeepEquals, "tsuru/deploy-agent:0.6.0")
	cmds := cleanCmds(containers[0].Command[2])
	c.Assert(cmds, check.Equals, `mkdir -p $(dirname /data/context.tar.gz) && cat >/data/context.tar.gz && tsuru_unit_agent`)

}

func (s *S) TestCreateDeployPodProgress(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	fakeWatcher := watch.NewFakeWithChanSize(2, false)
	fakeWatcher.Add(&apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: "myapp-v1-deploy.1",
		},
		InvolvedObject: apiv1.ObjectReference{
			Name: "myapp-v1-deploy",
		},
		Source: apiv1.EventSource{
			Component: "c1",
		},
		Message: "msg1",
	})
	fakeWatcher.Add(&apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: "myapp-v1-deploy.2",
		},
		InvolvedObject: apiv1.ObjectReference{
			Name:      "myapp-v1-deploy",
			FieldPath: "mycont",
		},
		Source: apiv1.EventSource{
			Component: "c1",
			Host:      "n1",
		},
		Message: "msg2",
	})
	watchCalled := make(chan struct{})
	podReactorDone := make(chan struct{})
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		pod := obj.(*apiv1.Pod)
		pod.Status.ContainerStatuses = []apiv1.ContainerStatus{
			{State: apiv1.ContainerState{Running: &apiv1.ContainerStateRunning{}}},
		}
		podCopy := *pod
		go func() {
			defer close(podReactorDone)
			<-watchCalled
			time.Sleep(time.Second)
			podCopy.Status.ContainerStatuses = nil
			s.clusterClient.CoreV1().Pods(ns).Update(&podCopy)
		}()
		return false, nil, nil
	})
	s.client.PrependWatchReactor("events", func(action ktesting.Action) (handled bool, ret watch.Interface, err error) {
		close(watchCalled)
		return true, fakeWatcher, nil
	})
	buf := safe.NewBuffer(nil)
	err = createDeployPod(context.Background(), createPodParams{
		client:            s.clusterClient,
		app:               a,
		sourceImage:       "myimg",
		destinationImages: []string{"destimg"},
		inputFile:         "/dev/null",
		attachInput:       strings.NewReader("."),
		attachOutput:      buf,
	})
	c.Assert(err, check.IsNil)
	<-podReactorDone
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	c.Assert(buf.String(), check.Matches, `(?s).*stdout data.*`)
	c.Assert(buf.String(), check.Matches, `(?s).*stderr data.*`)
	c.Assert(buf.String(), check.Matches, `(?s).* ---> myapp-v1-deploy - msg1 \[c1\].* ---> myapp-v1-deploy - mycont - msg2 \[c1, n1\].*`)
}

func (s *S) TestCreateDeployPodAttachFail(c *check.C) {
	config.Set("kubernetes:attach-after-finish-timeout", 1)
	defer config.Unset("kubernetes:attach-after-finish-timeout")
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	ch := make(chan struct{})
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		ch <- struct{}{}
		w.Write([]byte("ignored"))
	}
	err = createDeployPod(context.Background(), createPodParams{
		client:            s.clusterClient,
		app:               a,
		sourceImage:       "myimg",
		destinationImages: []string{"destimg"},
		inputFile:         "/dev/null",
		attachInput:       strings.NewReader("."),
		attachOutput:      buf,
	})
	c.Assert(err, check.ErrorMatches, `error attaching to myapp-v1-deploy/committer-cont: container finished while attach is running`)
	<-ch
}

func (s *S) TestCreateDeployPodContainersWithTag(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	err = createDeployPod(context.Background(), createPodParams{
		client:            s.clusterClient,
		app:               a,
		sourceImage:       "myimg",
		destinationImages: []string{"ip:destimg:v1"},
		inputFile:         "/dev/null",
	})
	c.Assert(err, check.IsNil)
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	containers := pods.Items[0].Spec.Containers
	pods.Items[0].Spec.Containers = nil
	pods.Items[0].Status = apiv1.PodStatus{}
	c.Assert(pods.Items[0], check.DeepEquals, apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-v1-deploy",
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-deploy":            "false",
				"tsuru.io/is-stopped":           "false",
				"tsuru.io/is-tsuru":             "true",
				"tsuru.io/app-name":             "myapp",
				"tsuru.io/is-isolated-run":      "false",
				"tsuru.io/builder":              "",
				"tsuru.io/app-process":          "",
				"tsuru.io/is-build":             "true",
				"tsuru.io/app-platform":         "python",
				"tsuru.io/is-service":           "true",
				"tsuru.io/app-process-replicas": "0",
				"tsuru.io/app-pool":             "test-default",
				"tsuru.io/provisioner":          "kubernetes",
			},
			Annotations: map[string]string{
				"tsuru.io/build-image": "ip:destimg:v1",
				"tsuru.io/router-name": "fake",
				"tsuru.io/router-type": "fake",
			},
		},
		Spec: apiv1.PodSpec{
			ServiceAccountName: "app-myapp",
			NodeName:           "n1",
			NodeSelector:       map[string]string{"tsuru.io/pool": "test-default"},
			Volumes: []apiv1.Volume{
				{
					Name: "dockersock",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: dockerSockPath,
						},
					},
				},
				{
					Name: "intercontainer",
					VolumeSource: apiv1.VolumeSource{
						EmptyDir: &apiv1.EmptyDirVolumeSource{},
					},
				},
			},
			RestartPolicy: apiv1.RestartPolicyNever,
		},
	})
	c.Assert(containers, check.HasLen, 2)
	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
	runAsUser := int64(1000)
	c.Assert(containers, check.DeepEquals, []apiv1.Container{
		{
			Name:  "committer-cont",
			Image: "tsuru/deploy-agent:0.6.0",
			VolumeMounts: []apiv1.VolumeMount{
				{Name: "dockersock", MountPath: dockerSockPath},
				{Name: "intercontainer", MountPath: buildIntercontainerPath},
			},
			Stdin:     true,
			StdinOnce: true,
			Command: []string{
				"sh", "-ec", `
				end() { touch /tmp/intercontainer/done; }
				trap end EXIT
				mkdir -p $(dirname /dev/null) && cat >/dev/null && tsuru_unit_agent   myapp deploy-only
			`,
			},
			Env: []apiv1.EnvVar{
				{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
				{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "ip:destimg:v1,ip:destimg:latest"},
				{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: ""},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: ""},
				{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: ""},
				{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/dev/null"},
				{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
				{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "false"},
			}},
		{
			Name:    "myapp-v1-deploy",
			Image:   "myimg",
			Command: []string{"/bin/sh", "-ec", `while [ ! -f /tmp/intercontainer/done ]; do sleep 5; done`},
			Env:     []apiv1.EnvVar{{Name: "TSURU_HOST", Value: ""}},
			SecurityContext: &apiv1.SecurityContext{
				RunAsUser: &runAsUser,
			},
			VolumeMounts: []apiv1.VolumeMount{
				{Name: "intercontainer", MountPath: buildIntercontainerPath},
			},
		},
	})
}

func (s *S) TestServiceManagerDeployServiceWithVolumes(c *check.C) {
	config.Set("docker:uid", 1001)
	defer config.Unset("docker:uid")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
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
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(ns).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep.Spec.Template.Spec.Volumes, check.DeepEquals, []apiv1.Volume{
		{
			Name: "v1-tsuru",
			VolumeSource: apiv1.VolumeSource{
				PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
					ClaimName: "v1-tsuru-claim",
					ReadOnly:  false,
				},
			},
		},
	})
	c.Assert(dep.Spec.Template.Spec.Containers[0].VolumeMounts, check.DeepEquals, []apiv1.VolumeMount{
		{
			Name:      "v1-tsuru",
			MountPath: "/mnt",
			ReadOnly:  false,
		},
	})
}

func (s *S) TestServiceManagerDeployServiceRollbackFullTimeout(c *check.C) {
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	config.Set("kubernetes:deployment-progress-timeout", 2)
	defer config.Unset("kubernetes:deployment-progress-timeout")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	buf := bytes.Buffer{}
	m := serviceManager{client: s.clusterClient, writer: &buf}
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
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	var rollbackObj *extensions.DeploymentRollback
	reaction := func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackObj = obj.(*extensions.DeploymentRollback)
			return true, rollbackObj, nil
		}
		dep := obj.(*v1beta2.Deployment)
		dep.Status.UnavailableReplicas = 2
		rev, _ := strconv.Atoi(dep.Annotations[replicaDepRevision])
		rev++
		dep.Annotations[replicaDepRevision] = strconv.Itoa(rev)
		return false, nil, nil
	}
	s.client.PrependReactor("create", "deployments", reaction)
	s.client.PrependReactor("update", "deployments", reaction)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		pod.Status.Conditions = append(pod.Status.Conditions, apiv1.PodCondition{
			Type:   apiv1.PodReady,
			Status: apiv1.ConditionFalse,
		})
		return false, nil, nil
	})
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.ErrorMatches, "^timeout waiting full rollout after .+ waiting for units: Pod myapp-p1-pod-2-1: invalid pod phase \"Running\"$")
	waitDep()
	c.Assert(rollbackObj, check.DeepEquals, &extensions.DeploymentRollback{
		Name: "myapp-p1",
	})
	c.Assert(buf.String(), check.Matches, `(?s).*---- Updating units \[p1\] ----.*ROLLING BACK AFTER FAILURE.*---> timeout waiting full rollout after .* waiting for units: Pod myapp-p1-pod-2-1: invalid pod phase \"Running\" <---\s*$`)
	err = cleanupDeployment(s.clusterClient, a, "p1")
	c.Assert(err, check.IsNil)
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	_, err = s.client.CoreV1().Events(ns).Create(&apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod.evt1",
			Namespace: ns,
		},
		Reason:  "Unhealthy",
		Message: "my evt message",
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.ErrorMatches, "^timeout waiting full rollout after .+ waiting for units: Pod myapp-p1-pod-3-1: invalid pod phase \"Running\" - last event: my evt message$")
}

func (s *S) TestServiceManagerDeployServiceRollbackHealthcheckTimeout(c *check.C) {
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	config.Set("kubernetes:deployment-progress-timeout", 2)
	defer config.Unset("kubernetes:deployment-progress-timeout")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	buf := bytes.Buffer{}
	m := serviceManager{client: s.clusterClient, writer: &buf}
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
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	var rollbackObj *extensions.DeploymentRollback
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	reaction := func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackObj = obj.(*extensions.DeploymentRollback)
			return true, rollbackObj, nil
		}
		dep := obj.(*v1beta2.Deployment)
		rev, _ := strconv.Atoi(dep.Annotations[replicaDepRevision])
		rev++
		dep.Annotations[replicaDepRevision] = strconv.Itoa(rev)
		dep.Status.UnavailableReplicas = 2
		labelsCp := make(map[string]string, len(dep.Labels))
		for k, v := range dep.Spec.Template.Labels {
			labelsCp[k] = v
		}
		go func() {
			_, repErr := s.client.Clientset.AppsV1beta2().ReplicaSets(ns).Create(&v1beta2.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "replica-for-" + dep.Name,
					Labels: labelsCp,
					Annotations: map[string]string{
						"deployment.kubernetes.io/revision": strconv.Itoa(rev),
					},
				},
			})
			c.Assert(repErr, check.IsNil)
		}()
		return false, nil, nil
	}
	s.client.PrependReactor("create", "deployments", reaction)
	s.client.PrependReactor("update", "deployments", reaction)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		pod.Status.Conditions = append(pod.Status.Conditions, apiv1.PodCondition{
			Type:   apiv1.PodReady,
			Status: apiv1.ConditionFalse,
		})
		pod.OwnerReferences = append(pod.OwnerReferences, metav1.OwnerReference{
			Kind: "ReplicaSet",
			Name: "replica-for-myapp-p1",
		})
		return false, nil, nil
	})
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.ErrorMatches, "^timeout waiting healthcheck after .+ waiting for units: Pod myapp-p1-pod-2-1: invalid pod phase \"Running\"$")
	waitDep()
	c.Assert(rollbackObj, check.DeepEquals, &extensions.DeploymentRollback{
		Name: "myapp-p1",
	})
	c.Assert(buf.String(), check.Matches, `(?s).*---- Updating units \[p1\] ----.*ROLLING BACK AFTER FAILURE.*---> timeout waiting healthcheck after .* waiting for units: Pod myapp-p1-pod-2-1: invalid pod phase \"Running\" <---\s*$`)
	err = cleanupDeployment(s.clusterClient, a, "p1")
	c.Assert(err, check.IsNil)
	_, err = s.client.CoreV1().Events(ns).Create(&apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod.evt1",
			Namespace: ns,
		},
		Reason:  "Unhealthy",
		Message: "my evt message",
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.ErrorMatches, "^timeout waiting healthcheck after .+ waiting for units: Pod myapp-p1-pod-3-1: invalid pod phase \"Running\" - last event: my evt message$")
	waitDep()
}

func (s *S) TestServiceManagerDeployServiceRollbackPendingPod(c *check.C) {
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	config.Set("kubernetes:deployment-progress-timeout", 2)
	defer config.Unset("kubernetes:deployment-progress-timeout")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	buf := bytes.Buffer{}
	m := serviceManager{client: s.clusterClient, writer: &buf}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cmd1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	var rollbackObj *extensions.DeploymentRollback
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackObj = obj.(*extensions.DeploymentRollback)
			return true, rollbackObj, nil
		}
		return false, nil, nil
	})
	s.client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		dep := obj.(*v1beta2.Deployment)
		rev, _ := strconv.Atoi(dep.Annotations[replicaDepRevision])
		rev++
		dep.Annotations[replicaDepRevision] = strconv.Itoa(rev)
		dep.Status.UnavailableReplicas = 2
		dep.Status.ObservedGeneration = 12
		labelsCp := make(map[string]string, len(dep.Labels))
		for k, v := range dep.Spec.Template.Labels {
			labelsCp[k] = v
		}
		go func() {
			ns, nsErr := s.client.AppNamespace(a)
			c.Assert(nsErr, check.IsNil)
			_, repErr := s.client.Clientset.AppsV1beta2().ReplicaSets(ns).Create(&v1beta2.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "replica-for-" + dep.Name,
					Labels: labelsCp,
					Annotations: map[string]string{
						"deployment.kubernetes.io/revision": strconv.Itoa(int(dep.Status.ObservedGeneration)),
					},
				},
			})
			c.Assert(repErr, check.IsNil)
		}()
		return false, nil, nil
	})
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		pod.Status.Conditions = append(pod.Status.Conditions, apiv1.PodCondition{
			Type:   apiv1.PodReady,
			Status: apiv1.ConditionFalse,
		})
		pod.Status.Phase = apiv1.PodPending
		pod.OwnerReferences = append(pod.OwnerReferences, metav1.OwnerReference{
			Kind: "ReplicaSet",
			Name: "replica-for-myapp-p1",
		})
		return false, nil, nil
	})
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.ErrorMatches, "^timeout waiting full rollout after .+ waiting for units: Pod myapp-p1-pod-2-1: invalid pod phase \"Pending\"$")
	waitDep()
	c.Assert(rollbackObj, check.DeepEquals, &extensions.DeploymentRollback{
		Name: "myapp-p1",
	})
}

func (s *S) TestServiceManagerDeployServiceNoRollbackFullTimeoutSameRevision(c *check.C) {
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	config.Set("kubernetes:deployment-progress-timeout", 2)
	defer config.Unset("kubernetes:deployment-progress-timeout")
	buf := bytes.Buffer{}
	m := serviceManager{client: s.clusterClient, writer: &buf}
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
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	_, err = s.client.AppsV1beta2().Deployments(ns).Create(&v1beta2.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "myapp-p1",
		},
		Spec: v1beta2.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
		},
	})
	c.Assert(err, check.IsNil)
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	var rollbackCalled bool
	s.client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackCalled = true
			return false, nil, nil
		}
		dep := obj.(*v1beta2.Deployment)
		dep.Status.UnavailableReplicas = 2
		return false, nil, nil
	})
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		pod.Status.Conditions = append(pod.Status.Conditions, apiv1.PodCondition{
			Type:   apiv1.PodReady,
			Status: apiv1.ConditionFalse,
		})
		return false, nil, nil
	})
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	}, nil)
	c.Assert(err, check.ErrorMatches, "^timeout waiting full rollout after .+ waiting for units: Pod myapp-p1-pod-1-1: invalid pod phase \"Running\"$")
	waitDep()
	c.Assert(rollbackCalled, check.Equals, false)
	c.Assert(buf.String(), check.Matches, `(?s).*---- Updating units \[p1\] ----.*UPDATING BACK AFTER FAILURE.*---> timeout waiting full rollout after .* waiting for units: Pod myapp-p1-pod-1-1: invalid pod phase \"Running\" <---\s*$`)
}

func (s *S) TestServiceManagerRemoveService(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", nil, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	expectedLabels := map[string]string{
		"tsuru.io/is-tsuru":             "true",
		"tsuru.io/is-build":             "false",
		"tsuru.io/is-stopped":           "false",
		"tsuru.io/is-service":           "true",
		"tsuru.io/is-deploy":            "false",
		"tsuru.io/is-isolated-run":      "false",
		"tsuru.io/app-name":             a.GetName(),
		"tsuru.io/app-process":          "p1",
		"tsuru.io/app-process-replicas": "1",
		"tsuru.io/restarts":             "0",
		"tsuru.io/app-platform":         a.GetPlatform(),
		"tsuru.io/app-pool":             a.GetPool(),
		"tsuru.io/provisioner":          provisionerName,
		"tsuru.io/builder":              "",
	}
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	_, err = s.client.Clientset.AppsV1beta2().ReplicaSets(ns).Create(&v1beta2.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: ns,
			Labels:    expectedLabels,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.CoreV1().Pods(ns).Create(&apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: ns,
			Labels:    expectedLabels,
		},
	})
	c.Assert(err, check.IsNil)
	err = m.RemoveService(a, "p1")
	c.Assert(err, check.IsNil)
	deps, err := s.client.Clientset.AppsV1beta2().Deployments(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	srvs, err := s.client.CoreV1().Services(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srvs.Items, check.HasLen, 0)
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.Clientset.AppsV1beta2().ReplicaSets(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestServiceManagerRemoveServiceMiddleFailure(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", nil, nil)
	c.Assert(err, check.IsNil)
	waitDep()
	s.client.PrependReactor("delete", "deployments", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("my dep err")
	})
	err = m.RemoveService(a, "p1")
	c.Assert(err, check.ErrorMatches, "(?s).*my dep err.*")
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	deps, err := s.client.Clientset.AppsV1beta2().Deployments(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	srvs, err := s.client.CoreV1().Services(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srvs.Items, check.HasLen, 0)
}

func (s *S) TestEnsureNamespace(c *check.C) {
	tests := []struct {
		name       string
		customData map[string]string
		expected   apiv1.Namespace
	}{
		{
			name:     "myns",
			expected: apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myns"}},
		},
		{
			name: "myns",
			customData: map[string]string{
				"namespace-labels": "lb1= val1,lb2 =val2 ",
			},
			expected: apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: "myns",
				Labels: map[string]string{
					"lb1": "val1",
					"lb2": "val2",
				},
			}},
		},
		{
			name: "myns",
			customData: map[string]string{
				"namespace-labels":       "lb1= val1,lb2 =val2 ",
				"myns:namespace-labels":  "lb3=val3",
				"other:namespace-labels": "lb4=val4",
			},
			expected: apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: "myns",
				Labels: map[string]string{
					"lb3": "val3",
				},
			}},
		},
		{
			name: "myns2",
			customData: map[string]string{
				"namespace-labels":      "lb1= val1,lb2 =val2 ",
				"myns:namespace-labels": "lb3=val3",
			},
			expected: apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: "myns2",
				Labels: map[string]string{
					"lb1": "val1",
					"lb2": "val2",
				},
			}},
		},
	}
	for _, tt := range tests {
		s.clusterClient.CustomData = tt.customData
		err := ensureNamespace(s.clusterClient, tt.name)
		c.Assert(err, check.IsNil)
		nss, err := s.client.CoreV1().Namespaces().List(metav1.ListOptions{})
		c.Assert(err, check.IsNil)
		c.Assert(nss.Items, check.DeepEquals, []apiv1.Namespace{
			tt.expected,
		})
		err = s.client.CoreV1().Namespaces().Delete(tt.name, nil)
		c.Assert(err, check.IsNil)
	}
}
