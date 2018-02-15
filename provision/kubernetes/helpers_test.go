// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"k8s.io/api/apps/v1beta2"
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
		c.Assert(serviceAccountNameForApp(a), check.Equals, tt.expected, check.Commentf("test %d", i))
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
		c.Assert(serviceAccountNameForNodeContainer(nodecontainer.NodeContainerConfig{
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
	}
	for i, tt := range tests {
		a := provisiontest.NewFakeApp(tt.name, "plat", 1)
		c.Assert(deploymentNameForApp(a, tt.process), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestHeadlessServiceNameForApp(c *check.C) {
	var tests = []struct {
		name, process, expected string
	}{
		{"myapp", "p1", "myapp-p1-units"},
		{"MYAPP", "p-1", "myapp-p-1-units"},
		{"my-app_app", "P_1-1", "my-app-app-p-1-1-units"},
	}
	for i, tt := range tests {
		a := provisiontest.NewFakeApp(tt.name, "plat", 1)
		c.Assert(headlessServiceNameForApp(a, tt.process), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestDeployPodNameForApp(c *check.C) {
	var tests = []struct {
		name, expected string
	}{
		{"myapp", "myapp-v1-deploy"},
		{"MYAPP", "myapp-v1-deploy"},
		{"my-app_app", "my-app-app-v1-deploy"},
	}
	for i, tt := range tests {
		a := provisiontest.NewFakeApp(tt.name, "plat", 1)
		name, err := deployPodNameForApp(a)
		c.Assert(err, check.IsNil)
		c.Assert(name, check.Equals, tt.expected, check.Commentf("test %d", i))
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
		c.Assert(execCommandPodNameForApp(a), check.Equals, tt.expected, check.Commentf("test %d", i))
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
		c.Assert(daemonSetName(tt.name, tt.pool), check.Equals, tt.expected, check.Commentf("test %d", i))
	}
}

func (s *S) TestWaitFor(c *check.C) {
	err := waitFor(100*time.Millisecond, func() (bool, error) {
		return true, nil
	}, nil)
	c.Assert(err, check.IsNil)
	called := false
	err = waitFor(100*time.Millisecond, func() (bool, error) {
		return true, nil
	}, func() error {
		called = true
		return nil
	})
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, false)
	err = waitFor(100*time.Millisecond, func() (bool, error) {
		return false, nil
	}, nil)
	c.Assert(err, check.ErrorMatches, `timeout after .*`)
	err = waitFor(100*time.Millisecond, func() (bool, error) {
		return false, nil
	}, func() error {
		return errors.New("my error")
	})
	c.Assert(err, check.ErrorMatches, `timeout after .*?: my error$`)
	err = waitFor(100*time.Millisecond, func() (bool, error) {
		return false, nil
	}, func() error {
		return nil
	})
	c.Assert(err, check.ErrorMatches, `timeout after .*?: <nil>$`)
	err = waitFor(100*time.Millisecond, func() (bool, error) {
		return true, errors.New("myerr")
	}, nil)
	c.Assert(err, check.ErrorMatches, `myerr`)
}

func (s *S) TestWaitForPodContainersRunning(c *check.C) {
	err := waitForPodContainersRunning(s.client.ClusterClient, "pod1", 100*time.Millisecond)
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
		{phase: apiv1.PodPending, err: `timeout after .*`},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed"`},
		{phase: apiv1.PodUnknown, err: `invalid pod phase "Unknown"`},
		{phase: apiv1.PodRunning, states: []apiv1.ContainerState{
			{},
		}, err: `timeout after .*`},
		{phase: apiv1.PodRunning, states: []apiv1.ContainerState{
			{Running: &apiv1.ContainerStateRunning{}}, {},
		}, err: `timeout after .*`},
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
		_, err = s.client.CoreV1().Pods(s.client.Namespace()).Create(&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: s.client.Namespace(),
			},
		})
		c.Assert(err, check.IsNil)
		err = waitForPodContainersRunning(s.client.ClusterClient, "pod1", 100*time.Millisecond)
		if tt.err == "" {
			c.Assert(err, check.IsNil)
		} else {
			c.Assert(err, check.ErrorMatches, tt.err)
		}
		err = cleanupPod(s.client.ClusterClient, "pod1")
		c.Assert(err, check.IsNil)
	}
}

func (s *S) TestWaitForPod(c *check.C) {
	srv, wg := s.createDeployReadyServer(c)
	s.mockfakeNodes(c, srv.URL)
	defer srv.Close()
	defer wg.Wait()
	err := waitForPod(s.client.ClusterClient, "pod1", false, 100*time.Millisecond)
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
	s.logHook = func(w io.Writer, r *http.Request) {
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
		{phase: apiv1.PodRunning, err: `timeout after .*`},
		{phase: apiv1.PodRunning, running: true},
		{phase: apiv1.PodPending, err: `timeout after .*`},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed"`},
		{phase: apiv1.PodFailed, msg: "my error msg", err: `invalid pod phase "Failed"\("my error msg"\)`},
		{phase: apiv1.PodUnknown, err: `invalid pod phase "Unknown"`},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed" - last event: my evt message`, evt: &apiv1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1.evt1",
				Namespace: s.client.Namespace(),
			},
			InvolvedObject: apiv1.ObjectReference{
				Kind:      "Pod",
				Name:      "pod1",
				Namespace: s.client.Namespace(),
			},
			Message: "my evt message",
		}},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed" - log: my log error`, containers: []apiv1.Container{
			{Name: "cont1"},
		}},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed" - last event: my evt with log - log: my log error`, containers: []apiv1.Container{
			{Name: "cont1"},
		}, evt: &apiv1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1.evt1",
				Namespace: s.client.Namespace(),
			},
			InvolvedObject: apiv1.ObjectReference{
				Kind:      "Pod",
				Name:      "pod1",
				Namespace: s.client.Namespace(),
			},
			Message: "my evt with log",
		}},
	}
	for _, tt := range tests {
		wantedPhase = tt.phase
		wantedMessage = tt.msg
		pod := &apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: s.client.Namespace(),
			},
		}
		if len(tt.containers) > 0 {
			pod.Spec.Containers = tt.containers
		}
		_, err = s.client.CoreV1().Pods(s.client.Namespace()).Create(pod)
		c.Assert(err, check.IsNil)
		if tt.evt != nil {
			_, err = s.client.CoreV1().Events(s.client.Namespace()).Create(tt.evt)
			c.Assert(err, check.IsNil)
		}
		err = waitForPod(s.client.ClusterClient, "pod1", tt.running, 100*time.Millisecond)
		if tt.err == "" {
			c.Assert(err, check.IsNil)
		} else {
			c.Assert(err, check.ErrorMatches, tt.err)
		}
		err = cleanupPod(s.client.ClusterClient, "pod1")
		c.Assert(err, check.IsNil)
		if tt.evt != nil {
			err = s.client.CoreV1().Events(s.client.Namespace()).Delete(tt.evt.Name, nil)
			c.Assert(err, check.IsNil)
		}
	}
}

