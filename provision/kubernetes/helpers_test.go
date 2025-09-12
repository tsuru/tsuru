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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	check "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func TestServiceAccountNameForApp(t *testing.T) {
	tests := []struct {
		name, expected string
	}{
		{"myapp", "app-myapp"},
		{"MYAPP", "app-myapp"},
		{"my-app_app", "app-my-app-app"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			a := provisiontest.NewFakeApp(tt.name, "plat", 1)
			require.Equal(t, tt.expected, serviceAccountNameForApp(a))
		})
	}
}

func TestDeploymentNameForAppBase(t *testing.T) {
	tests := []struct {
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
		t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			a := provisiontest.NewFakeApp(tt.name, "plat", 1)
			require.Equal(t, tt.expected, deploymentNameForAppBase(a, tt.process))
		})
	}
}

func TestDeploymentNameForApp(t *testing.T) {
	tests := []struct {
		name, process string
		version       int
		expected      string
	}{
		{"myapp", "p1", 1, "myapp-p1-v1"},
		{"MYAPP", "p-1", 9, "myapp-p-1-v9"},
		{"my-app_app", "P_1-1", 2, "my-app-app-p-1-1-v2"},
		{"app-with-a-very-very-long-name", "p1", 5, "app-with-a-very-very-long-name-p1-v5"},
		{"my-app", "process-with-a-very-very-long-name-1234567890", 5, "my-app-process-with-a-very-very-long-name-1234567890-v5"},
		{"my-app", "process-with-a-very-very-long-name-123456789", 12, "my-app-process-with-a-very-very-long-name-123456789-v12"},
		{"my-app", "process-with-a-very-very-long-name-1234567890", 12, "my-app-e4290187b8296fb015e9ba4803b102487f966040a0f995f4"},
		{"app-with-a-very-very-long-name", "process-with-a-very-very-long-name", 5, "app-with-a-very-very-long-name-fc2ee6e1b0ba94ee2bbfbacf"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			a := provisiontest.NewFakeApp(tt.name, "plat", 1)
			require.Equal(t, tt.expected, deploymentNameForApp(a, tt.process, tt.version))
		})
	}
}

func TestHeadlessServiceName(t *testing.T) {
	tests := []struct {
		name, process, expected string
	}{
		{"myapp", "p1", "myapp-p1-units"},
		{"MYAPP", "p-1", "myapp-p-1-units"},
		{"my-app_app", "P_1-1", "my-app-app-p-1-1-units"},
		{"app-with-a-very-very-long-name", "p1", "app-with-a-very-very-long-name-p1-units"},
		{"my-app", "process-with-a-very-very-long-name-1234567", "my-app-process-with-a-very-very-long-name-1234567-units"},
		{"my-app", "process-with-a-very-very-long-name-12345678", "my-app-8a923e03a0da9ec6e611490063d0a47f8ca3dd67fa6cdd93"},
		{"app-with-a-very-very-long-name", "process-with-a-very-very-long-name", "app-with-a-very-very-long-name-91b0cf4eb3ea4241ee2e84ab"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			a := provisiontest.NewFakeApp(tt.name, "plat", 1)
			require.Equal(t, tt.expected, headlessServiceName(a, tt.process))
		})
	}
}

func TestExecCommandPodNameForApp(t *testing.T) {
	tests := []struct {
		name, expected string
	}{
		{"myapp", "myapp-isolated-run"},
		{"MYAPP", "myapp-isolated-run"},
		{"my-app_app", "my-app-app-isolated-run"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			a := provisiontest.NewFakeApp(tt.name, "plat", 1)
			require.Equal(t, tt.expected, execCommandPodNameForApp(a))
		})
	}
}

func TestWaitFor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	err := waitFor(ctx, func() (bool, error) {
		return true, nil
	}, nil)
	cancel()
	require.NoError(t, err)
	called := false
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = waitFor(ctx, func() (bool, error) {
		return true, nil
	}, func() error {
		called = true
		return nil
	})
	cancel()
	require.NoError(t, err)
	require.False(t, called)
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = waitFor(ctx, func() (bool, error) {
		return false, nil
	}, nil)
	cancel()
	require.ErrorContains(t, err, `canceled after`)
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = waitFor(ctx, func() (bool, error) {
		return false, nil
	}, func() error {
		return errors.New("my error")
	})
	cancel()
	require.ErrorContains(t, err, "canceled after")
	require.ErrorContains(t, err, "my error: context deadline exceeded")
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = waitFor(ctx, func() (bool, error) {
		return false, nil
	}, func() error {
		return nil
	})
	cancel()
	require.ErrorContains(t, err, "canceled after")
	require.ErrorContains(t, err, "<nil>: context deadline exceeded")
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = waitFor(ctx, func() (bool, error) {
		return true, errors.New("myerr")
	}, nil)
	cancel()
	require.ErrorContains(t, err, `myerr`)
}

