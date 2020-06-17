// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router/rebuild"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestNewClusterController(c *check.C) {
	s.clusterClient.CustomData = map[string]string{
		routerAddressLocalKey: "true",
	}
	watchFake := watch.NewFake()
	s.client.Fake.PrependWatchReactor("pods", ktesting.DefaultWatchReactor(watchFake, nil))
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	labels, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:     a,
		Process: "p1",
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	c.Assert(err, check.IsNil)
	rebuildCalled := make(chan struct{})
	oldRebuildFunc := runRoutesRebuild
	defer func() {
		runRoutesRebuild = oldRebuildFunc
	}()
	runRoutesRebuild = func(appName string) {
		defer func() { rebuildCalled <- struct{}{} }()
		c.Assert(appName, check.Equals, "myapp")
	}
	c.Assert(err, check.IsNil)
	defer rebuild.Shutdown(context.Background())
	_, err = getClusterController(s.p, s.clusterClient)
	c.Assert(err, check.IsNil)

	basePod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "pod1",
			Labels:          labels.ToLabels(),
			ResourceVersion: "0",
		},
	}
	watchFake.Add(basePod.DeepCopy())
	basePod.ResourceVersion = "1"
	watchFake.Modify(basePod.DeepCopy())
	select {
	case <-rebuildCalled:
	case <-time.After(5 * time.Second):
		c.Fatal("timeout waiting for first rebuild call")
	}

	basePod.ResourceVersion = "2"
	watchFake.Modify(basePod.DeepCopy())
	select {
	case <-rebuildCalled:
		c.Fatal("rebuild called when no call was expected")
	case <-time.After(5 * time.Second):
	}

	basePod.ResourceVersion = "3"
	basePod.Status.Conditions = []apiv1.PodCondition{
		{Type: apiv1.PodReady, Status: apiv1.ConditionFalse},
	}
	watchFake.Modify(basePod.DeepCopy())
	select {
	case <-rebuildCalled:
	case <-time.After(5 * time.Second):
		c.Fatal("timeout waiting for second rebuild call")
	}
}

func (s *S) TestNewRouterControllerSameInstance(c *check.C) {
	c1, err := getClusterController(s.p, s.clusterClient)
	c.Assert(err, check.IsNil)
	c2, err := getClusterController(s.p, s.clusterClient)
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.Equals, c2)
}

func (s *S) TestPodListeners(c *check.C) {
	podListener1 := &podListener{}
	podListener2 := &podListener{}

	clusterController, err := getClusterController(s.p, s.clusterClient)
	c.Assert(err, check.IsNil)
	clusterController.addPodListener("my-app", podListener1)
	c.Assert(clusterController.podListeners["my-app"], check.HasLen, 1)
	clusterController.addPodListener("my-app", podListener2)
	clusterController.removePodListener("my-app", podListener1)
	c.Assert(clusterController.podListeners["my-app"], check.HasLen, 1)
	_, contains := clusterController.podListeners["my-app"][podListener2]
	c.Assert(contains, check.Equals, true)
	clusterController.removePodListener("my-app", podListener2)
	c.Assert(clusterController.podListeners["my-app"], check.HasLen, 0)
}
