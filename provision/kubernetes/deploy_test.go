// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/safe"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/volume"
	"gopkg.in/check.v1"
	"k8s.io/api/apps/v1beta2"
	apiv1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
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
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	one := int32(1)
	ten := int32(10)
	maxSurge := intstr.FromString("100%")
	maxUnavailable := intstr.FromInt(0)
	expectedUID := int64(1000)
	labels := map[string]string{
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
	}
	annotations := map[string]string{
		"tsuru.io/router-type": "fake",
		"tsuru.io/router-name": "fake",
	}
	c.Assert(dep, check.DeepEquals, &v1beta2.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "myapp-p1",
			Namespace:   s.client.Namespace(),
			Labels:      labels,
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
					Labels:      labels,
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
	srv, err := s.client.CoreV1().Services(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
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
				},
			},
			Type: apiv1.ServiceTypeNodePort,
		},
	})
	srvHeadless, err := s.client.CoreV1().Services(s.client.Namespace()).Get("myapp-p1-units", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srvHeadless, check.DeepEquals, &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-units",
			Namespace: s.client.Namespace(),
			Labels: map[string]string{
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
				},
			},
			ClusterIP: "None",
			Type:      apiv1.ServiceTypeClusterIP,
		},
	})
	account, err := s.client.CoreV1().ServiceAccounts(s.client.Namespace()).Get("app-myapp", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(account, check.DeepEquals, &apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-myapp",
			Namespace: s.client.Namespace(),
			Labels: map[string]string{
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/provisioner": "kubernetes",
			},
		},
	})
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
	})
	c.Assert(err, check.IsNil)
	srv, err := s.client.CoreV1().Services(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
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
				},
			},
			Type: apiv1.ServiceTypeNodePort,
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
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.AppReplicas(), check.Equals, 3)
				c.Assert(ls.IsStopped(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Start: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(3))
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {Start: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Restart: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				c.Assert(*dep.Spec.Replicas, check.Equals, int32(3))
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {Restart: true},
			},
			fn: func(dep *v1beta2.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, false)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {},
			},
			fn: func(dep *v1beta2.Deployment) {
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
			fn: func(dep *v1beta2.Deployment) {
				ls := labelSetFromMeta(&dep.Spec.Template.ObjectMeta)
				c.Assert(ls.IsAsleep(), check.Equals, true)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Restart: true}, {Restart: true},
			},
			fn: func(dep *v1beta2.Deployment) {
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
		var dep *v1beta2.Deployment
		dep, err = s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
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
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "cm1",
			"p2":  "cmd2",
		},
		"healthcheck": provision.TsuruYamlHealthcheck{
			Path:   "/hc",
			Scheme: "https",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
		"p2":  servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep.Spec.Template.Spec.Containers[0].ReadinessProbe, check.DeepEquals, &apiv1.Probe{
		Handler: apiv1.Handler{
			HTTPGet: &apiv1.HTTPGetAction{
				Path:   "/hc",
				Port:   intstr.FromInt(8888),
				Scheme: apiv1.URISchemeHTTPS,
			},
		},
	})
	dep, err = s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-p2", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep.Spec.Template.Spec.Containers[0].ReadinessProbe, check.IsNil)
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
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		dep := obj.(*v1beta2.Deployment)
		dep.Status.UnavailableReplicas = 1
		depCopy := *dep
		go func() {
			<-watchCalled
			time.Sleep(time.Second)
			depCopy.Status.UnavailableReplicas = 0
			s.client.AppsV1beta2().Deployments(s.clusterClient.Namespace()).Update(&depCopy)
		}()
		return false, nil, nil
	})
	s.client.PrependWatchReactor("events", func(action ktesting.Action) (handled bool, ret watch.Interface, err error) {
		close(watchCalled)
		return true, fakeWatcher, nil
	})
	buf := bytes.NewBuffer(nil)
	m := serviceManager{client: s.clusterClient, writer: buf}
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "cmd1",
		},
	})
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Matches, `(?s).* ---> 1 of 1 new units created.*? ---> 0 of 1 new units ready.*? ---> 1 of 1 new units ready.*? ---> Done updating units.*`)
	c.Assert(buf.String(), check.Matches, `(?s).*  ---> pod-name-1 - msg1 \[c1\].*?  ---> pod-name-1 - msg2 \[c1, n1\].*`)
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
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep, check.NotNil)
	daemon, err := s.client.Clientset.AppsV1beta2().DaemonSets(s.client.Namespace()).Get("node-container-bs-all", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemon, check.NotNil)
}

