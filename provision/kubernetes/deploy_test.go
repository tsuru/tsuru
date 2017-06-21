// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"gopkg.in/check.v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestServiceManagerDeployService(c *check.C) {
	waitDep := s.deploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.client.clusterClient}
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
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Extensions().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	one := int32(1)
	ten := int32(10)
	maxSurge := intstr.FromString("100%")
	maxUnavailable := intstr.FromInt(0)
	expectedUID := int64(1000)
	c.Assert(dep, check.DeepEquals, &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: s.client.Namespace(),
		},
		Status: extensions.DeploymentStatus{
			UpdatedReplicas: 1,
			Replicas:        1,
		},
		Spec: extensions.DeploymentSpec{
			Strategy: extensions.DeploymentStrategy{
				Type: extensions.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &extensions.RollingUpdateDeployment{
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
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
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
						"tsuru.io/app-pool":             "bonehunters",
						"tsuru.io/router-type":          "fake",
						"tsuru.io/router-name":          "fake",
						"tsuru.io/provisioner":          "kubernetes",
					},
				},
				Spec: v1.PodSpec{
					SecurityContext: &v1.PodSecurityContext{
						RunAsUser: &expectedUID,
					},
					NodeSelector: map[string]string{
						"pool": "bonehunters",
					},
					RestartPolicy: "Always",
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
								{Name: "TSURU_PROCESSNAME", Value: "p1"},
								{Name: "TSURU_HOST", Value: ""},
								{Name: "port", Value: "8888"},
								{Name: "PORT", Value: "8888"},
							},
							Resources: v1.ResourceRequirements{
								Limits: v1.ResourceList{},
							},
						},
					},
				},
			},
		},
	})
	srv, err := s.client.Core().Services(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srv, check.DeepEquals, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: s.client.Namespace(),
			Labels: map[string]string{
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
				"tsuru.io/app-pool":             "bonehunters",
				"tsuru.io/router-type":          "fake",
				"tsuru.io/router-name":          "fake",
				"tsuru.io/provisioner":          "kubernetes",
			},
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":        "myapp",
				"tsuru.io/app-process":     "p1",
				"tsuru.io/is-build":        "false",
				"tsuru.io/is-isolated-run": "false",
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

func (s *S) TestServiceManagerDeployServiceUpdateStates(c *check.C) {
	waitDep := s.deploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.client.clusterClient}
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
		fn     func(dep *extensions.Deployment)
	}{
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 1},
			},
			fn: func(dep *extensions.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(2))
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true},
			},
			fn: func(dep *extensions.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(0))
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.AppReplicas(), check.Equals, 3)
				c.Assert(ls.IsStopped(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true},
			},
			fn: func(dep *extensions.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Start: true},
			},
			fn: func(dep *extensions.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(3))
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {Start: true},
			},
			fn: func(dep *extensions.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Restart: true},
			},
			fn: func(dep *extensions.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(3))
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {Restart: true},
			},
			fn: func(dep *extensions.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {},
			},
			fn: func(dep *extensions.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(0))
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.AppReplicas(), check.Equals, 3)
				c.Assert(ls.IsStopped(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {},
			},
			fn: func(dep *extensions.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Restart: true}, {Restart: true},
			},
			fn: func(dep *extensions.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(1))
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.Restarts(), check.Equals, 2)
			},
		},
	}
	for _, tt := range tests {
		for _, s := range tt.states {
			err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
				"p1": s,
			})
			c.Assert(err, check.IsNil)
		}
		var dep *extensions.Deployment
		dep, err = s.client.Extensions().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
		c.Assert(err, check.IsNil)
		waitDep()
		tt.fn(dep)
		err = cleanupDeployment(s.client.clusterClient, a, "p1")
		c.Assert(err, check.IsNil)
		err = cleanupDeployment(s.client.clusterClient, a, "p2")
		c.Assert(err, check.IsNil)
	}
}

func (s *S) TestServiceManagerDeployServiceWithHC(c *check.C) {
	waitDep := s.deploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.client.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
			"p2": "cmd2",
		},
		"healthcheck": provision.TsuruYamlHealthcheck{
			Path: "/hc",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Extensions().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep.Spec.Template.Spec.Containers[0].ReadinessProbe, check.DeepEquals, &v1.Probe{
		Handler: v1.Handler{
			HTTPGet: &v1.HTTPGetAction{
				Path: "/hc",
				Port: intstr.FromInt(8888),
			},
		},
	})
}

func (s *S) TestServiceManagerDeployServiceWithNodeContainers(c *check.C) {
	waitDep := s.deploymentReactions(c)
	defer waitDep()
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	m := serviceManager{client: s.client.clusterClient}
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
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Extensions().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep, check.NotNil)
	daemon, err := s.client.Extensions().DaemonSets(s.client.Namespace()).Get("node-container-bs-all", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemon, check.NotNil)
}