func (s *S) TestWaitForPodContainersRunning(_ *check.C) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	ns := "default"
	err := waitForPodContainersRunning(ctx, s.clusterClient, &apiv1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}}, ns)
	cancel()
	require.ErrorContains(s.t, err, `"pod1" not found`)
	var wantedPhase apiv1.PodPhase
	var wantedStates []apiv1.ContainerState
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		require.True(s.t, ok)
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
		{phase: apiv1.PodPending, err: "canceled after"},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed"`},
		{phase: apiv1.PodUnknown, err: `invalid pod phase "Unknown"`},
		{phase: apiv1.PodRunning, states: []apiv1.ContainerState{
			{},
		}, err: "canceled after"},
		{phase: apiv1.PodRunning, states: []apiv1.ContainerState{
			{Running: &apiv1.ContainerStateRunning{}}, {},
		}, err: "canceled after"},
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
		_, err = s.client.CoreV1().Pods(ns).Create(context.TODO(), podObj, metav1.CreateOptions{})
		require.NoError(s.t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err = waitForPodContainersRunning(ctx, s.clusterClient, podObj, ns)
		cancel()
		if tt.err == "" {
			require.NoError(s.t, err)
		} else {
			require.ErrorContains(s.t, err, tt.err)
		}
		err = cleanupPod(context.TODO(), s.clusterClient, "pod1", ns)
		require.NoError(s.t, err)
	}
}

func (s *S) TestWaitForPod(c *check.C) {
	srv := s.mock.CreateDeployReadyServer(c)
	s.mock.MockfakeNodes(srv.URL)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	ns := "default"
	err := waitForPod(ctx, s.clusterClient, &apiv1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}}, ns, false)
	cancel()
	require.ErrorContains(s.t, err, `"pod1" not found`)
	var wantedPhase apiv1.PodPhase
	var wantedMessage string
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		require.True(s.t, ok)
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
		{phase: apiv1.PodRunning, err: "canceled after "},
		{phase: apiv1.PodRunning, running: true},
		{phase: apiv1.PodPending, err: "canceled after "},
		{phase: apiv1.PodFailed, err: `invalid pod phase "Failed"`},
		{phase: apiv1.PodFailed, msg: "my error msg", err: `invalid pod phase "Failed"("my error msg")`},
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
		_, err = s.client.CoreV1().Pods(ns).Create(context.TODO(), pod, metav1.CreateOptions{})
		require.NoError(s.t, err)
		if tt.evt != nil {
			_, err = s.client.CoreV1().Events(ns).Create(context.TODO(), tt.evt, metav1.CreateOptions{})
			require.NoError(s.t, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err = waitForPod(ctx, s.clusterClient, pod, ns, tt.running)
		cancel()
		if tt.err == "" {
			require.NoError(s.t, err)
		} else {
			require.ErrorContains(s.t, err, tt.err)
		}
		err = cleanupPod(context.TODO(), s.clusterClient, "pod1", ns)
		require.NoError(s.t, err)
		if tt.evt != nil {
			err = s.client.CoreV1().Events(ns).Delete(context.TODO(), tt.evt.Name, metav1.DeleteOptions{})
			require.NoError(s.t, err)
		}
	}
}

func (s *S) TestCleanupPods(_ *check.C) {
	ns := "default"
	rs, err := s.client.AppsV1().ReplicaSets(ns).Create(context.TODO(), &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs1",
			Namespace: ns,
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	controllerKind := appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	for i := 0; i < 3; i++ {
		labels := map[string]string{"a": "x"}
		if i == 2 {
			labels["a"] = "y"
		}
		_, err = s.client.CoreV1().Pods(ns).Create(context.TODO(), &apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: ns,
				Labels:    labels,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(rs, controllerKind),
				},
			},
		}, metav1.CreateOptions{})
		require.NoError(s.t, err)
	}
	err = cleanupPods(context.TODO(), s.clusterClient, metav1.ListOptions{
		LabelSelector: "a=x",
	}, rs)
	require.NoError(s.t, err)
	pods, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, []apiv1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: ns,
			Labels:    map[string]string{"a": "y"},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(rs, controllerKind),
			},
		},
	}}, pods.Items)
}

