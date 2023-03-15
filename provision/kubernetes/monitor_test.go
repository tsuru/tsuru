// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
)

func (s *S) TestNewRouterControllerSameInstance(c *check.C) {
	c1, err := getClusterController(s.p, s.clusterClient)
	c.Assert(err, check.IsNil)
	c2, err := getClusterController(s.p, s.clusterClient)
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.Equals, c2)
}

type podListenerImpl struct {
}

func (*podListenerImpl) OnPodEvent(pod *apiv1.Pod) {
}

func (s *S) TestPodListeners(c *check.C) {

	podListener1 := &podListenerImpl{}
	podListener2 := &podListenerImpl{}

	clusterController, err := getClusterController(s.p, s.clusterClient)
	c.Assert(err, check.IsNil)
	clusterController.addPodListener("listerner1", podListener1)
	c.Assert(clusterController.podListeners, check.HasLen, 1)
	clusterController.addPodListener("listerner2", podListener2)
	clusterController.removePodListener("listerner1")
	c.Assert(clusterController.podListeners, check.HasLen, 1)

	_, contains := clusterController.podListeners["listerner2"]
	c.Assert(contains, check.Equals, true)
	clusterController.removePodListener("listerner2")
	c.Assert(clusterController.podListeners, check.HasLen, 0)
}