func (s *S) TestServiceManagerDeployServiceWithHCInvalidMethod(c *check.C) {
	m := serviceManager{client: s.client.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
			"p2": "cmd2",
		},
		"healthcheck": provision.TsuruYamlHealthcheck{
			Path:   "/hc",
			Method: "POST",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.ErrorMatches, "healthcheck: only GET method is supported in kubernetes provisioner")
}

func (s *S) TestServiceManagerDeployServiceWithUID(c *check.C) {
	config.Set("docker:uid", 1001)
	defer config.Unset("docker:uid")
	waitDep := s.deploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.client.clusterClient}
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
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Extensions().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedUID := int64(1001)
	c.Assert(dep.Spec.Template.Spec.SecurityContext, check.DeepEquals, &v1.PodSecurityContext{
		RunAsUser: &expectedUID,
	})
}

func (s *S) TestServiceManagerDeployServiceWithLimits(c *check.C) {
	waitDep := s.deploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.client.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	a.Plan = app.Plan{Memory: 1024}
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Extensions().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedMemory := resource.NewQuantity(1024, resource.BinarySI)
	c.Assert(dep.Spec.Template.Spec.Containers[0].Resources, check.DeepEquals, v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *expectedMemory,
		},
	})
}

func (s *S) prepareRollbackTest(c *check.C) (*serviceManager, **extensions.DeploymentRollback, func()) {
	config.Set("docker:healthcheck:max-time", 1)
	waitDep := s.deploymentReactions(c)
	buf := bytes.Buffer{}
	m := serviceManager{client: s.client.clusterClient, writer: &buf}
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
	var rollbackObj *extensions.DeploymentRollback
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackObj = obj.(*extensions.DeploymentRollback)
			return true, rollbackObj, nil
		}
		dep := obj.(*extensions.Deployment)
		dep.Status.UnavailableReplicas = 2
		return false, nil, nil
	})
	return &m, &rollbackObj, func() {
		waitDep()
		config.Unset("docker:healthcheck:max-time")
	}
}

func (s *S) TestServiceManagerDeployServiceRollback(c *check.C) {
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	waitDep := s.deploymentReactions(c)
	defer waitDep()
	buf := bytes.Buffer{}
	m := serviceManager{client: s.client.clusterClient, writer: &buf}
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
	var rollbackObj *extensions.DeploymentRollback
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackObj = obj.(*extensions.DeploymentRollback)
			return true, rollbackObj, nil
		}
		dep := obj.(*extensions.Deployment)
		dep.Status.UnavailableReplicas = 2
		return false, nil, nil
	})
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*v1.Pod)
		pod.Status.Conditions = append(pod.Status.Conditions, v1.PodCondition{
			Type:   v1.PodReady,
			Status: v1.ConditionFalse,
		})
		return false, nil, nil
	})
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.ErrorMatches, "^timeout after .+ waiting for units$")
	c.Assert(rollbackObj, check.DeepEquals, &extensions.DeploymentRollback{
		Name: "myapp-p1",
	})
	c.Assert(buf.String(), check.Matches, `(?s).*---- Updating units \[p1\] ----.*ROLLING BACK AFTER FAILURE.*---> timeout after .* waiting for units <---\s*$`)
	cleanupDeployment(s.client.clusterClient, a, "p1")
	_, err = s.client.Core().Events(s.client.Namespace()).Create(&v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod.evt1",
			Namespace: s.client.Namespace(),
		},
		Reason:  "Unhealthy",
		Message: "my evt message",
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.ErrorMatches, "^timeout after .+ waiting for units: Pod myapp-p1-pod-2-1: Unhealthy - my evt message$")
}

func (s *S) TestServiceManagerRemoveService(c *check.C) {
	waitDep := s.deploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.client.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", nil)
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
		"tsuru.io/router-name":          "fake",
		"tsuru.io/router-type":          "fake",
		"tsuru.io/provisioner":          provisionerName,
	}
	_, err = s.client.Extensions().ReplicaSets(s.client.Namespace()).Create(&extensions.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: s.client.Namespace(),
			Labels:    expectedLabels,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Core().Pods(s.client.Namespace()).Create(&v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: s.client.Namespace(),
			Labels:    expectedLabels,
		},
	})
	c.Assert(err, check.IsNil)
	err = m.RemoveService(a, "p1")
	c.Assert(err, check.IsNil)
	deps, err := s.client.Extensions().Deployments(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	srvs, err := s.client.Core().Services(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srvs.Items, check.HasLen, 0)
	pods, err := s.client.Core().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.Extensions().ReplicaSets(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestServiceManagerRemoveServiceMiddleFailure(c *check.C) {
	waitDep := s.deploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.client.clusterClient}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", nil)
	c.Assert(err, check.IsNil)
	waitDep()
	s.client.PrependReactor("delete", "deployments", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("my dep err")
	})
	err = m.RemoveService(a, "p1")
	c.Assert(err, check.ErrorMatches, "(?s).*my dep err.*")
	deps, err := s.client.Extensions().Deployments(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	srvs, err := s.client.Core().Services(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srvs.Items, check.HasLen, 0)
}
