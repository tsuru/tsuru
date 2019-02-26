// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/router/rebuild"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision"
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
	err = rebuild.Initialize(func(appName string) (rebuild.RebuildApp, error) {
		defer close(rebuildCalled)
		c.Assert(appName, check.Equals, "myapp")
		return a, errors.New("stop here")
	})
	c.Assert(err, check.IsNil)
	defer rebuild.Shutdown(context.Background())
	controller, err := getClusterController(s.clusterClient)
	c.Assert(err, check.IsNil)
	defer controller.stop()
	basePod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "pod1",
			Labels:          labels.ToLabels(),
			ResourceVersion: "0",
		},
	}
	watchFake.Add(basePod)
	basePod = basePod.DeepCopy()
	basePod.ResourceVersion = "1"
	watchFake.Modify(basePod)
	select {
	case <-rebuildCalled:
	case <-time.After(5 * time.Second):
		c.Fatal("timeout waiting for rebuild call")
	}
}

func (s *S) TestNewRouterControllerSameInstance(c *check.C) {
	c1, err := getClusterController(s.clusterClient)
	c.Assert(err, check.IsNil)
	defer c1.stop()
	c2, err := getClusterController(s.clusterClient)
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.Equals, c2)
}
