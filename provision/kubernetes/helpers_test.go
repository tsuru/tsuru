// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestServiceAccountNameForApp(c *check.C) {
	var tests = []struct {
		name, expected string
	}{
		{"myapp", "app-myapp"},
		{"MYAPP", "app-myapp"},
		{"my-app_app", "app-my-app-app"},
	}
	for i, tt := range tests {
		a := provisiontest.NewFakeApp(tt.name, "plat", 1)
		c.Check(serviceAccountNameForApp(a), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestServiceAccountNameForNodeContainer(c *check.C) {
	var tests = []struct {
		name, expected string
	}{
		{"mync", "node-container-mync"},
		{"MYNC", "node-container-mync"},
		{"my-nc_nc", "node-container-my-nc-nc"},
	}
	for i, tt := range tests {
		c.Check(serviceAccountNameForNodeContainer(nodecontainer.NodeContainerConfig{
			Name: tt.name,
		}), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestDeploymentNameForApp(c *check.C) {
	var tests = []struct {
		name, process, expected string
	}{
		{"myapp", "p1", "myapp-p1"},
		{"MYAPP", "p-1", "myapp-p-1"},
		{"my-app_app", "P_1-1", "my-app-app-p-1-1"},
		{"app-with-a-very-very-long-name", "p1", "app-with-a-very-very-long-name-p1"},
		{"my-app", "process-with-a-very-very-long-name-1234567890123", "my-app-process-with-a-very-very-long-name-1234567890123"},
		{"my-app", "process-with-a-very-very-long-name-12345678901234", "my-app-0718ca0d56b1219fb50636a73252a47b977839e983558e08"},
		{"app-with-a-very-very-long-name", "process-with-a-very-very-long-name", "app-with-a-very-very-long-name-a9101bf0964e84e3f4c4b2b0"},
	}
	for i, tt := range tests {
		a := provisiontest.NewFakeApp(tt.name, "plat", 1)
		c.Check(deploymentNameForApp(a, tt.process), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestHeadlessServiceNameForApp(c *check.C) {
	var tests = []struct {
		name, process, expected string
	}{
		{"myapp", "p1", "myapp-p1-units"},
		{"MYAPP", "p-1", "myapp-p-1-units"},
		{"my-app_app", "P_1-1", "my-app-app-p-1-1-units"},
		{"app-with-a-very-very-long-name", "p1", "app-with-a-very-very-long-name-p1-units"},
		{"my-app", "process-with-a-very-very-long-name-1234567890123", "my-app-process-with-a-very-very-long-name-1234567890123-units"},
		{"my-app", "process-with-a-very-very-long-name-12345678901234", "my-app-0718ca0d56b1219fb50636a73252a47b977839e983558e08-units"},
		{"app-with-a-very-very-long-name", "process-with-a-very-very-long-name", "app-with-a-very-very-long-name-a9101bf0964e84e3f4c4b2b0-units"},
	}
	for i, tt := range tests {
		a := provisiontest.NewFakeApp(tt.name, "plat", 1)
		c.Check(headlessServiceNameForApp(a, tt.process), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestDeployPodNameForApp(c *check.C) {
	var tests = []struct {
		name, expected string
	}{
		{"myapp", "myapp-v1-deploy"},
		{"MYAPP", "myapp-v1-deploy"},
		{"my-app_app", "my-app-app-v1-deploy"},
		{"myapp", "myapp-v2-deploy"},
	}
	for i, tt := range tests {
		fakeApp := provisiontest.NewFakeApp(tt.name, "python", 0)
		version, err := servicemanager.AppVersion.NewAppVersion(appTypes.NewVersionArgs{
			App: fakeApp,
		})
		c.Assert(err, check.IsNil)
		a := provisiontest.NewFakeApp(tt.name, "plat", 1)
		name := deployPodNameForApp(a, version)
		c.Check(name, check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestExecCommandPodNameForApp(c *check.C) {
	var tests = []struct {
		name, expected string
	}{
		{"myapp", "myapp-isolated-run"},
		{"MYAPP", "myapp-isolated-run"},
		{"my-app_app", "my-app-app-isolated-run"},
	}
	for i, tt := range tests {
		a := provisiontest.NewFakeApp(tt.name, "plat", 1)
		c.Check(execCommandPodNameForApp(a), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestDaemonSetName(c *check.C) {
	var tests = []struct {
		name, pool, expected string
	}{
		{"d1", "", "node-container-d1-all"},
		{"D1", "", "node-container-d1-all"},
		{"d1_x", "", "node-container-d1-x-all"},
		{"d1", "p1", "node-container-d1-pool-p1"},
		{"d1", "P1", "node-container-d1-pool-p1"},
		{"d1", "P_1", "node-container-d1-pool-p-1"},
		{"d1", "P-x_1", "node-container-d1-pool-p-x-1"},
	}
	for i, tt := range tests {
		c.Check(daemonSetName(tt.name, tt.pool), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestRegistrySecretName(c *check.C) {
	var tests = []struct {
		name, expected string
	}{
		{"registry.tsuru.io", "registry-registry.tsuru.io"},
		{"my-registry", "registry-my-registry"},
	}
	for i, tt := range tests {
		c.Check(registrySecretName(tt.name), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestAppLabelForApp(c *check.C) {
	var tests = []struct {
		name, process, expected string
	}{
		{"myapp", "p1", "myapp-p1"},
		{"MYAPP", "p-1", "myapp-p-1"},
		{"my-app_app", "P_1-1", "my-app-app-p-1-1"},
		{"app-with-a-very-very-long-name", "p1", "app-with-a-very-very-long-name-p1"},
		{"my-app", "process-with-a-very-very-long-name-1234567890123", "my-app-process-with-a-very-very-long-name-1234567890123"},
		{"my-app", "process-with-a-very-very-long-name-12345678901234", "my-app-0718ca0d56b1219fb50636a73252a47b977839e983558e08"},
		{"app-with-a-very-very-long-name", "process-with-a-very-very-long-name", "app-with-a-very-very-long-name-a9101bf0964e84e3f4c4b2b0"},
	}
	for i, tt := range tests {
		a := provisiontest.NewFakeApp(tt.name, "plat", 1)
		c.Check(appLabelForApp(a, tt.process), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestWaitFor(c *check.C) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	err := waitFor(ctx, func() (bool, error) {
		return true, nil
	}, nil)
	cancel()
	c.Assert(err, check.IsNil)
	called := false
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = waitFor(ctx, func() (bool, error) {
		return true, nil
	}, func() error {
		called = true
		return nil
	})
	cancel()
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, false)
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = waitFor(ctx, func() (bool, error) {
		return false, nil
	}, nil)
	cancel()
	c.Assert(err, check.ErrorMatches, `canceled after .*`)
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = waitFor(ctx, func() (bool, error) {
		return false, nil
	}, func() error {
		return errors.New("my error")
	})
	cancel()
	c.Assert(err, check.ErrorMatches, `canceled after .*?: my error: context deadline exceeded$`)
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = waitFor(ctx, func() (bool, error) {
		return false, nil
	}, func() error {
		return nil
	})
	cancel()
	c.Assert(err, check.ErrorMatches, `canceled after .*?: <nil>: context deadline exceeded$`)
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = waitFor(ctx, func() (bool, error) {
		return true, errors.New("myerr")
	}, nil)
	cancel()
	c.Assert(err, check.ErrorMatches, `myerr`)
}

func (s *S) TestWaitForPodContainersRunning(c *check.C) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	ns := "default"
	err := waitForPodContainersRunning(ctx, s.clusterClient, &apiv1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}}, ns)
	cancel()
	c.Assert(err, check.ErrorMatches, `.*"pod1" not found`)
	var wantedPhase apiv1.PodPhase
	var wantedStates []apiv1.ContainerState
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(ok, check.Equals, true)
		pod.Status.Phase = wantedPhase
		statuses := make([]apiv1.ContainerStatus, len(wantedStates))
		for i, s := range wantedStates {
			statuses[i] = apiv1.ContainerStatus{Name: fmt.Sprintf("c-%d", i), State: s}
		}
		pod.Status.ContainerStatuses = statuses
		return false, nil, nil
	})
	tests := []struct {
		states []apiv1.ContainerState
		phase  apiv1.PodPhase
		err    string
	}{
		{phase: apiv1.PodSucceeded},
		{phase: apiv1.PodPending, err: `canceled after .*`},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed"`},
		{phase: apiv1.PodUnknown, err: `invalid pod phase "Unknown"`},
		{phase: apiv1.PodRunning, states: []apiv1.ContainerState{
			{},
		}, err: `canceled after .*`},
		{phase: apiv1.PodRunning, states: []apiv1.ContainerState{
			{Running: &apiv1.ContainerStateRunning{}}, {},
		}, err: `canceled after .*`},
		{phase: apiv1.PodRunning, states: []apiv1.ContainerState{
			{Running: &apiv1.ContainerStateRunning{}}, {Running: &apiv1.ContainerStateRunning{}},
		}},
		{phase: apiv1.PodRunning, states: []apiv1.ContainerState{
			{Running: &apiv1.ContainerStateRunning{}}, {Terminated: &apiv1.ContainerStateTerminated{
				ExitCode: 9, Reason: "x", Message: "y",
			}},
		}, err: `unexpected container "c-1" termination: Exit 9 - Reason: "x" - Message: "y"`},
	}
	for _, tt := range tests {
		wantedPhase = tt.phase
		wantedStates = tt.states
		podObj := &apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: ns,
			},
		}
		_, err = s.client.CoreV1().Pods(ns).Create(podObj)
		c.Assert(err, check.IsNil)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err = waitForPodContainersRunning(ctx, s.clusterClient, podObj, ns)
		cancel()
		if tt.err == "" {
			c.Assert(err, check.IsNil)
		} else {
			c.Assert(err, check.ErrorMatches, tt.err)
		}
		err = cleanupPod(s.clusterClient, "pod1", ns)
		c.Assert(err, check.IsNil)
	}
}

func (s *S) TestWaitForPod(c *check.C) {
	srv, wg := s.mock.CreateDeployReadyServer(c)
	s.mock.MockfakeNodes(c, srv.URL)
	defer srv.Close()
	defer wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	ns := "default"
	err := waitForPod(ctx, s.clusterClient, &apiv1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}}, ns, false)
	cancel()
	c.Assert(err, check.ErrorMatches, `.*"pod1" not found`)
	var wantedPhase apiv1.PodPhase
	var wantedMessage string
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(ok, check.Equals, true)
		pod.Status.Phase = wantedPhase
		pod.Status.Message = wantedMessage
		return false, nil, nil
	})
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		w.Write([]byte(`my log error`))
	}
	tests := []struct {
		phase      apiv1.PodPhase
		containers []apiv1.Container
		msg        string
		err        string
		evt        *apiv1.Event
		running    bool
	}{
		{phase: apiv1.PodSucceeded},
		{phase: apiv1.PodRunning, err: `canceled after .*`},
		{phase: apiv1.PodRunning, running: true},
		{phase: apiv1.PodPending, err: `canceled after .*`},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed"`},
		{phase: apiv1.PodFailed, msg: "my error msg", err: `invalid pod phase "Failed"\("my error msg"\)`},
		{phase: apiv1.PodUnknown, err: `invalid pod phase "Unknown"`},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed" - last event: my evt message`, evt: &apiv1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1.evt1",
				Namespace: ns,
			},
			InvolvedObject: apiv1.ObjectReference{
				Kind:      "Pod",
				Name:      "pod1",
				Namespace: ns,
			},
			Message: "my evt message",
		}},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed"`, containers: []apiv1.Container{
			{Name: "cont1"},
		}},
	}
	for i, tt := range tests {
		c.Logf("test %d", i)
		wantedPhase = tt.phase
		wantedMessage = tt.msg
		pod := &apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: ns,
			},
		}
		if len(tt.containers) > 0 {
			pod.Spec.Containers = tt.containers
		}
		_, err = s.client.CoreV1().Pods(ns).Create(pod)
		c.Assert(err, check.IsNil)
		if tt.evt != nil {
			_, err = s.client.CoreV1().Events(ns).Create(tt.evt)
			c.Assert(err, check.IsNil)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err = waitForPod(ctx, s.clusterClient, pod, ns, tt.running)
		cancel()
		if tt.err == "" {
			c.Assert(err, check.IsNil)
		} else {
			c.Assert(err, check.ErrorMatches, tt.err)
		}
		err = cleanupPod(s.clusterClient, "pod1", ns)
		c.Assert(err, check.IsNil)
		if tt.evt != nil {
			err = s.client.CoreV1().Events(ns).Delete(tt.evt.Name, nil)
			c.Assert(err, check.IsNil)
		}
	}
}

func (s *S) TestCleanupPods(c *check.C) {
	ns := "default"
	for i := 0; i < 3; i++ {
		labels := map[string]string{"a": "x"}
		if i == 2 {
			labels["a"] = "y"
		}
		_, err := s.client.CoreV1().Pods(ns).Create(&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: ns,
				Labels:    labels,
			},
		})
		c.Assert(err, check.IsNil)
	}
	err := cleanupPods(s.clusterClient, metav1.ListOptions{
		LabelSelector: "a=x",
	}, ns)
	c.Assert(err, check.IsNil)
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.DeepEquals, []apiv1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: ns,
			Labels:    map[string]string{"a": "y"},
		},
	}})
}

