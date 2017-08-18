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
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/volume"
	"gopkg.in/check.v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
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
	c.Assert(dep, check.DeepEquals, &v1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: s.client.Namespace(),
		},
		Status: v1beta1.DeploymentStatus{
			UpdatedReplicas: 1,
			Replicas:        1,
		},
		Spec: v1beta1.DeploymentSpec{
			Strategy: v1beta1.DeploymentStrategy{
				Type: v1beta1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &v1beta1.RollingUpdateDeployment{
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
						"tsuru.io/app-pool":             "test-default",
						"tsuru.io/router-type":          "fake",
						"tsuru.io/router-name":          "fake",
						"tsuru.io/provisioner":          "kubernetes",
						"tsuru.io/builder":              "",
					},
				},
				Spec: apiv1.PodSpec{
					SecurityContext: &apiv1.PodSecurityContext{
						RunAsUser: &expectedUID,
					},
					NodeSelector: map[string]string{
						"pool": "test-default",
					},
					RestartPolicy: "Always",
					Containers: []apiv1.Container{
						{
							Name:  "myapp-p1",
							Image: "myimg",
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
								Limits: apiv1.ResourceList{},
							},
						},
					},
				},
			},
		},
	})
	srv, err := s.client.Core().Services(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srv, check.DeepEquals, &apiv1.Service{
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
				"tsuru.io/app-pool":             "test-default",
				"tsuru.io/router-type":          "fake",
				"tsuru.io/router-name":          "fake",
				"tsuru.io/provisioner":          "kubernetes",
				"tsuru.io/builder":              "",
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
				},
			},
			Type: apiv1.ServiceTypeNodePort,
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
		fn     func(dep *v1beta1.Deployment)
	}{
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 1},
			},
			fn: func(dep *v1beta1.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(2))
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true},
			},
			fn: func(dep *v1beta1.Deployment) {
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
			fn: func(dep *v1beta1.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Start: true},
			},
			fn: func(dep *v1beta1.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(3))
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {Start: true},
			},
			fn: func(dep *v1beta1.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Restart: true},
			},
			fn: func(dep *v1beta1.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(3))
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {Restart: true},
			},
			fn: func(dep *v1beta1.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {},
			},
			fn: func(dep *v1beta1.Deployment) {
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
			fn: func(dep *v1beta1.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Restart: true}, {Restart: true},
			},
			fn: func(dep *v1beta1.Deployment) {
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
		var dep *v1beta1.Deployment
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
			"web": "cm1",
			"p2":  "cmd2",
		},
		"healthcheck": provision.TsuruYamlHealthcheck{
			Path: "/hc",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
		"p2":  servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Extensions().Deployments(s.client.Namespace()).Get("myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep.Spec.Template.Spec.Containers[0].ReadinessProbe, check.DeepEquals, &apiv1.Probe{
		Handler: apiv1.Handler{
			HTTPGet: &apiv1.HTTPGetAction{
				Path: "/hc",
				Port: intstr.FromInt(8888),
			},
		},
	})
	dep, err = s.client.Extensions().Deployments(s.client.Namespace()).Get("myapp-p2", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep.Spec.Template.Spec.Containers[0].ReadinessProbe, check.IsNil)
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
			"web": "cm1",
			"p2":  "cmd2",
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
	c.Assert(dep.Spec.Template.Spec.SecurityContext, check.DeepEquals, &apiv1.PodSecurityContext{
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
	a.Plan = appTypes.Plan{Memory: 1024}
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
	c.Assert(dep.Spec.Template.Spec.Containers[0].Resources, check.DeepEquals, apiv1.ResourceRequirements{
		Limits: apiv1.ResourceList{
			apiv1.ResourceMemory: *expectedMemory,
		},
	})
}

func (s *S) TestServiceManagerDeployServiceWithVolumes(c *check.C) {
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
	err = v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Extensions().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
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
	var rollbackObj *v1beta1.DeploymentRollback
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackObj = obj.(*v1beta1.DeploymentRollback)
			return true, rollbackObj, nil
		}
		dep := obj.(*v1beta1.Deployment)
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
	})
	c.Assert(err, check.ErrorMatches, "^timeout after .+ waiting for units$")
	c.Assert(rollbackObj, check.DeepEquals, &v1beta1.DeploymentRollback{
		Name: "myapp-p1",
	})
	c.Assert(buf.String(), check.Matches, `(?s).*---- Updating units \[p1\] ----.*ROLLING BACK AFTER FAILURE.*---> timeout after .* waiting for units <---\s*$`)
	cleanupDeployment(s.client.clusterClient, a, "p1")
	_, err = s.client.Core().Events(s.client.Namespace()).Create(&apiv1.Event{
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
		"tsuru.io/builder":              "",
	}
	_, err = s.client.Extensions().ReplicaSets(s.client.Namespace()).Create(&v1beta1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: s.client.Namespace(),
			Labels:    expectedLabels,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Core().Pods(s.client.Namespace()).Create(&apiv1.Pod{
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