func (s *S) TestCleanupPods(c *check.C) {
	for i := 0; i < 3; i++ {
		labels := map[string]string{"a": "x"}
		if i == 2 {
			labels["a"] = "y"
		}
		_, err := s.client.CoreV1().Pods(s.client.Namespace()).Create(&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: s.client.Namespace(),
				Labels:    labels,
			},
		})
		c.Assert(err, check.IsNil)
	}
	err := cleanupPods(s.client.ClusterClient, metav1.ListOptions{
		LabelSelector: "a=x",
	})
	c.Assert(err, check.IsNil)
	pods, err := s.client.CoreV1().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.DeepEquals, []apiv1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: s.client.Namespace(),
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
	_, err := s.client.AppsV1beta2().Deployments(s.client.Namespace()).Create(&v1beta2.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: s.client.Namespace(),
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.AppsV1beta2().ReplicaSets(s.client.Namespace()).Create(&v1beta2.ReplicaSet{
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
	err = cleanupDeployment(s.client.ClusterClient, a, "p1")
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1beta2().Deployments(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	pods, err := s.client.CoreV1().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.AppsV1beta2().ReplicaSets(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestCleanupReplicas(c *check.C) {
	_, err := s.client.AppsV1beta2().ReplicaSets(s.client.Namespace()).Create(&v1beta2.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: s.client.Namespace(),
			Labels: map[string]string{
				"a": "x",
			},
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.CoreV1().Pods(s.client.Namespace()).Create(&apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: s.client.Namespace(),
			Labels: map[string]string{
				"a": "x",
			},
		},
	})
	c.Assert(err, check.IsNil)
	err = cleanupReplicas(s.client.ClusterClient, metav1.ListOptions{
		LabelSelector: "a=x",
	})
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1beta2().Deployments(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	pods, err := s.client.CoreV1().Pods(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.AppsV1beta2().ReplicaSets(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestCleanupDaemonSet(c *check.C) {
	_, err := s.client.AppsV1beta2().DaemonSets(s.client.Namespace()).Create(&v1beta2.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-container-bs-pool-p1",
			Namespace: s.client.Namespace(),
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.CoreV1().Pods(s.client.Namespace()).Create(&apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-container-bs-pool-p1-xyz",
			Namespace: s.client.Namespace(),
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
	err = cleanupDaemonSet(s.client.ClusterClient, "bs", "p1")
	c.Assert(err, check.IsNil)
	daemons, err := s.client.AppsV1beta2().DaemonSets(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 0)
	pods, err := s.client.CoreV1().Pods(s.client.Namespace()).List(metav1.ListOptions{})
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
	port, err := getServicePort(s.client.ClusterClient, "notfound")
	c.Assert(err, check.IsNil)
	c.Assert(port, check.Equals, int32(0))
	_, err = s.client.CoreV1().Services(s.client.Namespace()).Create(&apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "srv1",
			Namespace: s.client.Namespace(),
		},
	})
	c.Assert(err, check.IsNil)
	port, err = getServicePort(s.client.ClusterClient, "srv1")
	c.Assert(err, check.IsNil)
	c.Assert(port, check.Equals, int32(0))
	_, err = s.client.CoreV1().Services(s.client.Namespace()).Create(&apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "srv2",
			Namespace: s.client.Namespace(),
		},
		Spec: apiv1.ServiceSpec{
			Ports: []apiv1.ServicePort{{NodePort: 123}},
		},
	})
	c.Assert(err, check.IsNil)
	port, err = getServicePort(s.client.ClusterClient, "srv2")
	c.Assert(err, check.IsNil)
	c.Assert(port, check.Equals, int32(123))
}