func (s *S) TestCleanupDeployment(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	expectedLabels := map[string]string{
		"tsuru.io/is-tsuru":             "true",
		"tsuru.io/is-service":           "true",
		"tsuru.io/is-build":             "false",
		"tsuru.io/is-stopped":           "false",
		"tsuru.io/is-deploy":            "false",
		"tsuru.io/is-isolated-run":      "false",
		"tsuru.io/restarts":             "0",
		"tsuru.io/app-name":             "myapp",
		"tsuru.io/app-process":          "p1",
		"tsuru.io/app-process-replicas": "1",
		"tsuru.io/app-platform":         "plat",
		"tsuru.io/app-pool":             "test-default",
		"tsuru.io/router-type":          "fake",
		"tsuru.io/router-name":          "fake",
		"tsuru.io/provisioner":          "kubernetes",
	}
	err := s.p.Provision(a)
	c.Assert(err, check.IsNil)
	ns, err := s.client.AppNamespace(a)
	c.Assert(err, check.IsNil)
	_, err = s.client.AppsV1().Deployments(ns).Create(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: ns,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.AppsV1().ReplicaSets(ns).Create(&appsv1.ReplicaSet{
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
	err = cleanupDeployment(s.clusterClient, a, "p1")
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1().Deployments(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.AppsV1().ReplicaSets(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestCleanupReplicas(c *check.C) {
	ns := "tsuru_pool"
	_, err := s.client.AppsV1().ReplicaSets(ns).Create(&appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: ns,
			Labels: map[string]string{
				"a": "x",
			},
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.CoreV1().Pods(ns).Create(&apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: ns,
			Labels: map[string]string{
				"a": "x",
			},
		},
	})
	c.Assert(err, check.IsNil)
	err = cleanupReplicas(s.clusterClient, metav1.ListOptions{
		LabelSelector: "a=x",
	}, ns)
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1().Deployments(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.AppsV1().ReplicaSets(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestCleanupDaemonSet(c *check.C) {
	ns := s.client.PoolNamespace("pool")
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
	err = cleanupDaemonSet(s.clusterClient, "bs", "p1")
	c.Assert(err, check.IsNil)
	daemons, err := s.client.AppsV1().DaemonSets(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 0)
	pods, err := s.client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
}

func (s *S) TestLabelSetFromMeta(c *check.C) {
	meta := metav1.ObjectMeta{
		Labels: map[string]string{
			"tsuru.io/x": "a",
			"y":          "b",
		},
		Annotations: map[string]string{
			"tsuru.io/a": "1",
			"b":          "2",
		},
	}
	ls := labelSetFromMeta(&meta)
	c.Assert(ls, check.DeepEquals, &provision.LabelSet{
		Labels: map[string]string{
			"tsuru.io/x": "a",
			"y":          "b",
			"tsuru.io/a": "1",
			"b":          "2",
		},
		Prefix: tsuruLabelPrefix,
	})
}

func (s *S) TestGetServicePort(c *check.C) {
	ns := "default"
	controller, err := getClusterController(s.p, s.clusterClient)
	c.Assert(err, check.IsNil)
	svcInformer, err := controller.getServiceInformer()
	c.Assert(err, check.IsNil)
	port, err := getServicePort(svcInformer, "notfound", ns)
	c.Assert(err, check.IsNil)
	c.Assert(port, check.Equals, int32(0))
	svc := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "srv1",
			Namespace: ns,
		},
	}
	_, err = s.client.CoreV1().Services(ns).Create(svc)
	c.Assert(err, check.IsNil)
	err = s.factory.Core().V1().Services().Informer().GetStore().Add(svc)
	c.Assert(err, check.IsNil)
	port, err = getServicePort(svcInformer, "srv1", ns)
	c.Assert(err, check.IsNil)
	c.Assert(port, check.Equals, int32(0))
	svc = &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "srv2",
			Namespace: ns,
		},
		Spec: apiv1.ServiceSpec{
			Ports: []apiv1.ServicePort{{NodePort: 123}},
		},
	}
	_, err = s.client.CoreV1().Services(ns).Create(svc)
	c.Assert(err, check.IsNil)
	err = s.factory.Core().V1().Services().Informer().GetStore().Add(svc)
	c.Assert(err, check.IsNil)
	port, err = getServicePort(svcInformer, "srv2", ns)
	c.Assert(err, check.IsNil)
	c.Assert(port, check.Equals, int32(123))
}

func (s *S) TestGetServicePorts(c *check.C) {
	ns := "default"
	controller, err := getClusterController(s.p, s.clusterClient)
	c.Assert(err, check.IsNil)
	svcInformer, err := controller.getServiceInformer()
	c.Assert(err, check.IsNil)
	ports, err := getServicePorts(svcInformer, "notfound", ns)
	c.Assert(err, check.IsNil)
	c.Assert(ports, check.HasLen, 0)
	svc := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "srv1",
			Namespace: ns,
		},
		Spec: apiv1.ServiceSpec{
			Ports: []apiv1.ServicePort{{NodePort: 123}, {NodePort: 456}},
		},
	}
	_, err = s.client.CoreV1().Services(ns).Create(svc)
	c.Assert(err, check.IsNil)
	err = s.factory.Core().V1().Services().Informer().GetStore().Add(svc)
	c.Assert(err, check.IsNil)
	ports, err = getServicePorts(svcInformer, "srv1", ns)
	c.Assert(err, check.IsNil)
	c.Assert(ports, check.DeepEquals, []int32{123, 456})
}