func (s *S) TestCleanupDeployment(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	version := newCommittedVersion(c, a, map[string][]string{"p1": {"cm1"}})
	expectedLabels := map[string]string{
		"tsuru.io/is-tsuru":        "true",
		"tsuru.io/is-service":      "true",
		"tsuru.io/is-build":        "false",
		"tsuru.io/is-stopped":      "false",
		"tsuru.io/is-isolated-run": "false",
		"tsuru.io/restarts":        "0",
		"tsuru.io/app-name":        "myapp",
		"tsuru.io/app-process":     "p1",
		"tsuru.io/app-platform":    "plat",
		"tsuru.io/app-pool":        "test-default",
		"tsuru.io/app-version":     "1",
	}
	err := s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.AppsV1().Deployments(ns).Create(context.TODO(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: ns,
			Labels:    expectedLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: expectedLabels,
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"tsuru.io/app-name":        "myapp",
					"tsuru.io/app-process":     "p1",
					"tsuru.io/app-version":     "1",
					"tsuru.io/is-build":        "false",
					"tsuru.io/is-isolated-run": "false",
				},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)

	_, err = s.client.CoreV1().Secrets(ns).Create(context.TODO(), &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appSecretPrefix + "myapp-p1",
			Namespace: ns,
			Labels:    expectedLabels,
		},
		Data: map[string][]byte{
			"TSURU_SERVICES": []byte(`{}`),
			"MY_ENV":         []byte("myvalue"),
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	rs, err := s.client.AppsV1().ReplicaSets(ns).Create(context.TODO(), &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: ns,
			Labels:    expectedLabels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(dep, appsv1.SchemeGroupVersion.WithKind("Deployment")),
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Pods(ns).Create(context.TODO(), &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: ns,
			Labels:    expectedLabels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(rs, appsv1.SchemeGroupVersion.WithKind("Deployment")),
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	err = cleanupDeployment(context.TODO(), s.clusterClient, a, "p1", version.Version())
	require.NoError(s.t, err)
	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, deps.Items, 0)
	secrets, err := s.client.CoreV1().Secrets(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, secrets.Items, 0)
	pods, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pods.Items, 0)
	replicas, err := s.client.AppsV1().ReplicaSets(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, replicas.Items, 0)
}

func (s *S) TestCleanupReplicas(_ *check.C) {
	ns := "tsuru_pool"
	dep, err := s.client.AppsV1().Deployments(ns).Create(context.TODO(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp",
			Namespace: ns,
			Labels: map[string]string{
				"a": "x",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "x",
				},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	rs, err := s.client.AppsV1().ReplicaSets(ns).Create(context.TODO(), &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: ns,
			Labels: map[string]string{
				"a": "x",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(dep, appsv1.SchemeGroupVersion.WithKind("Deployment")),
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Pods(ns).Create(context.TODO(), &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: ns,
			Labels: map[string]string{
				"a": "x",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(rs, appsv1.SchemeGroupVersion.WithKind("ReplicaSet")),
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	err = cleanupReplicas(context.TODO(), s.clusterClient, dep)
	require.NoError(s.t, err)
	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, deps.Items, 1)
	pods, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pods.Items, 0)
	replicas, err := s.client.AppsV1().ReplicaSets(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, replicas.Items, 0)
}

func TestLabelSetFromMeta(t *testing.T) {
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
	require.EqualValues(t, &provision.LabelSet{
		Labels: map[string]string{
			"tsuru.io/x": "a",
			"tsuru.io/a": "1",
			"b":          "2",
		},
		RawLabels: map[string]string{
			"y": "b",
		},
		Prefix: tsuruLabelPrefix,
	}, ls)
}

func TestTopologySpreadConstraints(t *testing.T) {
	tests := []struct {
		labels     map[string]string
		constraint string
		expected   []apiv1.TopologySpreadConstraint
		errorMsg   string
	}{
		{
			labels:     map[string]string{"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-version": "v1", "tsuru.io/app-pool": "pool1"},
			constraint: "[{\"maxskew\":1, \"topologykey\":\"zone\"}]",
			expected: []apiv1.TopologySpreadConstraint{
				{
					MaxSkew:           1,
					TopologyKey:       "zone",
					WhenUnsatisfiable: apiv1.ScheduleAnyway,
					LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-version": "v1"}},
				},
			},
		},
		{
			labels:     map[string]string{"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-version": "v1", "tsuru.io/app-pool": "pool1"},
			constraint: "[{\"maxskew\":1, \"topologykey\":\"zone\"}, {\"maxskew\":3, \"topologykey\":\"hostname\"}]",
			expected: []apiv1.TopologySpreadConstraint{
				{
					MaxSkew:           1,
					TopologyKey:       "zone",
					WhenUnsatisfiable: apiv1.ScheduleAnyway,
					LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-version": "v1"}},
				},
				{
					MaxSkew:           3,
					TopologyKey:       "hostname",
					WhenUnsatisfiable: apiv1.ScheduleAnyway,
					LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-version": "v1"}},
				},
			},
		},
		{
			labels: map[string]string{"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-version": "v1"}, constraint: "", expected: nil,
		},
		{
			constraint: "[{\"topologykey\":\"testing\"}]",
			errorMsg:   "maxskew and topologykey are required in each topologySpreadConstraint",
		},
		{
			constraint: "[wrong json]",
			errorMsg:   "failed to parse JSON object for topologySpreadConstraint: invalid character 'w' looking for beginning of value",
		},
	}
	for _, tt := range tests {
		constraints, err := topologySpreadConstraints(tt.labels, tt.constraint)
		if tt.errorMsg != "" {
			require.ErrorContains(t, err, tt.errorMsg)
			continue
		}
		require.NoError(t, err)
		require.EqualValues(t, tt.expected, constraints)
	}
}
