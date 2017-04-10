// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"errors"
	"fmt"
	"time"

	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestDeploymentNameForApp(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	name := deploymentNameForApp(a, "p1")
	c.Assert(name, check.Equals, "myapp-p1")
}

func (s *S) TestDeployPodNameForApp(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	name := deployPodNameForApp(a)
	c.Assert(name, check.Equals, "myapp-deploy")
}

func (s *S) TestExecCommandPodNameForApp(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	name := execCommandPodNameForApp(a)
	c.Assert(name, check.Equals, "myapp-isolated-run")
}

func (s *S) TestDaemonSetName(c *check.C) {
	c.Assert(daemonSetName("d1", ""), check.Equals, "node-container-d1-all")
	c.Assert(daemonSetName("d1", "p1"), check.Equals, "node-container-d1-pool-p1")
}

func (s *S) TestWaitFor(c *check.C) {
	err := waitFor(100*time.Millisecond, func() (bool, error) {
		return true, nil
	})
	c.Assert(err, check.IsNil)
	err = waitFor(100*time.Millisecond, func() (bool, error) {
		return false, nil
	})
	c.Assert(err, check.ErrorMatches, `timeout after .*`)
	err = waitFor(100*time.Millisecond, func() (bool, error) {
		return true, errors.New("myerr")
	})
	c.Assert(err, check.ErrorMatches, `myerr`)
}

func (s *S) TestWaitForPod(c *check.C) {
	err := waitForPod(s.client.clusterClient, "pod1", false, 100*time.Millisecond)
	c.Assert(err, check.ErrorMatches, `Pod "pod1" not found`)
	var wantedPhase v1.PodPhase
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod, ok := action.(ktesting.CreateAction).GetObject().(*v1.Pod)
		c.Assert(ok, check.Equals, true)
		pod.Status.Phase = wantedPhase
		return false, nil, nil
	})
	tests := []struct {
		phase   v1.PodPhase
		err     string
		evt     *v1.Event
		running bool
	}{
		{phase: v1.PodSucceeded},
		{phase: v1.PodRunning, err: `timeout after .*`},
		{phase: v1.PodRunning, running: true},
		{phase: v1.PodPending, err: `timeout after .*`},
		{phase: v1.PodFailed, err: `invalid pod phase "Failed"`},
		{phase: v1.PodUnknown, err: `invalid pod phase "Unknown"`},
		{phase: v1.PodFailed, err: `invalid pod phase "Failed": my evt message`, evt: &v1.Event{
			ObjectMeta: v1.ObjectMeta{
				Name:      "pod1.evt1",
				Namespace: s.client.Namespace(),
			},
			InvolvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Name:      "pod1",
				Namespace: s.client.Namespace(),
			},
			Message: "my evt message",
		}},
	}
	for _, tt := range tests {
		wantedPhase = tt.phase
		_, err = s.client.Core().Pods(s.client.Namespace()).Create(&v1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Name:      "pod1",
				Namespace: s.client.Namespace(),
			},
		})
		c.Assert(err, check.IsNil)
		if tt.evt != nil {
			_, err = s.client.Core().Events(s.client.Namespace()).Create(tt.evt)
			c.Assert(err, check.IsNil)
		}
		err = waitForPod(s.client.clusterClient, "pod1", tt.running, 100*time.Millisecond)
		if tt.err == "" {
			c.Assert(err, check.IsNil)
		} else {
			c.Assert(err, check.ErrorMatches, tt.err)
		}
		err = cleanupPod(s.client.clusterClient, "pod1")
		c.Assert(err, check.IsNil)
	}
}

func (s *S) TestCleanupPods(c *check.C) {
	for i := 0; i < 3; i++ {
		labels := map[string]string{"a": "x"}
		if i == 2 {
			labels["a"] = "y"
		}
		_, err := s.client.Core().Pods(s.client.Namespace()).Create(&v1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: s.client.Namespace(),
				Labels:    labels,
			},
		})
		c.Assert(err, check.IsNil)
	}
	err := cleanupPods(s.client.clusterClient, v1.ListOptions{
		LabelSelector: "a=x",
	})
	c.Assert(err, check.IsNil)
	pods, err := s.client.Core().Pods(s.client.Namespace()).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.DeepEquals, []v1.Pod{{
		ObjectMeta: v1.ObjectMeta{
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
	_, err := s.client.Extensions().Deployments(s.client.Namespace()).Create(&extensions.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: s.client.Namespace(),
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Extensions().ReplicaSets(s.client.Namespace()).Create(&extensions.ReplicaSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: s.client.Namespace(),
			Labels:    expectedLabels,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Core().Pods(s.client.Namespace()).Create(&v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: s.client.Namespace(),
			Labels:    expectedLabels,
		},
	})
	c.Assert(err, check.IsNil)
	err = cleanupDeployment(s.client.clusterClient, a, "p1")
	c.Assert(err, check.IsNil)
	deps, err := s.client.Extensions().Deployments(s.client.Namespace()).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	pods, err := s.client.Core().Pods(s.client.Namespace()).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.Extensions().ReplicaSets(s.client.Namespace()).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestCleanupReplicas(c *check.C) {
	_, err := s.client.Extensions().ReplicaSets(s.client.Namespace()).Create(&extensions.ReplicaSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: s.client.Namespace(),
			Labels: map[string]string{
				"a": "x",
			},
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Core().Pods(s.client.Namespace()).Create(&v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: s.client.Namespace(),
			Labels: map[string]string{
				"a": "x",
			},
		},
	})
	c.Assert(err, check.IsNil)
	err = cleanupReplicas(s.client.clusterClient, v1.ListOptions{
		LabelSelector: "a=x",
	})
	c.Assert(err, check.IsNil)
	deps, err := s.client.Extensions().Deployments(s.client.Namespace()).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	pods, err := s.client.Core().Pods(s.client.Namespace()).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.Extensions().ReplicaSets(s.client.Namespace()).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestCleanupDaemonSet(c *check.C) {
	_, err := s.client.Extensions().DaemonSets(s.client.Namespace()).Create(&extensions.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "node-container-bs-pool-p1",
			Namespace: s.client.Namespace(),
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Core().Pods(s.client.Namespace()).Create(&v1.Pod{
		ObjectMeta: v1.ObjectMeta{
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
	err = cleanupDaemonSet(s.client.clusterClient, "bs", "p1")
	c.Assert(err, check.IsNil)
	daemons, err := s.client.Extensions().DaemonSets(s.client.Namespace()).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 0)
	pods, err := s.client.Core().Pods(s.client.Namespace()).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
}

func (s *S) TestLabelSetFromMeta(c *check.C) {
	meta := v1.ObjectMeta{
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