func (s *S) TestServiceManagerDeployServiceWithHCInvalidMethod(c *check.C) {
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
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
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
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
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
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
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
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
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
	err := createBuildPod(createPodParams{
		client:           s.clusterClient,
		app:              a,
		sourceImage:      "myimg",
		destinationImage: "destimg",
		inputFile:        "/home/application/archive.tar.gz",
	})
	c.Assert(err, check.IsNil)
	pods, err := s.client.Core().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	containers := pods.Items[0].Spec.Containers
	c.Assert(containers, check.HasLen, 2)
	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
	runAsUser := int64(1000)
	c.Assert(containers[0], check.DeepEquals, apiv1.Container{
		Name:  "committer-cont",
		Image: "docker:1.11.2",
		VolumeMounts: []apiv1.VolumeMount{
			{Name: "dockersock", MountPath: dockerSockPath},
			{Name: "intercontainer", MountPath: buildIntercontainerPath},
		},
		TTY: true,
		Command: []string{
			"sh", "-ec",
			`
							end() { touch /tmp/intercontainer/done; }
							trap end EXIT
							while [ ! -f /tmp/intercontainer/status ]; do sleep 1; done
							exit_code=$(cat /tmp/intercontainer/status)
							[ "${exit_code}" != "0" ] && exit "${exit_code}"
							id=$(docker ps -aq -f "label=io.kubernetes.container.name=myapp-v1-build" -f "label=io.kubernetes.pod.name=$(hostname)")
							img="destimg"
							echo
							echo '---- Building application image ----'
							docker commit "${id}" "${img}" >/dev/null
							sz=$(docker history "${img}" | head -2 | tail -1 | grep -E -o '[0-9.]+\s[a-zA-Z]+\s*$' | sed 's/[[:space:]]*$//g')
							echo " ---> Sending image to repository (${sz})"
							` + `
							docker push destimg
						`,
		},
	})
	c.Assert(containers[1], check.DeepEquals, apiv1.Container{
		Name:  "myapp-v1-build",
		Image: "myimg",
		Command: []string{"/bin/sh", "-lc", `
		cat >/home/application/archive.tar.gz && tsuru_unit_agent   myapp "/var/lib/tsuru/deploy archive file:///home/application/archive.tar.gz" build
		exit_code=$?
		echo "${exit_code}" >/tmp/intercontainer/status
		[ "${exit_code}" != "0" ] && exit "${exit_code}"
		while [ ! -f /tmp/intercontainer/done ]; do sleep 1; done
	`},
		Stdin:     true,
		StdinOnce: true,
		Env:       []apiv1.EnvVar{{Name: "TSURU_HOST", Value: ""}},
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
	err := createDeployPod(createPodParams{
		client:           s.clusterClient,
		app:              a,
		sourceImage:      "myimg",
		destinationImage: "destimg",
		inputFile:        "/dev/null",
	})
	c.Assert(err, check.IsNil)
	pods, err := s.client.CoreV1().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	containers := pods.Items[0].Spec.Containers
	pods.Items[0].Spec.Containers = nil
	pods.Items[0].Status = apiv1.PodStatus{}
	c.Assert(pods.Items[0], check.DeepEquals, apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-v1-deploy",
			Namespace: s.client.Namespace(),
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
			Image: "docker:1.11.2",
			VolumeMounts: []apiv1.VolumeMount{
				{Name: "dockersock", MountPath: dockerSockPath},
				{Name: "intercontainer", MountPath: buildIntercontainerPath},
			},
			TTY: true,
			Command: []string{
				"sh", "-ec",
				`
							end() { touch /tmp/intercontainer/done; }
							trap end EXIT
							while [ ! -f /tmp/intercontainer/status ]; do sleep 1; done
							exit_code=$(cat /tmp/intercontainer/status)
							[ "${exit_code}" != "0" ] && exit "${exit_code}"
							id=$(docker ps -aq -f "label=io.kubernetes.container.name=myapp-v1-deploy" -f "label=io.kubernetes.pod.name=$(hostname)")
							img="destimg"
							echo
							echo '---- Building application image ----'
							docker commit "${id}" "${img}" >/dev/null
							sz=$(docker history "${img}" | head -2 | tail -1 | grep -E -o '[0-9.]+\s[a-zA-Z]+\s*$' | sed 's/[[:space:]]*$//g')
							echo " ---> Sending image to repository (${sz})"
							` + `
							docker push destimg
						`,
			},
		},
		{
			Name:  "myapp-v1-deploy",
			Image: "myimg",
			Command: []string{"/bin/sh", "-lc", `
		cat >/dev/null && tsuru_unit_agent   myapp deploy-only
		exit_code=$?
		echo "${exit_code}" >/tmp/intercontainer/status
		[ "${exit_code}" != "0" ] && exit "${exit_code}"
		while [ ! -f /tmp/intercontainer/done ]; do sleep 1; done
	`},
			Stdin:     true,
			StdinOnce: true,
			Env:       []apiv1.EnvVar{{Name: "TSURU_HOST", Value: ""}},
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
	err := createDeployPod(createPodParams{
		client:           s.clusterClient,
		app:              a,
		sourceImage:      "myimg",
		destinationImage: "registry.example.com/destimg",
		inputFile:        "/dev/null",
	})
	c.Assert(err, check.IsNil)
	pods, err := s.client.CoreV1().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	containers := pods.Items[0].Spec.Containers
	pods.Items[0].Spec.Containers = nil
	pods.Items[0].Status = apiv1.PodStatus{}
	c.Assert(pods.Items[0], check.DeepEquals, apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-v1-deploy",
			Namespace: s.client.Namespace(),
			Labels: map[string]string{
				"tsuru.io/is-deploy":            "false",
				"tsuru.io/is-stopped":           "false",
				"tsuru.io/is-tsuru":             "true",
				"tsuru.io/app-name":             "myapp",
				"tsuru.io/router-type":          "fake",
				"tsuru.io/is-isolated-run":      "false",
				"tsuru.io/builder":              "",
				"tsuru.io/app-process":          "",
				"tsuru.io/is-build":             "true",
				"tsuru.io/app-platform":         "python",
				"tsuru.io/is-service":           "true",
				"tsuru.io/app-process-replicas": "0",
				"tsuru.io/app-pool":             "test-default",
				"tsuru.io/provisioner":          "kubernetes",
				"tsuru.io/router-name":          "fake",
			},
			Annotations: map[string]string{"build-image": "registry.example.com/destimg"},
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
	cmds := cleanCmds(containers[0].Command[2])
	c.Assert(cmds, check.Equals, `end() { touch /tmp/intercontainer/done; }
trap end EXIT
while [ ! -f /tmp/intercontainer/status ]; do sleep 1; done
exit_code=$(cat /tmp/intercontainer/status)
[ "${exit_code}" != "0" ] && exit "${exit_code}"
id=$(docker ps -aq -f "label=io.kubernetes.container.name=myapp-v1-deploy" -f "label=io.kubernetes.pod.name=$(hostname)")
img="registry.example.com/destimg"
echo
echo '---- Building application image ----'
docker commit "${id}" "${img}" >/dev/null
sz=$(docker history "${img}" | head -2 | tail -1 | grep -E -o '[0-9.]+\s[a-zA-Z]+\s*$' | sed 's/[[:space:]]*$//g')
echo " ---> Sending image to repository (${sz})"
docker login -u "user" -p "pwd" "registry.example.com"
docker push registry.example.com/destimg`)
}

func (s *S) TestCreateDeployPodProgress(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
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
			s.clusterClient.CoreV1().Pods(s.clusterClient.Namespace()).Update(&podCopy)
		}()
		return false, nil, nil
	})
	s.client.PrependWatchReactor("events", func(action ktesting.Action) (handled bool, ret watch.Interface, err error) {
		close(watchCalled)
		return true, fakeWatcher, nil
	})
	buf := safe.NewBuffer(nil)
	err := createDeployPod(createPodParams{
		client:           s.clusterClient,
		app:              a,
		sourceImage:      "myimg",
		destinationImage: "destimg",
		inputFile:        "/dev/null",
		attachInput:      strings.NewReader("."),
		attachOutput:     buf,
	})
	c.Assert(err, check.IsNil)
	<-podReactorDone
	pods, err := s.client.CoreV1().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	c.Assert(buf.String(), check.Matches, `(?s).*stdout data.*`)
	c.Assert(buf.String(), check.Matches, `(?s).*stderr data.*`)
	c.Assert(buf.String(), check.Matches, `(?s).* ---> myapp-v1-deploy - msg1 \[c1\].* ---> myapp-v1-deploy - mycont - msg2 \[c1, n1\].*`)
}

func (s *S) TestCreateDeployPodContainersWithTag(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := createDeployPod(createPodParams{
		client:           s.clusterClient,
		app:              a,
		sourceImage:      "myimg",
		destinationImage: "ip:destimg:v1",
		inputFile:        "/dev/null",
	})
	c.Assert(err, check.IsNil)
	pods, err := s.client.CoreV1().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	containers := pods.Items[0].Spec.Containers
	pods.Items[0].Spec.Containers = nil
	pods.Items[0].Status = apiv1.PodStatus{}
	c.Assert(pods.Items[0], check.DeepEquals, apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-v1-deploy",
			Namespace: s.client.Namespace(),
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
			Image: "docker:1.11.2",
			VolumeMounts: []apiv1.VolumeMount{
				{Name: "dockersock", MountPath: dockerSockPath},
				{Name: "intercontainer", MountPath: buildIntercontainerPath},
			},
			TTY: true,
			Command: []string{
				"sh", "-ec",
				`
							end() { touch /tmp/intercontainer/done; }
							trap end EXIT
							while [ ! -f /tmp/intercontainer/status ]; do sleep 1; done
							exit_code=$(cat /tmp/intercontainer/status)
							[ "${exit_code}" != "0" ] && exit "${exit_code}"
							id=$(docker ps -aq -f "label=io.kubernetes.container.name=myapp-v1-deploy" -f "label=io.kubernetes.pod.name=$(hostname)")
							img="ip:destimg:v1"
							echo
							echo '---- Building application image ----'
							docker commit "${id}" "${img}" >/dev/null
							sz=$(docker history "${img}" | head -2 | tail -1 | grep -E -o '[0-9.]+\s[a-zA-Z]+\s*$' | sed 's/[[:space:]]*$//g')
							echo " ---> Sending image to repository (${sz})"
							` + `
							docker push ip:destimg:v1
						` + `

				docker tag ip:destimg:v1 ip:destimg:latest
				docker push ip:destimg:latest
			`,
			},
		},
		{
			Name:  "myapp-v1-deploy",
			Image: "myimg",
			Command: []string{"/bin/sh", "-lc", `
		cat >/dev/null && tsuru_unit_agent   myapp deploy-only
		exit_code=$?
		echo "${exit_code}" >/tmp/intercontainer/status
		[ "${exit_code}" != "0" ] && exit "${exit_code}"
		while [ ! -f /tmp/intercontainer/done ]; do sleep 1; done
	`},
			Stdin:     true,
			StdinOnce: true,
			Env:       []apiv1.EnvVar{{Name: "TSURU_HOST", Value: ""}},
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
	err = v.Save()
	c.Assert(err, check.IsNil)
	err = v.BindApp(a.GetName(), "/mnt", false)
	c.Assert(err, check.IsNil)
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.IsNil)
	dep, err := s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).Get("myapp-p1", metav1.GetOptions{})
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
	var rollbackObj *extensions.DeploymentRollback
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackObj = obj.(*extensions.DeploymentRollback)
			return true, rollbackObj, nil
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
	})
	c.Assert(err, check.ErrorMatches, "^timeout waiting full rollout after .+ waiting for units: Pod myapp-p1-pod-1-1: invalid pod phase \"Running\"$")
	c.Assert(rollbackObj, check.DeepEquals, &extensions.DeploymentRollback{
		Name: "myapp-p1",
	})
	c.Assert(buf.String(), check.Matches, `(?s).*---- Updating units \[p1\] ----.*ROLLING BACK AFTER FAILURE.*---> timeout waiting full rollout after .* waiting for units: Pod myapp-p1-pod-1-1: invalid pod phase \"Running\" <---\s*$`)
	cleanupDeployment(s.clusterClient, a, "p1")
	_, err = s.client.CoreV1().Events(s.client.Namespace()).Create(&apiv1.Event{
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
	c.Assert(err, check.ErrorMatches, "^timeout waiting full rollout after .+ waiting for units: Pod myapp-p1-pod-2-1: invalid pod phase \"Running\" - last event: my evt message$")
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
	var rollbackObj *extensions.DeploymentRollback
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackObj = obj.(*extensions.DeploymentRollback)
			return true, rollbackObj, nil
		}
		dep := obj.(*v1beta2.Deployment)
		dep.Status.UnavailableReplicas = 2
		dep.Status.ObservedGeneration = 12
		labelsCp := make(map[string]string, len(dep.Labels))
		for k, v := range dep.Spec.Template.Labels {
			labelsCp[k] = v
		}
		go func() {
			_, repErr := s.client.Clientset.AppsV1beta2().ReplicaSets(s.client.Namespace()).Create(&v1beta2.ReplicaSet{
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
		pod.OwnerReferences = append(pod.OwnerReferences, metav1.OwnerReference{
			Kind: "ReplicaSet",
			Name: "replica-for-myapp-p1",
		})
		return false, nil, nil
	})
	err = servicecommon.RunServicePipeline(&m, a, "myimg", servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	c.Assert(err, check.ErrorMatches, "^timeout waiting healthcheck after .+ waiting for units: Pod myapp-p1-pod-1-1: invalid pod phase \"Running\"$")
	c.Assert(rollbackObj, check.DeepEquals, &extensions.DeploymentRollback{
		Name: "myapp-p1",
	})
	c.Assert(buf.String(), check.Matches, `(?s).*---- Updating units \[p1\] ----.*ROLLING BACK AFTER FAILURE.*---> timeout waiting healthcheck after .* waiting for units: Pod myapp-p1-pod-1-1: invalid pod phase \"Running\" <---\s*$`)
	cleanupDeployment(s.clusterClient, a, "p1")
	_, err = s.client.CoreV1().Events(s.client.Namespace()).Create(&apiv1.Event{
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
	c.Assert(err, check.ErrorMatches, "^timeout waiting healthcheck after .+ waiting for units: Pod myapp-p1-pod-2-1: invalid pod phase \"Running\" - last event: my evt message$")
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
	var rollbackObj *extensions.DeploymentRollback
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackObj = obj.(*extensions.DeploymentRollback)
			return true, rollbackObj, nil
		}
		dep := obj.(*v1beta2.Deployment)
		dep.Status.UnavailableReplicas = 2
		dep.Status.ObservedGeneration = 12
		labelsCp := make(map[string]string, len(dep.Labels))
		for k, v := range dep.Spec.Template.Labels {
			labelsCp[k] = v
		}
		go func() {
			_, repErr := s.client.Clientset.AppsV1beta2().ReplicaSets(s.client.Namespace()).Create(&v1beta2.ReplicaSet{
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
	})
	c.Assert(err, check.ErrorMatches, "^timeout waiting full rollout after .+ waiting for units: Pod myapp-p1-pod-1-1: invalid pod phase \"Pending\"$")
	c.Assert(rollbackObj, check.DeepEquals, &extensions.DeploymentRollback{
		Name: "myapp-p1",
	})
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
		"tsuru.io/provisioner":          provisionerName,
		"tsuru.io/builder":              "",
	}
	_, err = s.client.Clientset.AppsV1beta2().ReplicaSets(s.client.Namespace()).Create(&v1beta2.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: s.client.Namespace(),
			Labels:    expectedLabels,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.CoreV1().Pods(s.client.Namespace()).Create(&apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: s.client.Namespace(),
			Labels:    expectedLabels,
		},
	})
	c.Assert(err, check.IsNil)
	err = m.RemoveService(a, "p1")
	c.Assert(err, check.IsNil)
	deps, err := s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	srvs, err := s.client.CoreV1().Services(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srvs.Items, check.HasLen, 0)
	pods, err := s.client.CoreV1().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.Clientset.AppsV1beta2().ReplicaSets(s.client.Namespace()).List(metav1.ListOptions{})
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
	err = servicecommon.RunServicePipeline(&m, a, "myimg", nil)
	c.Assert(err, check.IsNil)
	waitDep()
	s.client.PrependReactor("delete", "deployments", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("my dep err")
	})
	err = m.RemoveService(a, "p1")
	c.Assert(err, check.ErrorMatches, "(?s).*my dep err.*")
	deps, err := s.client.Clientset.AppsV1beta2().Deployments(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	srvs, err := s.client.CoreV1().Services(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(srvs.Items, check.HasLen, 0)
}
