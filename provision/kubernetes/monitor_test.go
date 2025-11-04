// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"time"

	"github.com/stretchr/testify/require"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	clusterController.addPodListener("listener1", podListener1)
	require.Len(s.t, clusterController.podListeners, 1)
	clusterController.addPodListener("listener2", podListener2)
	clusterController.removePodListener("listener1")
	require.Len(s.t, clusterController.podListeners, 1)

	_, contains := clusterController.podListeners["listener2"]
	require.True(s.t, contains)
	clusterController.removePodListener("listener2")
	require.Len(s.t, clusterController.podListeners, 0)
}

func (s *S) TestLeaderElection(_ *check.C) {
	clusterController, err := getClusterController(s.p, s.clusterClient)
	require.NoError(s.t, err)

	require.Eventually(s.t, func() bool {
		return clusterController.isLeader()
	}, 5*time.Second, 100*time.Millisecond, "controller should become leader")
}

func (s *S) TestLeaderElectionUsesLeases(_ *check.C) {
	clusterController, err := getClusterController(s.p, s.clusterClient)
	require.NoError(s.t, err)

	require.Eventually(s.t, func() bool {
		return clusterController.isLeader()
	}, 5*time.Second, 100*time.Millisecond, "controller should become leader")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lease, err := s.clusterClient.CoordinationV1().Leases(s.clusterClient.Namespace()).Get(ctx, leaderElectionName, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.NotNil(s.t, lease)
	require.Equal(s.t, leaderElectionName, lease.Name)
	require.NotNil(s.t, lease.Spec.HolderIdentity)

	// Verify that no Endpoints resource was created (old behavior)
	_, err = s.clusterClient.CoreV1().Endpoints(s.clusterClient.Namespace()).Get(ctx, leaderElectionName, metav1.GetOptions{})
	require.Error(s.t, err, "Endpoints resource should not exist when using LeasesResourceLock")
}

func (s *S) TestLeaderElectionRenewal(_ *check.C) {
	clusterController, err := getClusterController(s.p, s.clusterClient)
	require.NoError(s.t, err)

	require.Eventually(s.t, func() bool {
		return clusterController.isLeader()
	}, 5*time.Second, 100*time.Millisecond, "controller should become leader")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lease1, err := s.clusterClient.CoordinationV1().Leases(s.clusterClient.Namespace()).Get(ctx, leaderElectionName, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.NotNil(s.t, lease1.Spec.RenewTime)

	initialRenewTime := lease1.Spec.RenewTime.Time

	time.Sleep(retryPeriod + time.Second)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	lease2, err := s.clusterClient.CoordinationV1().Leases(s.clusterClient.Namespace()).Get(ctx2, leaderElectionName, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.NotNil(s.t, lease2.Spec.RenewTime)
	require.True(s.t, lease2.Spec.RenewTime.Time.After(initialRenewTime), "lease should have been renewed")
}
