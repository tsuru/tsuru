// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"errors"
	"fmt"
	"time"

	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"k8s.io/client-go/pkg/api/v1"
	batch "k8s.io/client-go/pkg/apis/batch/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

func (s *S) TestDeploymentNameForApp(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	name := deploymentNameForApp(a, "p1")
	c.Assert(name, check.Equals, "myapp-p1")
}

func (s *S) TestDeployJobNameForApp(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	name := deployJobNameForApp(a)
	c.Assert(name, check.Equals, "myapp-deploy")
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

func (s *S) TestWaitForJobContainerRunning(c *check.C) {
	podName, err := waitForJobContainerRunning(s.client, map[string]string{"a": "x"}, "cont1", 100*time.Millisecond)
	c.Assert(err, check.ErrorMatches, `timeout after .*`)
	c.Assert(podName, check.Equals, "")
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	reaction, podReady := s.jobWithPodReaction(a, c)
	defer podReady.Wait()
	s.client.PrependReactor("create", "jobs", reaction)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, jErr := s.client.Batch().Jobs(tsuruNamespace).Create(&batch.Job{
			ObjectMeta: v1.ObjectMeta{
				Name:      "job1",
				Namespace: tsuruNamespace,
			},
			Spec: batch.JobSpec{
				Template: v1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Name:   "job1",
						Labels: map[string]string{"a": "x"},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{Name: "cont1"},
						},
					},
				},
			},
		})
		c.Assert(jErr, check.IsNil)
	}()
	podName, err = waitForJobContainerRunning(s.client, map[string]string{"a": "x"}, "cont1", 2*time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(podName, check.Equals, "job1-pod")
	<-done
}

func (s *S) TestWaitForJob(c *check.C) {
	err := waitForJob(s.client, "job1", 100*time.Millisecond)
	c.Assert(err, check.ErrorMatches, `Job.batch "job1" not found`)
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	reaction, podReady := s.jobWithPodReaction(a, c)
	defer podReady.Wait()
	s.client.PrependReactor("create", "jobs", reaction)
	_, err = s.client.Batch().Jobs(tsuruNamespace).Create(&batch.Job{
		ObjectMeta: v1.ObjectMeta{
			Name:      "job1",
			Namespace: tsuruNamespace,
		},
	})
	c.Assert(err, check.IsNil)
	err = waitForJob(s.client, "job1", 2*time.Minute)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCleanupPods(c *check.C) {
	for i := 0; i < 3; i++ {
		labels := map[string]string{"a": "x"}
		if i == 2 {
			labels["a"] = "y"
		}
		_, err := s.client.Core().Pods(tsuruNamespace).Create(&v1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: tsuruNamespace,
				Labels:    labels,
			},
		})
		c.Assert(err, check.IsNil)
	}
	err := cleanupPods(s.client, v1.ListOptions{
		LabelSelector: "a=x",
	})
	c.Assert(err, check.IsNil)
	pods, err := s.client.Core().Pods(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.DeepEquals, []v1.Pod{{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod-2",
			Namespace: tsuruNamespace,
			Labels:    map[string]string{"a": "y"},
		},
	}})
}

func (s *S) TestCleanupJob(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	reaction, podReady := s.jobWithPodReaction(a, c)
	s.client.PrependReactor("create", "jobs", reaction)
	_, err := s.client.Batch().Jobs(tsuruNamespace).Create(&batch.Job{
		ObjectMeta: v1.ObjectMeta{
			Name:      "job1",
			Namespace: tsuruNamespace,
		},
		Spec: batch.JobSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Name: "job1",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "cont1"},
					},
				},
			},
		},
	})
	c.Assert(err, check.IsNil)
	podReady.Wait()
	err = cleanupJob(s.client, "job1")
	c.Assert(err, check.IsNil)
	pods, err := s.client.Core().Pods(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	jobs, err := s.client.Batch().Jobs(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(jobs.Items, check.HasLen, 0)
}

func (s *S) TestCleanupDeployment(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "plat", 1)
	ls, err := podLabels(a, "p1", "", 0)
	c.Assert(err, check.IsNil)
	_, err = s.client.Extensions().Deployments(tsuruNamespace).Create(&extensions.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: tsuruNamespace,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Extensions().ReplicaSets(tsuruNamespace).Create(&extensions.ReplicaSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: tsuruNamespace,
			Labels:    ls.ToLabels(),
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Core().Pods(tsuruNamespace).Create(&v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: tsuruNamespace,
			Labels:    ls.ToLabels(),
		},
	})
	c.Assert(err, check.IsNil)
	err = cleanupDeployment(s.client, a, "p1")
	c.Assert(err, check.IsNil)
	deps, err := s.client.Extensions().Deployments(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	pods, err := s.client.Core().Pods(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.Extensions().ReplicaSets(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestCleanupReplicas(c *check.C) {
	_, err := s.client.Extensions().ReplicaSets(tsuruNamespace).Create(&extensions.ReplicaSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: tsuruNamespace,
			Labels: map[string]string{
				"a": "x",
			},
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Core().Pods(tsuruNamespace).Create(&v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: tsuruNamespace,
			Labels: map[string]string{
				"a": "x",
			},
		},
	})
	c.Assert(err, check.IsNil)
	err = cleanupReplicas(s.client, v1.ListOptions{
		LabelSelector: "a=x",
	})
	c.Assert(err, check.IsNil)
	deps, err := s.client.Extensions().Deployments(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	pods, err := s.client.Core().Pods(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	replicas, err := s.client.Extensions().ReplicaSets(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(replicas.Items, check.HasLen, 0)
}

func (s *S) TestCleanupDaemonSet(c *check.C) {
	ls := nodeContainerPodLabels("bs", "p1")
	_, err := s.client.Extensions().DaemonSets(tsuruNamespace).Create(&extensions.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "node-container-bs-pool-p1",
			Namespace: tsuruNamespace,
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.client.Core().Pods(tsuruNamespace).Create(&v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "node-container-bs-pool-p1-xyz",
			Namespace: tsuruNamespace,
			Labels:    ls.ToLabels(),
		},
	})
	c.Assert(err, check.IsNil)
	err = cleanupDaemonSet(s.client, "bs", "p1")
	c.Assert(err, check.IsNil)
	deps, err := s.client.Extensions().Deployments(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	pods, err := s.client.Core().Pods(tsuruNamespace).List(v1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
}
