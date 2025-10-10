// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/stretchr/testify/require"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
)

func (s *S) TestNewRouterControllerSameInstance(_ *check.C) {
	c1, err := getClusterController(s.p, s.clusterClient)
	require.NoError(s.t, err)
	c2, err := getClusterController(s.p, s.clusterClient)
	require.NoError(s.t, err)
	require.EqualValues(s.t, c1, c2)
}

type podListenerImpl struct{}

func (*podListenerImpl) OnPodEvent(pod *apiv1.Pod) {
}

func (s *S) TestPodListeners(_ *check.C) {
	podListener1 := &podListenerImpl{}
	podListener2 := &podListenerImpl{}

	clusterController, err := getClusterController(s.p, s.clusterClient)
	require.NoError(s.t, err)
	clusterController.addPodListener("listerner1", podListener1)
	require.Len(s.t, clusterController.podListeners, 1)
	clusterController.addPodListener("listerner2", podListener2)
	clusterController.removePodListener("listerner1")
	require.Len(s.t, clusterController.podListeners, 1)

	_, contains := clusterController.podListeners["listerner2"]
	require.True(s.t, contains)
	clusterController.removePodListener("listerner2")
	require.Len(s.t, clusterController.podListeners, 0)
}
