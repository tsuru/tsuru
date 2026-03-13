// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	fakekedaclientset "github.com/kedacore/keda/v2/pkg/generated/clientset/versioned/fake"
	"github.com/stretchr/testify/require"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	kTesting "github.com/tsuru/tsuru/provision/kubernetes/testing"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	eventTypes "github.com/tsuru/tsuru/types/event"
	provTypes "github.com/tsuru/tsuru/types/provision"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	check "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	fakevpa "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/fake"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/remotecommand"
	fakeBackendConfig "k8s.io/ingress-gce/pkg/backendconfig/client/clientset/versioned/fake"
	fakemetrics "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func (s *S) prepareMultiCluster(_ *check.C) (*kTesting.ClientWrapper, *kTesting.ClientWrapper, *kTesting.ClientWrapper) {
	cluster1 := &provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"https://clusteraddr1"},
		Default:     true,
		Provisioner: provisionerName,
		CustomData:  map[string]string{},
	}
	clusterClient1, err := NewClusterClient(cluster1)
	require.NoError(s.t, err)
	client1 := &kTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
		MetricsClientset:       fakemetrics.NewSimpleClientset(),
		VPAClientset:           fakevpa.NewSimpleClientset(),
		BackendClientset:       fakeBackendConfig.NewSimpleClientset(),
		KEDAClientForConfig:    fakekedaclientset.NewSimpleClientset(),
		ClusterInterface:       clusterClient1,
	}
	clusterClient1.Interface = client1

	cluster2 := &provTypes.Cluster{
		Name:        "c2",
		Addresses:   []string{"https://clusteraddr2"},
		Pools:       []string{"pool2"},
		Provisioner: provisionerName,
		CustomData:  map[string]string{},
	}
	clusterClient2, err := NewClusterClient(cluster2)
	require.NoError(s.t, err)
	client2 := &kTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
		MetricsClientset:       fakemetrics.NewSimpleClientset(),
		VPAClientset:           fakevpa.NewSimpleClientset(),
		BackendClientset:       fakeBackendConfig.NewSimpleClientset(),
		KEDAClientForConfig:    fakekedaclientset.NewSimpleClientset(),
		ClusterInterface:       clusterClient2,
	}
	clusterClient2.Interface = client2

	cluster3 := &provTypes.Cluster{
		Name:        "c3",
		Addresses:   []string{"https://clusteraddr3"},
		Pools:       []string{"pool3"},
		Provisioner: provisionerName,
		CustomData:  map[string]string{},
	}
	clusterClient3, err := NewClusterClient(cluster2)
	require.NoError(s.t, err)
	client3 := &kTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
		MetricsClientset:       fakemetrics.NewSimpleClientset(),
		VPAClientset:           fakevpa.NewSimpleClientset(),
		BackendClientset:       fakeBackendConfig.NewSimpleClientset(),
		KEDAClientForConfig:    fakekedaclientset.NewSimpleClientset(),
		ClusterInterface:       clusterClient2,
	}
	clusterClient3.Interface = client3

	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return []provTypes.Cluster{*cluster1, *cluster2, *cluster3}, nil
	}

	s.mockService.Cluster.OnFindByPool = func(provName, poolName string) (*provTypes.Cluster, error) {
		if poolName == "pool2" {
			return cluster2, nil
		}
		if poolName == "pool3" {
			return cluster3, nil
		}
		return cluster1, nil
	}

	ClientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
		if conf.Host == "https://clusteraddr1" {
			return client1, nil
		}
		if conf.Host == "https://clusteraddr3" {
			return client3, nil
		}
		return client2, nil
	}

	return client1, client2, client3
}

func (s *S) TestUnits(c *check.C) {
	_, err := s.client.CoreV1().Pods("default").Create(context.TODO(), &apiv1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "non-app-pod",
	}}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	s.client.PrependReactor("create", "services", s.mock.ServiceWithPortReaction(c, []apiv1.ServicePort{
		{NodePort: int32(30001)},
		{NodePort: int32(30002)},
		{NodePort: int32(30003)},
	}))
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web":    {"python", "myapp.py"},
		"worker": {"myworker"},
	})
	require.NoError(s.t, err)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	require.NoError(s.t, err)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 2)
	sort.Slice(units, func(i, j int) bool {
		return units[i].ProcessName < units[j].ProcessName
	})
	for i, u := range units {
		splittedName := strings.Split(u.ID, "-")
		require.Len(s.t, splittedName, 5)
		require.Equal(s.t, splittedName[0], "myapp")
		units[i].ID = ""
		units[i].Name = ""
		require.NotNil(s.t, units[i].CreatedAt)
		units[i].CreatedAt = nil
	}
	restarts := int32(0)
	ready := false
	require.EqualValues(s.t, []provTypes.Unit{
		{
			AppName:     "myapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "192.168.99.1",
			Status:      "started",
			Version:     1,
			Routable:    true,
			Restarts:    &restarts,
			Ready:       &ready,
		},
		{
			AppName:     "myapp",
			ProcessName: "worker",
			Type:        "python",
			IP:          "192.168.99.1",
			Status:      "started",
			Version:     1,
			Routable:    true,
			Restarts:    &restarts,
			Ready:       &ready,
		},
	}, units)
}

func (s *S) TestUnitsMultipleAppsNodes(c *check.C) {
	a1 := provisiontest.NewFakeAppWithPool("myapp", "python", "pool1", 0)
	a2 := provisiontest.NewFakeAppWithPool("otherapp", "python", "pool2", 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/pods" {
			s.mock.ListPodsHandler(c)(w, r)
			return
		}
	}))
	s.mock.MockfakeNodes(srv.URL)
	t0 := time.Now().UTC()
	for _, a := range []*appTypes.App{a1, a2} {
		for i := 1; i <= 2; i++ {
			s.waitPodUpdate(c, func() {
				_, err := s.client.CoreV1().Pods("default").Create(context.TODO(), &apiv1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%s-%d", a.Name, i),
						Labels: map[string]string{
							"tsuru.io/app-name":     a.Name,
							"tsuru.io/app-process":  "web",
							"tsuru.io/app-platform": "python",
						},
						Namespace:         "default",
						CreationTimestamp: metav1.Time{Time: t0},
					},
					Spec: apiv1.PodSpec{
						NodeName: fmt.Sprintf("n%d", i),
					},
					Status: apiv1.PodStatus{
						HostIP: fmt.Sprintf("192.168.99.%d", i),
					},
				}, metav1.CreateOptions{})
				require.NoError(s.t, err)
			})
		}
	}
	_, err := s.client.CoreV1().Services("default").Create(context.TODO(), &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-web",
			Namespace: "default",
		},
		Spec: apiv1.ServiceSpec{
			Ports: []apiv1.ServicePort{
				{NodePort: int32(30001)},
				{NodePort: int32(30002)},
			},
			Type: apiv1.ServiceTypeNodePort,
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Services("default").Create(context.TODO(), &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "otherapp-web",
			Namespace: "default",
		},
		Spec: apiv1.ServiceSpec{
			Ports: []apiv1.ServicePort{
				{NodePort: int32(30001)},
				{NodePort: int32(30002)},
			},
			Type: apiv1.ServiceTypeNodePort,
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	units, err := s.p.Units(context.TODO(), a1, a2)
	require.NoError(s.t, err)
	require.Len(s.t, units, 4)
	sort.Slice(units, func(i, j int) bool {
		return units[i].ID < units[j].ID
	})
	restarts := int32(0)
	ready := false
	require.EqualValues(s.t, []provTypes.Unit{
		{
			ID:          "myapp-1",
			Name:        "myapp-1",
			AppName:     "myapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "192.168.99.1",
			Status:      "",
			Routable:    true,
			Restarts:    &restarts,
			Ready:       &ready,
			CreatedAt:   &t0,
		},
		{
			ID:          "myapp-2",
			Name:        "myapp-2",
			AppName:     "myapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "192.168.99.2",
			Status:      "",
			Routable:    true,
			Restarts:    &restarts,
			Ready:       &ready,
			CreatedAt:   &t0,
		},
		{
			ID:          "otherapp-1",
			Name:        "otherapp-1",
			AppName:     "otherapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "192.168.99.1",
			Status:      "",
			Routable:    true,
			Restarts:    &restarts,
			Ready:       &ready,
			CreatedAt:   &t0,
		},
		{
			ID:          "otherapp-2",
			Name:        "otherapp-2",
			AppName:     "otherapp",
			ProcessName: "web",
			Type:        "python",
			IP:          "192.168.99.2",
			Status:      "",
			Routable:    true,
			Restarts:    &restarts,
			Ready:       &ready,
			CreatedAt:   &t0,
		},
	}, units)
}

func (s *S) TestUnitsSkipTerminating(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web":    {"python", "myapp.py"},
		"worker": {"myworker"},
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	require.NoError(s.t, err)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, podlist.Items, 2)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			if p.Labels["tsuru.io/app-process"] == "worker" {
				deadline := int64(10)
				p.Spec.ActiveDeadlineSeconds = &deadline
				_, err = s.client.CoreV1().Pods("default").Update(context.TODO(), &p, metav1.UpdateOptions{})
				require.NoError(s.t, err)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	require.Equal(s.t, "web", units[0].ProcessName)
}

func (s *S) TestUnitsSkipEvicted(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web":    {"python", "myapp.py"},
		"worker": {"myworker"},
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	require.NoError(s.t, err)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, podlist.Items, 2)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			if p.Labels["tsuru.io/app-process"] == "worker" {
				p.Status.Phase = apiv1.PodFailed
				p.Status.Reason = "Evicted"
				_, err = s.client.CoreV1().Pods("default").Update(context.TODO(), &p, metav1.UpdateOptions{})
				require.NoError(s.t, err)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	require.Equal(s.t, "web", units[0].ProcessName)
}

func (s *S) TestUnitsStarting(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	require.NoError(s.t, err)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, podlist.Items, 1)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			if p.Labels["tsuru.io/app-process"] == "web" {
				p.Status.Phase = apiv1.PodRunning
				p.Status.ContainerStatuses = []apiv1.ContainerStatus{
					{
						Ready: false,
					},
				}
				_, err = s.client.CoreV1().Pods("default").Update(context.TODO(), &p, metav1.UpdateOptions{})
				require.NoError(s.t, err)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	require.EqualValues(s.t, provTypes.UnitStatusStarting, units[0].Status)
}

func (s *S) TestUnitsStartingError(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	require.NoError(s.t, err)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, podlist.Items, 1)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			if p.Labels["tsuru.io/app-process"] == "web" {
				p.Status.Phase = apiv1.PodRunning
				p.Status.ContainerStatuses = []apiv1.ContainerStatus{
					{
						Ready: false,
						LastTerminationState: apiv1.ContainerState{
							Terminated: &apiv1.ContainerStateTerminated{
								Reason: "OOMKilled",
							},
						},
					},
				}
				_, err = s.client.CoreV1().Pods("default").Update(context.TODO(), &p, metav1.UpdateOptions{})
				require.NoError(s.t, err)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	require.EqualValues(s.t, provTypes.UnitStatusError, units[0].Status)
	require.EqualValues(s.t, "OOMKilled", units[0].StatusReason)
}

func (s *S) TestUnitsCrashLoopBackOff(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	require.NoError(s.t, err)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, podlist.Items, 1)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			if p.Labels["tsuru.io/app-process"] == "web" {
				p.Status.Phase = apiv1.PodRunning
				p.Status.ContainerStatuses = []apiv1.ContainerStatus{
					{
						Ready: false,
						LastTerminationState: apiv1.ContainerState{
							Terminated: &apiv1.ContainerStateTerminated{
								Reason: "OOMKilled",
							},
						},
						State: apiv1.ContainerState{
							Waiting: &apiv1.ContainerStateWaiting{
								Reason: "CrashLoopBackOff",
							},
						},
					},
				}
				_, err = s.client.CoreV1().Pods("default").Update(context.TODO(), &p, metav1.UpdateOptions{})
				require.NoError(s.t, err)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	require.Equal(s.t, provTypes.UnitStatusError, units[0].Status)
	require.Equal(s.t, "OOMKilled", units[0].StatusReason)
}

func (s *S) TestUnitsCrashLoopBackOffWithExitCode(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	require.NoError(s.t, err)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, podlist.Items, 1)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			if p.Labels["tsuru.io/app-process"] == "web" {
				p.Status.Phase = apiv1.PodRunning
				p.Status.ContainerStatuses = []apiv1.ContainerStatus{
					{
						Ready: false,
						LastTerminationState: apiv1.ContainerState{
							Terminated: &apiv1.ContainerStateTerminated{
								Reason:   "Error",
								ExitCode: 1,
							},
						},
						State: apiv1.ContainerState{
							Waiting: &apiv1.ContainerStateWaiting{
								Reason: "CrashLoopBackOff",
							},
						},
					},
				}
				_, err = s.client.CoreV1().Pods("default").Update(context.TODO(), &p, metav1.UpdateOptions{})
				require.NoError(s.t, err)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	require.Equal(s.t, provTypes.UnitStatusError, units[0].Status)
	require.Equal(s.t, "exitCode: 1", units[0].StatusReason)
}

func (s *S) TestUnitsEmpty(_ *check.C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(s.t, "tsuru.io/app-name in (myapp)", r.FormValue("labelSelector"))
		output := `{"items": []}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(output))
	}))
	defer srv.Close()
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 0)
}

func (s *S) TestUnitsNoApps(_ *check.C) {
	units, err := s.p.Units(context.TODO())
	require.NoError(s.t, err)
	require.Len(s.t, units, 0)
}

func (s *S) TestAddUnits(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 3, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 3)
}

func (s *S) TestAddUnitsNotProvisionedRecreateAppCRD(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Destroy(context.TODO(), a)
	require.NoError(s.t, err)
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	a.Deploys = 1
	err = s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
}

func (s *S) TestRemoveUnits(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 3, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 3)
	err = s.p.RemoveUnits(context.TODO(), a, 2, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	units, err = s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
}

func (s *S) TestRemoveUnits_SetUnitsToZero(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 5, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 5)
	var buffer bytes.Buffer
	err = s.p.RemoveUnits(context.TODO(), a, 5, "web", version, &buffer)
	require.NoError(s.t, err)
	wait()
	units, err = s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 0)
	require.Contains(s.t, buffer.String(), "---- Calling app stop internally as the number of units is zero ----")
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.NotNil(s.t, dep)
	require.Equal(s.t, "true", dep.Labels["tsuru.io/is-stopped"])
	svcs, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "tsuru.io/app-name=myapp"})
	require.NoError(s.t, err)
	require.Len(s.t, svcs.Items, 2)
}

func (s *S) TestRestart(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	id := units[0].ID
	err = s.p.Restart(context.TODO(), a, "", version, nil)
	require.NoError(s.t, err)
	wait()
	units, err = s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	require.NotEqual(s.t, id, units[0].ID)
}

func (s *S) TestShouldRestartOnlyOnce(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	var updateCount int32
	s.client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		atomic.AddInt32(&updateCount, 1)
		return false, nil, nil
	})

	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 3, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 3)

	atomic.StoreInt32(&updateCount, 0)

	err = s.p.Restart(context.TODO(), a, "", nil, nil)
	require.NoError(s.t, err)
	wait()

	require.Equal(s.t, int32(1), atomic.LoadInt32(&updateCount))
	units, err = s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 3)
}

func (s *S) TestRestartNotProvisionedRecreateAppCRD(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Destroy(context.TODO(), a)
	require.NoError(s.t, err)
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	a.Deploys = 1
	err = s.p.Restart(context.TODO(), a, "", version, nil)
	require.NoError(s.t, err)
}

func (s *S) TestRestart_ShouldNotRestartBaseVersionWhenStopped_StoppedDueToScaledToZero(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	v1 := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})

	evt1, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	require.NoError(s.t, err)

	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{
		App:     a,
		Version: v1,
		Event:   evt1,
	})
	require.NoError(s.t, err)
	err = evt1.Done(context.TODO(), nil)
	require.NoError(s.t, err)

	wait()

	v2 := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"./my/app.sh"},
	})

	evt2, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDeploy,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	require.NoError(s.t, err)

	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{
		App:              a,
		Version:          v2,
		Event:            evt2,
		PreserveVersions: true,
	})
	require.NoError(s.t, err)

	err = evt2.Done(context.TODO(), nil)
	require.NoError(s.t, err)

	wait()

	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 2)

	err = s.p.RemoveUnits(context.TODO(), a, 1, "", v1, nil)
	require.NoError(s.t, err)

	units, err = s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	require.Equal(s.t, 2, units[0].Version)

	err = s.p.Restart(context.TODO(), a, "", nil, nil)
	require.NoError(s.t, err)

	units, err = s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	require.Equal(s.t, 2, units[0].Version)
}

func (s *S) TestStopStart(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 2, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	err = s.p.Stop(context.TODO(), a, "", version, &bytes.Buffer{})
	require.NoError(s.t, err)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	svcs, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "tsuru.io/app-name=myapp",
	})
	require.NoError(s.t, err)
	require.Len(s.t, svcs.Items, 2)
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 0)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	require.NoError(s.t, err)
	wait()
	svcs, err = s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "tsuru.io/app-name=myapp",
	})
	require.NoError(s.t, err)
	require.Len(s.t, svcs.Items, 2)
}

func (s *S) TestProvisionerDestroy(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	err = s.p.Destroy(context.TODO(), a)
	require.NoError(s.t, err)
	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, deps.Items, 0)
	services, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, services.Items, 0)
	serviceAccounts, err := s.client.CoreV1().ServiceAccounts(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, serviceAccounts.Items, 0)
	pdbList, err := s.client.PolicyV1().PodDisruptionBudgets(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pdbList.Items, 0)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, appList.Items, 0)
}

func (s *S) TestProvisionerDestroyVersion(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	deployEvent, err := event.New(context.TODO(), &event.Opts{
		Target:      eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:        permission.PermAppDeploy,
		Owner:       s.token,
		Allowed:     event.Allowed(permission.PermAppDeploy),
		DisableLock: true,
	})
	require.NoError(s.t, err)
	customData1 := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version1 := newCommittedVersion(c, a, customData1)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: deployEvent})
	require.NoError(s.t, err)
	wait()

	customData2 := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version2 := newCommittedVersion(c, a, customData2)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version2, Event: deployEvent, PreserveVersions: true})
	require.NoError(s.t, err)
	wait()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	services, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, services.Items, 4)
	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-web-v2", metav1.GetOptions{})
	require.NoError(s.t, err)
	err = s.p.DestroyVersion(context.TODO(), a, version2)
	require.NoError(s.t, err)
	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, deps.Items, 1)
	services, err = s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, services.Items, 3)
	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-web-v2", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	serviceAccounts, err := s.client.CoreV1().ServiceAccounts(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, serviceAccounts.Items, 1)
	pdbList, err := s.client.PolicyV1().PodDisruptionBudgets(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pdbList.Items, 1)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, appList.Items, 1)
}

func (s *S) TestProvisionerRoutableAddressesMultipleProcs(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web":   {"run", "mycmd", "arg1"},
		"other": {"my", "other", "cmd"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()
	addrs, err := s.p.RoutableAddresses(context.TODO(), a)
	require.NoError(s.t, err)
	sort.Slice(addrs, func(i, j int) bool {
		return addrs[i].Prefix < addrs[j].Prefix
	})
	for k := range addrs {
		sort.Slice(addrs[k].Addresses, func(i, j int) bool {
			return addrs[k].Addresses[i].Host < addrs[k].Addresses[j].Host
		})
	}
	expected := []appTypes.RoutableAddresses{
		{
			Prefix: "",
			ExtraData: map[string]string{
				"service":   "myapp-web",
				"namespace": "default",
			},
			Addresses: []*url.URL{
				{
					Scheme: "http",
					Host:   "192.168.99.1:30000",
				},
			},
		},
		{
			Prefix: "other.process",
			ExtraData: map[string]string{
				"service":   "myapp-other",
				"namespace": "default",
			},
			Addresses: []*url.URL{
				{
					Scheme: "http",
					Host:   "192.168.99.1:30000",
				},
			},
		},
		{
			Prefix: "web.process",
			ExtraData: map[string]string{
				"service":   "myapp-web",
				"namespace": "default",
			},
			Addresses: []*url.URL{
				{
					Scheme: "http",
					Host:   "192.168.99.1:30000",
				},
			},
		},
	}
	require.EqualValues(s.t, expected, addrs)
}

func (s *S) TestProvisionerRoutableAddresses(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()
	addrs, err := s.p.RoutableAddresses(context.TODO(), a)
	require.NoError(s.t, err)
	sort.Slice(addrs, func(i, j int) bool {
		return addrs[i].Prefix < addrs[j].Prefix
	})
	expected := []appTypes.RoutableAddresses{
		{
			Prefix: "",
			ExtraData: map[string]string{
				"service":   "myapp-web",
				"namespace": "default",
			},
			Addresses: []*url.URL{
				{
					Scheme: "http",
					Host:   "192.168.99.1:30000",
				},
			},
		},
		{
			Prefix: "web.process",
			ExtraData: map[string]string{
				"service":   "myapp-web",
				"namespace": "default",
			},
			Addresses: []*url.URL{
				{
					Scheme: "http",
					Host:   "192.168.99.1:30000",
				},
			},
		},
	}
	require.EqualValues(s.t, expected, addrs)
}

func (s *S) TestDeploy(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run mycmd arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	require.Equal(s.t, "tsuru/app-myapp:v1", img)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, deps.Items, 1)
	require.Equal(s.t, "myapp-web", deps.Items[0].Name)
	containers := deps.Items[0].Spec.Template.Spec.Containers
	require.Len(s.t, containers, 1)
	require.EqualValues(s.t, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; exec run mycmd arg1",
	}, containers[0].Command[len(containers[0].Command)-3:])
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, appList.Items, 1)
	require.EqualValues(s.t, tsuruv1.AppSpec{
		NamespaceName:        "default",
		ServiceAccountName:   "app-myapp",
		Deployments:          map[string][]string{"web": {"myapp-web"}},
		Services:             map[string][]string{"web": {"myapp-web", "myapp-web-units"}},
		PodDisruptionBudgets: map[string][]string{"web": {"myapp-web"}},
	}, appList.Items[0].Spec)
}

func (s *S) TestDeployCreatesAppCR(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Destroy(context.TODO(), a)
	require.NoError(s.t, err)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
}

func (s *S) TestDeployWithPoolNamespaces(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	var counter int32
	s.client.PrependReactor("create", "namespaces", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		new := atomic.AddInt32(&counter, 1)
		ns, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Namespace)
		require.True(s.t, ok)
		if new == 2 {
			require.Equal(s.t, "tsuru-test-default", ns.ObjectMeta.Name)
		} else {
			require.Equal(s.t, s.client.Namespace(), ns.ObjectMeta.Name)
		}
		return false, nil, nil
	})
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	require.Equal(s.t, "tsuru/app-myapp:v1", img)
	wait()
	require.Equal(s.t, int32(3), atomic.LoadInt32(&counter))
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, appList.Items, 1)
	require.EqualValues(s.t, tsuruv1.AppSpec{
		NamespaceName:        "tsuru-test-default",
		ServiceAccountName:   "app-myapp",
		Deployments:          map[string][]string{"web": {"myapp-web"}},
		Services:             map[string][]string{"web": {"myapp-web", "myapp-web-units"}},
		PodDisruptionBudgets: map[string][]string{"web": {"myapp-web"}},
	}, appList.Items[0].Spec)
}

func (s *S) TestInternalAddresses(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	s.client.PrependReactor("create", "services", func(action ktesting.Action) (bool, runtime.Object, error) {
		srv := action.(ktesting.CreateAction).GetObject().(*apiv1.Service)
		if srv.Name == "myapp-web" {
			srv.Spec.Ports = []apiv1.ServicePort{
				{
					Port:       int32(80),
					TargetPort: intstr.FromInt(8080),
					NodePort:   int32(30002),
					Protocol:   "TCP",
				},
				{
					Port:       int32(443),
					TargetPort: intstr.FromInt(8443),
					NodePort:   int32(30003),
					Protocol:   "TCP",
				},
			}
		} else if srv.Name == "myapp-jobs" {
			srv.Spec.Ports = []apiv1.ServicePort{
				{
					Port:       int32(12201),
					TargetPort: intstr.FromInt(12201),
					NodePort:   int32(30004),
					Protocol:   "UDP",
				},
			}
		}

		return false, nil, nil
	})

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web":  {"run", "mycmd", "web"},
		"jobs": {"run", "mycmd", "jobs"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)

	addrs, err := s.p.InternalAddresses(context.Background(), a)
	require.NoError(s.t, err)
	wait()

	require.EqualValues(s.t, []appTypes.AppInternalAddress{
		{Domain: "myapp-web.default.svc.cluster.local", Protocol: "TCP", Port: 80, TargetPort: 8080, Process: "web"},
		{Domain: "myapp-web.default.svc.cluster.local", Protocol: "TCP", Port: 443, TargetPort: 8443, Process: "web"},
		{Domain: "myapp-jobs.default.svc.cluster.local", Protocol: "UDP", Port: 12201, TargetPort: 12201, Process: "jobs"},
	}, addrs)
}

func (s *S) TestInternalAddressesNoService(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	processes := map[string][]string{
		"web": {"run", "mycmd", "web"},
	}
	customData := map[string]interface{}{
		"kubernetes": map[string]interface{}{
			"groups": map[string]interface{}{
				"pod1": map[string]interface{}{
					"web": map[string]interface{}{
						"ports": []interface{}{},
					},
				},
			},
		},
	}
	version := newCommittedVersion(c, a, processes, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)

	addrs, err := s.p.InternalAddresses(context.Background(), a)
	require.NoError(s.t, err)
	wait()

	require.Len(s.t, addrs, 0)
}

func (s *S) TestDeployWithCustomConfig(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	processes := map[string][]string{
		"web": {"run mycmd arg1"},
	}
	customData := map[string]interface{}{
		"kubernetes": map[string]interface{}{
			"groups": map[string]interface{}{
				"pod1": map[string]interface{}{
					"web": map[string]interface{}{
						"ports": []interface{}{
							map[string]interface{}{
								"name": "my-port",
								"port": 9000,
							},
							map[string]interface{}{
								"protocol":    "tcp",
								"target_port": 8080,
							},
						},
					},
				},
				"pod2": map[string]interface{}{
					"proc2": map[string]interface{}{},
				},
				"pod3": map[string]interface{}{
					"proc3": map[string]interface{}{
						"ports": []interface{}{
							map[string]interface{}{
								"protocol":    "udp",
								"port":        9000,
								"target_port": 9001,
							},
						},
					},
				},
			},
		},
	}
	version := newCommittedVersion(c, a, processes, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	require.Equal(s.t, "tsuru/app-myapp:v1", img)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, deps.Items, 1)
	require.Equal(s.t, "myapp-web", deps.Items[0].Name)
	containers := deps.Items[0].Spec.Template.Spec.Containers
	require.Len(s.t, containers, 1)
	require.EqualValues(s.t, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; exec run mycmd arg1",
	}, containers[0].Command[len(containers[0].Command)-3:])
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, appList.Items, 1)
	expected := tsuruv1.AppSpec{
		NamespaceName:        "default",
		ServiceAccountName:   "app-myapp",
		Deployments:          map[string][]string{"web": {"myapp-web"}},
		Services:             map[string][]string{"web": {"myapp-web", "myapp-web-units"}},
		PodDisruptionBudgets: map[string][]string{"web": {"myapp-web"}},
		Configs: &provTypes.TsuruYamlKubernetesConfig{
			Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
				"pod1": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"web": {
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
							{Name: "my-port", Protocol: "TCP", Port: 9000, TargetPort: 9000},
							{Name: "http-default-2", Protocol: "TCP", Port: 8080, TargetPort: 8080},
						},
					},
				},
				"pod2": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"proc2": {
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{},
					},
				},
				"pod3": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
					"proc3": {
						Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
							{Name: "udp-default-1", Protocol: "UDP", Port: 9000, TargetPort: 9001},
						},
					},
				},
			},
		},
	}
	require.EqualValues(s.t, expected, appList.Items[0].Spec)
}

func (s *S) TestDeployRollback(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	deployEvt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run mycmd arg1"},
	}
	version1 := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: deployEvt})
	require.NoError(s.t, err)
	require.Equal(s.t, "tsuru/app-myapp:v1", img)
	customData = map[string][]string{
		"web": {"run mycmd arg2"},
	}
	version2 := newCommittedVersion(c, a, customData)
	img, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version2, Event: deployEvt})
	require.NoError(s.t, err)
	require.Equal(s.t, "tsuru/app-myapp:v2", img)
	deployEvt.Done(context.TODO(), err)
	require.NoError(s.t, err)
	rollbackEvt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeployRollback,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeployRollback),
	})
	require.NoError(s.t, err)
	img, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: rollbackEvt})
	require.NoError(s.t, err)
	testBaseImage, err := version1.BaseImageName()
	require.NoError(s.t, err)
	require.Equal(s.t, testBaseImage, img)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, deps.Items, 1)
	require.Equal(s.t, "myapp-web", deps.Items[0].Name)
	containers := deps.Items[0].Spec.Template.Spec.Containers
	require.Len(s.t, containers, 1)
	require.EqualValues(s.t, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; exec run mycmd arg1",
	}, containers[0].Command[len(containers[0].Command)-3:])
	units, err := s.p.Units(context.TODO(), a)
	require.NoError(s.t, err)
	require.Len(s.t, units, 1)
}

func (s *S) TestExecuteCommandWithStdin(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	s.mock.HandleSize = true
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdin:  conn,
		Stdout: conn,
		Stderr: conn,
		Width:  99,
		Height: 42,
		Term:   "xterm",
		Units:  []string{"myapp-web-pod-1-1"},
		Cmds:   []string{"mycmd", "arg1"},
	})
	require.NoError(s.t, err)
	rollback()
	require.Equal(s.t, "echo test", s.mock.Stream["myapp-web"].Stdin)
	var sz remotecommand.TerminalSize
	err = json.Unmarshal([]byte(s.mock.Stream["myapp-web"].Resize), &sz)
	require.NoError(s.t, err)
	require.EqualValues(s.t, remotecommand.TerminalSize{Width: 99, Height: 42}, sz)
	require.Len(s.t, s.mock.Stream["myapp-web"].Urls, 1)
	require.Equal(s.t, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec", s.mock.Stream["myapp-web"].Urls[0].Path)
	require.EqualValues(s.t, []string{"/usr/bin/env", "TERM=xterm", "mycmd", "arg1"}, s.mock.Stream["myapp-web"].Urls[0].Query()["command"])
}

func (s *S) TestExecuteCommandWithStdinNoSize(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	buf := safe.NewBuffer([]byte("echo test"))
	conn := &provisiontest.FakeConn{Buf: buf}
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdin:  conn,
		Stdout: conn,
		Stderr: conn,
		Term:   "xterm",
		Units:  []string{"myapp-web-pod-1-1"},
		Cmds:   []string{"mycmd", "arg1"},
	})
	require.NoError(s.t, err)
	rollback()
	require.Equal(s.t, "echo test", s.mock.Stream["myapp-web"].Stdin)
	require.Len(s.t, s.mock.Stream["myapp-web"].Urls, 1)
	require.Equal(s.t, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec", s.mock.Stream["myapp-web"].Urls[0].Path)
	require.EqualValues(s.t, []string{"/usr/bin/env", "TERM=xterm", "mycmd", "arg1"}, s.mock.Stream["myapp-web"].Urls[0].Query()["command"])
}

func (s *S) TestExecuteCommandUnitNotFound(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	buf := bytes.NewBuffer(nil)
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: buf,
		Width:  99,
		Height: 42,
		Units:  []string{"invalid-unit"},
	})
	require.Error(s.t, err)
	expectedError := &provision.UnitNotFoundError{ID: "invalid-unit"}
	require.ErrorContains(s.t, err, expectedError.Error())
}

func (s *S) TestExecuteCommand(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 2, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Units:  []string{"myapp-web-pod-1-1", "myapp-web-pod-2-2"},
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	require.NoError(s.t, err)
	rollback()
	require.Equal(s.t, "stdout datastdout data", stdout.String())
	require.Equal(s.t, "stderr datastderr data", stderr.String())
	require.Len(s.t, s.mock.Stream["myapp-web"].Urls, 2)
	require.Equal(s.t, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec", s.mock.Stream["myapp-web"].Urls[0].Path)
	require.Equal(s.t, "/api/v1/namespaces/default/pods/myapp-web-pod-2-2/exec", s.mock.Stream["myapp-web"].Urls[1].Path)
	require.Equal(s.t, []string{"mycmd", "arg1", "arg2"}, s.mock.Stream["myapp-web"].Urls[0].Query()["command"])
	require.Equal(s.t, []string{"mycmd", "arg1", "arg2"}, s.mock.Stream["myapp-web"].Urls[1].Query()["command"])
}

func (s *S) TestExecuteCommandSingleUnit(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 2, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Units:  []string{"myapp-web-pod-1-1"},
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	require.NoError(s.t, err)
	rollback()
	require.Equal(s.t, "stdout data", stdout.String())
	require.Equal(s.t, "stderr data", stderr.String())
	require.Len(s.t, s.mock.Stream["myapp-web"].Urls, 1)
	require.Equal(s.t, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec", s.mock.Stream["myapp-web"].Urls[0].Path)
	require.EqualValues(s.t, []string{"mycmd", "arg1", "arg2"}, s.mock.Stream["myapp-web"].Urls[0].Query()["command"])
}

func (s *S) TestExecuteCommandNoUnits(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err := s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	require.NoError(s.t, err)
	require.Equal(s.t, "stdout data", stdout.String())
	require.Equal(s.t, "stderr data", stderr.String())
	require.Len(s.t, s.mock.Stream["myapp-isolated-run"].Urls, 1)
	require.Equal(s.t, "/api/v1/namespaces/default/pods/myapp-isolated-run/attach", s.mock.Stream["myapp-isolated-run"].Urls[0].Path)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	pods, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pods.Items, 0)
	account, err := s.client.CoreV1().ServiceAccounts(ns).Get(context.TODO(), "app-myapp", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-myapp",
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-tsuru": "true",
				"tsuru.io/app-name": "myapp",
			},
		},
	}, account)
}

func (s *S) TestExecuteCommandNoUnitsCheckPodRequirements(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	a.Plan.CPUMilli = 250000
	a.Plan.Memory = 100000
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	shouldFail := true
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		shouldFail = false
		var ephemeral resource.Quantity
		ephemeral, err = s.clusterClient.ephemeralStorage(a.Pool)
		require.NoError(s.t, err)
		expectedLimits := &apiv1.ResourceList{
			apiv1.ResourceMemory:           *resource.NewQuantity(a.Plan.Memory, resource.BinarySI),
			apiv1.ResourceCPU:              *resource.NewMilliQuantity(int64(a.Plan.CPUMilli), resource.DecimalSI),
			apiv1.ResourceEphemeralStorage: ephemeral,
		}
		expectedRequests := &apiv1.ResourceList{
			apiv1.ResourceMemory:           *resource.NewQuantity(a.Plan.Memory, resource.BinarySI),
			apiv1.ResourceCPU:              *resource.NewMilliQuantity(int64(a.Plan.CPUMilli), resource.DecimalSI),
			apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
		}
		require.EqualValues(s.t, *expectedLimits, pod.Spec.Containers[0].Resources.Limits)
		require.EqualValues(s.t, *expectedRequests, pod.Spec.Containers[0].Resources.Requests)

		return false, nil, nil
	})
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	require.False(s.t, shouldFail)
	require.NoError(s.t, err)
}

func (s *S) TestExecuteCommandNoUnitsPodFailed(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		require.True(s.t, ok)
		pod.Status.Phase = apiv1.PodFailed
		return false, nil, nil
	})
	newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err := s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	require.ErrorContains(s.t, err, `invalid pod phase "Failed"`)
}

func (s *S) TestExecuteCommandIsolatedWithoutNodeSelector(c *check.C) {
	s.clusterClient.CustomData["disable-default-node-selector"] = "true"
	defer delete(s.clusterClient.CustomData, "disable-default-node-selector")
	s.mock.IgnorePool = true
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	var checked bool
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		require.Equal(s.t, "myapp", pod.Labels["tsuru.io/app-name"])
		require.Equal(s.t, "true", pod.Labels["tsuru.io/is-isolated-run"])
		require.Nil(s.t, pod.Spec.NodeSelector)
		require.Nil(s.t, pod.Spec.Affinity)
		checked = true
		return false, nil, nil
	})
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Cmds:   []string{"sh", "-l"},
	})
	require.NoError(s.t, err)
	require.True(s.t, checked)
}

func (s *S) TestStartupMessage(c *check.C) {
	msg, err := s.p.StartupMessage()
	require.NoError(s.t, err)
	require.Equal(s.t, `Kubernetes provisioner on cluster "c1" - https://clusteraddr
`, msg)
	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return nil, nil
	}
	msg, err = s.p.StartupMessage()
	require.NoError(s.t, err)
	require.Zero(s.t, msg)
}

func (s *S) TestGetKubeConfig(c *check.C) {
	config.Set("kubernetes:api-timeout", 10)
	config.Set("kubernetes:pod-ready-timeout", 6)
	config.Set("kubernetes:pod-running-timeout", 2*60)
	config.Set("kubernetes:deployment-progress-timeout", 3*60)
	config.Set("kubernetes:attach-after-finish-timeout", 5)
	config.Set("kubernetes:headless-service-port", 8889)
	defer config.Unset("kubernetes")
	kubeConf := getKubeConfig()
	require.EqualValues(s.t, kubernetesConfig{
		APITimeout:                          10 * time.Second,
		PodReadyTimeout:                     6 * time.Second,
		PodRunningTimeout:                   2 * time.Minute,
		DeploymentProgressTimeout:           3 * time.Minute,
		AttachTimeoutAfterContainerFinished: 5 * time.Second,
		HeadlessServicePort:                 8889,
	}, kubeConf)
}

func (s *S) TestGetKubeConfigDefaults(c *check.C) {
	config.Unset("kubernetes")
	kubeConf := getKubeConfig()
	require.EqualValues(s.t, kubernetesConfig{
		APITimeout:                          60 * time.Second,
		PodReadyTimeout:                     time.Minute,
		PodRunningTimeout:                   10 * time.Minute,
		DeploymentProgressTimeout:           10 * time.Minute,
		AttachTimeoutAfterContainerFinished: time.Minute,
		HeadlessServicePort:                 8888,
	}, kubeConf)
}

func (s *S) TestProvisionerProvision(c *check.C) {
	_, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	crdList, err := s.client.ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, crdList.Items, 1)
	require.Equal(s.t, "apps.tsuru.io", crdList.Items[0].Name)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, appList.Items, 1)
	require.Equal(s.t, a.Name, appList.Items[0].Name)
	require.Equal(s.t, "default", appList.Items[0].Spec.NamespaceName)
}

func (s *S) TestProvisionerUpdateApp(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	require.NoError(s.t, err)
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	rebuild.Initialize(func(appName string) (*appTypes.App, error) {
		return &appTypes.App{
			Name:    appName,
			Pool:    "test-pool-2",
			Routers: a.Routers,
		}, nil
	})
	require.NoError(s.t, err)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	require.Equal(s.t, "tsuru/app-myapp:v1", img)
	wait()
	sList, err := s.client.CoreV1().Services("tsuru-test-pool-2").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, sList.Items, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, sList.Items, 2)
	newApp := provisiontest.NewFakeAppWithPool(a.Name, a.Platform, "test-pool-2", 0)
	buf := new(bytes.Buffer)
	var recreatedPods bool
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		require.EqualValues(s.t, map[string]string{
			"tsuru.io/pool": newApp.Pool,
		}, pod.Spec.NodeSelector)
		require.Equal(s.t, newApp.Pool, pod.ObjectMeta.Labels["tsuru.io/app-pool"])
		recreatedPods = true
		return true, nil, nil
	})
	s.client.PrependReactor("create", "services", func(action ktesting.Action) (bool, runtime.Object, error) {
		srv := action.(ktesting.CreateAction).GetObject().(*apiv1.Service)
		srv.Spec.Ports = []apiv1.ServicePort{
			{
				NodePort: int32(30002),
			},
		}
		return false, nil, nil
	})
	_, err = s.client.CoreV1().Nodes().Create(context.TODO(), &apiv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-test-pool-2",
			Labels: map[string]string{"tsuru.io/pool": "test-pool-2"},
		},
		Status: apiv1.NodeStatus{
			Addresses: []apiv1.NodeAddress{{Type: apiv1.NodeInternalIP, Address: "192.168.100.1"}},
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	err = s.p.UpdateApp(context.TODO(), a, newApp, buf)
	require.NoError(s.t, err)
	require.Contains(s.t, buf.String(), "All units ready")
	require.True(s.t, recreatedPods)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, appList.Items, 1)
	require.Equal(s.t, a.Name, appList.Items[0].Name)
	require.Equal(s.t, "tsuru-test-pool-2", appList.Items[0].Spec.NamespaceName)
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, sList.Items, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-pool-2").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, sList.Items, 2)
}

func (s *S) TestProvisionerUpdateAppCanaryDeploy(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	require.NoError(s.t, err)
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	rebuild.Initialize(func(appName string) (*appTypes.App, error) {
		return &appTypes.App{
			Name:    appName,
			Pool:    "test-pool-2",
			Routers: a.Routers,
		}, nil
	})
	require.NoError(s.t, err)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	{
		customData := map[string][]string{
			"web": {"run", "mycmd", "arg1"},
		}
		version1 := newCommittedVersion(c, a, customData)
		var img1 string
		img1, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: evt})
		require.NoError(s.t, err)
		require.Equal(s.t, "tsuru/app-myapp:v1", img1)
		wait()
		version2 := newCommittedVersion(c, a, customData)
		var img2 string
		img2, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version2, Event: evt, PreserveVersions: true})
		require.NoError(s.t, err)
		require.Equal(s.t, "tsuru/app-myapp:v2", img2)
		wait()
	}
	sList, err := s.client.CoreV1().Services("tsuru-test-pool-2").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, sList.Items, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, sList.Items, 4)
	contains := func(svcList []apiv1.Service, name string) bool {
		for _, svc := range svcList {
			if svc.Name == name {
				return true
			}
		}
		return false
	}
	require.True(s.t, contains(sList.Items, "myapp-web"))
	require.True(s.t, contains(sList.Items, "myapp-web-units"))
	require.True(s.t, contains(sList.Items, "myapp-web-v1"))
	require.True(s.t, contains(sList.Items, "myapp-web-v2"))
	newApp := provisiontest.NewFakeAppWithPool(a.Name, a.Platform, "test-pool-2", 0)
	buf := new(bytes.Buffer)
	err = s.p.UpdateApp(context.TODO(), a, newApp, buf)
	require.Error(s.t, err)
	expectedError := &tsuruErrors.ValidationError{Message: "can't provision new app with multiple versions, please unify them and try again"}
	require.ErrorContains(s.t, err, expectedError.Error())
}

func (s *S) TestProvisionerUpdateAppCanaryDeployWithStoppedBaseDep(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	require.NoError(s.t, err)
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	rebuild.Initialize(func(appName string) (*appTypes.App, error) {
		return &appTypes.App{
			Name:    appName,
			Pool:    "test-pool-2",
			Routers: a.Routers,
		}, nil
	})
	require.NoError(s.t, err)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version1 := newCommittedVersion(c, a, customData)
	img1, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: evt})
	require.NoError(s.t, err)
	require.Equal(s.t, "tsuru/app-myapp:v1", img1)
	wait()
	version2 := newCommittedVersion(c, a, customData)
	img2, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version2, Event: evt, PreserveVersions: true})
	require.NoError(s.t, err)
	require.Equal(s.t, "tsuru/app-myapp:v2", img2)
	wait()

	sList, err := s.client.CoreV1().Services("tsuru-test-pool-2").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, sList.Items, 0)
	newApp := provisiontest.NewFakeAppWithPool(a.Name, a.Platform, "test-pool-2", 0)
	buf := new(bytes.Buffer)
	err = s.p.Stop(context.TODO(), a, "", version1, buf)
	require.NoError(s.t, err)
	contains := func(depList []appsv1.Deployment, name string) bool {
		for _, dep := range depList {
			if dep.Name == name {
				return true
			}
		}
		return false
	}
	replicaCount := func(depList []appsv1.Deployment, name string, expectedReplicas int) bool {
		for _, dep := range depList {
			if dep.Name == name {
				return *dep.Spec.Replicas == int32(expectedReplicas)
			}
		}
		return false
	}
	depList, err := s.client.AppsV1().Deployments("tsuru-test-default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, depList.Items, 2)
	require.True(s.t, contains(depList.Items, "myapp-web-v2"))
	require.True(s.t, contains(depList.Items, "myapp-web"))
	require.True(s.t, replicaCount(depList.Items, "myapp-web", 0))
	require.True(s.t, replicaCount(depList.Items, "myapp-web-v2", 1))
	err = s.p.UpdateApp(context.TODO(), a, newApp, buf)
	require.Error(s.t, err)
	expectedError := &tsuruErrors.ValidationError{Message: "can't provision new app with multiple versions, please unify them and try again"}
	require.ErrorContains(s.t, err, expectedError.Error())
}

func (s *S) TestProvisionerUpdateAppWithCanaryOtherCluster(c *check.C) {
	client1, client2, _ := s.prepareMultiCluster(c)
	s.client = client1
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	s.factory = informers.NewSharedInformerFactory(s.client, s.defaultSharedInformerDuration)
	s.mock = kTesting.NewKubeMock(s.client, s.p, s.p, s.factory)
	a, wait, rollback := s.mock.NoNodeReactions(c)
	defer rollback()

	pool2 := client2.GetCluster().Pools[0]
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        pool2,
		Provisioner: "kubernetes",
	})
	require.NoError(s.t, err)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	{
		customData := map[string][]string{
			"web": {"run", "mycmd", "arg1"},
		}
		version1 := newCommittedVersion(c, a, customData)
		var img1 string
		img1, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: evt})
		require.NoError(s.t, err)
		require.Equal(s.t, "tsuru/app-myapp:v1", img1)
		wait()
		customData = map[string][]string{
			"web": {"run", "mycmd", "arg1"},
		}
		version2 := newCommittedVersion(c, a, customData)
		var img2 string
		img2, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version2, Event: evt, PreserveVersions: true})
		require.NoError(s.t, err)
		require.Equal(s.t, "tsuru/app-myapp:v2", img2)
		wait()
	}

	newApp := provisiontest.NewFakeAppWithPool(a.Name, a.Platform, pool2, 0)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		require.EqualValues(s.t, map[string]string{"tsuru.io/pool": newApp.Pool}, pod.Spec.NodeSelector)
		require.Equal(s.t, newApp.Pool, pod.ObjectMeta.Labels["tsuru.io/app-pool"])
		return true, nil, nil
	})
	err = s.p.UpdateApp(context.TODO(), a, newApp, new(bytes.Buffer))
	require.Error(s.t, err)
	expectedError := &tsuruErrors.ValidationError{Message: "can't provision new app with multiple versions, please unify them and try again"}
	require.ErrorContains(s.t, err, expectedError.Error())
}

func (s *S) TestProvisionerUpdateAppWithVolumeSameClusterAndNamespace(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	require.NoError(s.t, err)
	config.Set("kubernetes:use-pool-namespaces", false)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	require.Equal(s.t, "tsuru/app-myapp:v1", img)
	wait()
	sList, err := s.client.CoreV1().Services("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, sList.Items, 2)
	newApp := provisiontest.NewFakeAppWithPool(a.Name, a.Platform, "test-pool-2", 0)
	buf := new(bytes.Buffer)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		require.EqualValues(s.t, map[string]string{"tsuru.io/pool": newApp.Pool}, pod.Spec.NodeSelector)
		require.Equal(s.t, newApp.Pool, pod.ObjectMeta.Labels["tsuru.io/app-pool"])
		return true, nil, nil
	})
	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan: volumeTypes.VolumePlan{Name: "p1", Opts: map[string]interface{}{
			"plugin": "emptyDir",
		}},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	err = s.p.UpdateApp(context.TODO(), a, newApp, buf)
	require.NoError(s.t, err)
}

func (s *S) TestProvisionerUpdateAppWithVolumeSameClusterOtherNamespace(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	require.NoError(s.t, err)
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	require.Equal(s.t, "tsuru/app-myapp:v1", img)
	wait()
	sList, err := s.client.CoreV1().Services("tsuru-test-pool-2").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, sList.Items, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, sList.Items, 2)
	newApp := provisiontest.NewFakeAppWithPool(a.Name, a.Platform, "test-pool-2", 0)
	buf := new(bytes.Buffer)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		require.EqualValues(s.t, map[string]string{"tsuru.io/pool": newApp.Pool}, pod.Spec.NodeSelector)
		require.Equal(s.t, newApp.Pool, pod.ObjectMeta.Labels["tsuru.io/app-pool"])
		return true, nil, nil
	})
	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan: volumeTypes.VolumePlan{Name: "p1", Opts: map[string]interface{}{
			"storage-class": "my-storageclass",
		}},
		Pool:      "test-default",
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	err = s.p.UpdateApp(context.TODO(), a, newApp, buf)
	require.Error(s.t, err)
	require.ErrorContains(s.t, err, "can't change the pool of an app with binded volumes")
}

func (s *S) TestProvisionerUpdateAppWithVolumeOtherCluster(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	client1, client2, _ := s.prepareMultiCluster(c)
	s.client = client1
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	s.factory = informers.NewSharedInformerFactory(s.client, s.defaultSharedInformerDuration)
	s.mock = kTesting.NewKubeMock(s.client, s.p, s.p, s.factory)
	_, _, rollback1 := s.mock.NoNodeReactions(c)
	defer rollback1()

	pool2 := client2.GetCluster().Pools[0]
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        pool2,
		Provisioner: "kubernetes",
	})
	require.NoError(s.t, err)
	s.client = client2
	s.factory = informers.NewSharedInformerFactory(s.client, s.defaultSharedInformerDuration)
	s.mock = kTesting.NewKubeMock(s.client, s.p, s.p, s.factory)
	s.mock.IgnorePool = true
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan: volumeTypes.VolumePlan{Name: "p1", Opts: map[string]interface{}{
			"storage-class": "mystorage-class",
		}},
		Pool:      a.Pool,
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt1",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt2",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	_, _, err = createVolumesForApp(context.TODO(), client1.ClusterInterface.(*ClusterClient), a)
	require.NoError(s.t, err)

	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newSuccessfulVersion(c, a, customData)
	newApp := provisiontest.NewFakeAppWithPool(a.Name, a.Platform, pool2, 0)
	pvcs, err := client1.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pvcs.Items, 1)

	err = s.p.Restart(context.TODO(), a, "web", version, nil)
	require.NoError(s.t, err)

	err = s.p.UpdateApp(context.TODO(), a, newApp, new(bytes.Buffer))
	require.NoError(s.t, err)
	// Check if old volume was removed
	pvcs, err = client1.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pvcs.Items, 0)
	// Check if new volume was created
	pvcs, err = client2.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pvcs.Items, 1)
}

func (s *S) TestProvisionerUpdateAppWithVolumeWithTwoBindsOtherCluster(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	client1, client2, _ := s.prepareMultiCluster(c)
	s.client = client1
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	s.factory = informers.NewSharedInformerFactory(s.client, s.defaultSharedInformerDuration)
	s.mock = kTesting.NewKubeMock(s.client, s.p, s.p, s.factory)
	_, _, rollback1 := s.mock.NoNodeReactions(c)
	defer rollback1()

	pool2 := client2.GetCluster().Pools[0]
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        pool2,
		Provisioner: "kubernetes",
	})
	require.NoError(s.t, err)
	s.client = client2
	s.factory = informers.NewSharedInformerFactory(s.client, s.defaultSharedInformerDuration)
	s.mock = kTesting.NewKubeMock(s.client, s.p, s.p, s.factory)
	s.mock.IgnorePool = true
	s.mock.IgnoreAppName = true
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan: volumeTypes.VolumePlan{Name: "p1", Opts: map[string]interface{}{
			"storage-class": "myclass",
		}},
		Pool:      a.Pool,
		TeamOwner: "admin",
	}
	err = servicemanager.Volume.Create(context.TODO(), &v)
	require.NoError(s.t, err)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	_, _, err = createVolumesForApp(context.TODO(), client1.ClusterInterface.(*ClusterClient), a)
	require.NoError(s.t, err)
	a2 := provisiontest.NewFakeApp("myapp2", "python", 0)
	err = s.p.Provision(context.TODO(), a2)
	require.NoError(s.t, err)
	client1.TsuruClientset.PrependReactor("create", "apps", s.mock.AppReaction(a2, c))
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a2.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	require.NoError(s.t, err)
	_, _, err = createVolumesForApp(context.TODO(), client1.ClusterInterface.(*ClusterClient), a2)
	require.NoError(s.t, err)

	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newSuccessfulVersion(c, a, customData)
	pvcs, err := client1.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pvcs.Items, 1)

	err = s.p.Restart(context.TODO(), a, "web", version, nil)
	require.NoError(s.t, err)

	newApp := provisiontest.NewFakeAppWithPool(a.Name, a.Platform, pool2, 0)
	err = s.p.UpdateApp(context.TODO(), a, newApp, new(bytes.Buffer))
	require.NoError(s.t, err)
	// Check if old volume was not removed
	pvcs, err = client1.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pvcs.Items, 1)
	// Check if new volume was created
	pvcs, err = client2.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pvcs.Items, 1)
}

func (s *S) TestProvisionerInitialize(c *check.C) {
	_, ok := s.p.clusterControllers[s.clusterClient.Name]
	require.False(s.t, ok)
	err := s.p.Initialize()
	require.NoError(s.t, err)
	_, ok = s.p.clusterControllers[s.clusterClient.Name]
	require.True(s.t, ok)
}

func (s *S) TestProvisionerValidation(c *check.C) {
	err := s.p.ValidateCluster(&provTypes.Cluster{
		Addresses:  []string{"blah"},
		CaCert:     []byte(`fakeca`),
		ClientCert: []byte(`clientcert`),
		ClientKey:  []byte(`clientkey`),
		KubeConfig: &provTypes.KubeConfig{
			Cluster: clientcmdapi.Cluster{},
		},
	})
	require.ErrorContains(s.t, err, "when kubeConfig is set the use of cacert is not used")
	require.ErrorContains(s.t, err, "when kubeConfig is set the use of clientcert is not used")
	require.ErrorContains(s.t, err, "when kubeConfig is set the use of clientkey is not used")

	err = s.p.ValidateCluster(&provTypes.Cluster{
		KubeConfig: &provTypes.KubeConfig{
			Cluster: clientcmdapi.Cluster{},
		},
	})
	require.Error(s.t, err)
	require.ErrorContains(s.t, err, "kubeConfig.cluster.server field is required")
}

func (s *S) TestProvisionerInitializeNoClusters(c *check.C) {
	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return nil, provTypes.ErrNoCluster
	}
	err := s.p.Initialize()
	require.NoError(s.t, err)
}

func (s *S) TestEnvsForAppCustomPorts(c *check.C) {
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"proc1": {"python", "proc1.py"},
		"proc2": {"python", "proc2.py"},
		"proc3": {"python", "proc3.py"},
		"proc4": {"python", "proc4.py"},
		"proc5": {"python", "worker.py"},
		"proc6": {"python", "proc6.py"},
	},
		map[string]interface{}{
			"kubernetes": provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"mypod1": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"proc1": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{TargetPort: 8080},
								{Port: 9000},
							},
						},
					},
					"mypod2": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"proc2": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{TargetPort: 8000},
							},
						},
					},
					"mypod3": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"proc3": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{Port: 8000, TargetPort: 8080},
							},
						},
					},
					"mypod5": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"proc5": {},
						"proc6": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{Port: 8000},
							},
						},
					},
					"mypod6": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"proc6": {},
					},
				},
			},
		})
	require.NoError(s.t, err)
	fa := provisiontest.NewFakeApp("myapp", "java", 1)

	if fa.Env == nil {
		fa.Env = make(map[string]bindTypes.EnvVar, 0)
	}
	fa.Env["e1"] = bindTypes.EnvVar{Name: "e1", Value: "v1"}

	envs := EnvsForApp(fa, "proc1", version)
	require.EqualValues(s.t, []bindTypes.EnvVar{
		{Name: "TSURU_APPDIR", Value: "/home/application/current", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_APPNAME", Value: "myapp", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_SERVICES", Value: "{}", ManagedBy: "tsuru"},
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc1", Public: true},
		{Name: "TSURU_APPVERSION", Value: "1", Public: true},
		{Name: "TSURU_HOST", Value: "", Public: true},
		{Name: "PORT_proc1", Value: "8080,9000", Public: true},
	}, envs)

	envs = EnvsForApp(fa, "proc2", version)
	require.EqualValues(s.t, []bindTypes.EnvVar{
		{Name: "TSURU_APPDIR", Value: "/home/application/current", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_APPNAME", Value: "myapp", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_SERVICES", Value: "{}", ManagedBy: "tsuru"},
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc2", Public: true},
		{Name: "TSURU_APPVERSION", Value: "1", Public: true},
		{Name: "TSURU_HOST", Value: "", Public: true},
		{Name: "PORT_proc2", Value: "8000", Public: true},
	}, envs)

	envs = EnvsForApp(fa, "proc3", version)
	require.EqualValues(s.t, []bindTypes.EnvVar{
		{Name: "TSURU_APPDIR", Value: "/home/application/current", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_APPNAME", Value: "myapp", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_SERVICES", Value: "{}", ManagedBy: "tsuru"},
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc3", Public: true},
		{Name: "TSURU_APPVERSION", Value: "1", Public: true},
		{Name: "TSURU_HOST", Value: "", Public: true},
		{Name: "PORT_proc3", Value: "8080", Public: true},
	}, envs)

	envs = EnvsForApp(fa, "proc4", version)
	require.EqualValues(s.t, []bindTypes.EnvVar{
		{Name: "TSURU_APPDIR", Value: "/home/application/current", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_APPNAME", Value: "myapp", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_SERVICES", Value: "{}", ManagedBy: "tsuru"},
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc4", Public: true},
		{Name: "TSURU_APPVERSION", Value: "1", Public: true},
		{Name: "TSURU_HOST", Value: "", Public: true},
		{Name: "port", Value: "8888", Public: true},
		{Name: "PORT", Value: "8888", Public: true},
		{Name: "PORT_proc4", Value: "8888", Public: true},
	}, envs)

	envs = EnvsForApp(fa, "proc5", version)
	require.EqualValues(s.t, []bindTypes.EnvVar{
		{Name: "TSURU_APPDIR", Value: "/home/application/current", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_APPNAME", Value: "myapp", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_SERVICES", Value: "{}", ManagedBy: "tsuru"},
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc5", Public: true},
		{Name: "TSURU_APPVERSION", Value: "1", Public: true},
		{Name: "TSURU_HOST", Value: "", Public: true},
	}, envs)

	envs = EnvsForApp(fa, "proc6", version)
	require.EqualValues(s.t, []bindTypes.EnvVar{
		{Name: "TSURU_APPDIR", Value: "/home/application/current", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_APPNAME", Value: "myapp", ManagedBy: "tsuru", Public: true},
		{Name: "TSURU_SERVICES", Value: "{}", ManagedBy: "tsuru"},
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc6", Public: true},
		{Name: "TSURU_APPVERSION", Value: "1", Public: true},
		{Name: "TSURU_HOST", Value: "", Public: true},
		{Name: "port", Value: "8888", Public: true},
		{Name: "PORT", Value: "8888", Public: true},
	}, envs)
}

func (s *S) TestProvisionerToggleRoutable(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	err = s.p.ToggleRoutable(context.TODO(), a, version, false)
	require.NoError(s.t, err)
	wait()

	dep, err := s.client.AppsV1().Deployments("default").Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.False(s.t, dep.Spec.Paused)
	require.Equal(s.t, "false", dep.Labels["tsuru.io/is-routable"])
	require.Equal(s.t, "false", dep.Spec.Template.Labels["tsuru.io/is-routable"])

	rsList, err := s.client.AppsV1().ReplicaSets("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	for _, rs := range rsList.Items {
		require.Equal(s.t, "false", rs.Labels["tsuru.io/is-routable"])
		require.Equal(s.t, "false", rs.Spec.Template.Labels["tsuru.io/is-routable"])
	}

	pods, err := s.client.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pods.Items, 1)
	require.Equal(s.t, "false", pods.Items[0].Labels["tsuru.io/is-routable"])

	err = s.p.ToggleRoutable(context.TODO(), a, version, true)
	require.NoError(s.t, err)
	wait()

	dep, err = s.client.AppsV1().Deployments("default").Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.False(s.t, dep.Spec.Paused)
	require.Equal(s.t, "true", dep.Labels["tsuru.io/is-routable"])
	require.Equal(s.t, "true", dep.Spec.Template.Labels["tsuru.io/is-routable"])

	rsList, err = s.client.AppsV1().ReplicaSets("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	for _, rs := range rsList.Items {
		require.Equal(s.t, "true", rs.Labels["tsuru.io/is-routable"])
		require.Equal(s.t, "true", rs.Spec.Template.Labels["tsuru.io/is-routable"])
	}

	pods, err = s.client.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pods.Items, 1)
	require.Equal(s.t, "true", pods.Items[0].Labels["tsuru.io/is-routable"])
}

func (s *S) TestProvisionerToggleRoutableAtomic(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	dep, err := s.client.AppsV1().Deployments("default").Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)

	dep.Spec.Template.ObjectMeta.Labels["tsuru.io/test-atomic"] = "true"
	_, err = s.client.AppsV1().Deployments("default").Update(context.TODO(), dep, metav1.UpdateOptions{})
	require.NoError(s.t, err)

	err = s.p.ToggleRoutable(context.TODO(), a, version, false)
	require.NoError(s.t, err)
	wait()

	dep, err = s.client.AppsV1().Deployments("default").Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)

	require.Equal(s.t, "true", dep.Spec.Template.ObjectMeta.Labels["tsuru.io/test-atomic"])
}

func (s *S) TestEnsureAppCustomResourceSyncedPreserveAnnotations(c *check.C) {
	ctx := context.TODO()
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(ctx, a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	tclient, err := TsuruClientForConfig(s.clusterClient.restConfig)
	require.NoError(s.t, err)

	foundTsuruApp, err := tclient.TsuruV1().Apps(s.clusterClient.Namespace()).Get(ctx, a.Name, metav1.GetOptions{})
	require.NoError(s.t, err)

	foundTsuruApp.ObjectMeta.Annotations = map[string]string{
		"external.io/teste": "true",
	}

	_, err = tclient.TsuruV1().Apps(s.clusterClient.Namespace()).Update(ctx, foundTsuruApp, metav1.UpdateOptions{})
	require.NoError(s.t, err)

	err = ensureAppCustomResourceSynced(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)

	appCRD, err := tclient.TsuruV1().Apps(s.clusterClient.Namespace()).Get(ctx, a.Name, metav1.GetOptions{})
	require.NoError(s.t, err)

	require.EqualValues(s.t, metav1.ObjectMeta{
		Namespace: "tsuru",
		Name:      "myapp",
		Annotations: map[string]string{
			"external.io/teste": "true",
		},
	}, appCRD.ObjectMeta)

	require.EqualValues(s.t, tsuruv1.AppSpec{
		NamespaceName:      "default",
		ServiceAccountName: "app-myapp",
		Deployments: map[string][]string{
			"web": {"myapp-web"},
		},
		Services: map[string][]string{
			"web": {"myapp-web", "myapp-web-units"},
		},
		PodDisruptionBudgets: map[string][]string{
			"web": {"myapp-web"},
		},
	}, appCRD.Spec)
}

func (s *S) TestEnsureAppCustomResourceSyncedPreserveAnnotations2(c *check.C) {
	ctx := context.TODO()
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(ctx, a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	tclient, err := TsuruClientForConfig(s.clusterClient.restConfig)
	require.NoError(s.t, err)

	foundTsuruApp, err := tclient.TsuruV1().Apps(s.clusterClient.Namespace()).Get(ctx, a.Name, metav1.GetOptions{})
	require.NoError(s.t, err)

	foundTsuruApp.ObjectMeta.Annotations = map[string]string{
		"external.io/teste": "true",
	}
	foundTsuruApp.Spec.ServiceAccountName = "another" // this is the unique line different from previous test

	_, err = tclient.TsuruV1().Apps(s.clusterClient.Namespace()).Update(ctx, foundTsuruApp, metav1.UpdateOptions{})
	require.NoError(s.t, err)

	err = ensureAppCustomResourceSynced(context.TODO(), s.clusterClient, a)
	require.NoError(s.t, err)

	appCRD, err := tclient.TsuruV1().Apps(s.clusterClient.Namespace()).Get(ctx, a.Name, metav1.GetOptions{})
	require.NoError(s.t, err)

	require.EqualValues(s.t, metav1.ObjectMeta{
		Namespace: "tsuru",
		Name:      "myapp",
		Annotations: map[string]string{
			"external.io/teste": "true",
		},
	}, appCRD.ObjectMeta)

	require.EqualValues(s.t, tsuruv1.AppSpec{
		NamespaceName:      "default",
		ServiceAccountName: "app-myapp",
		Deployments: map[string][]string{
			"web": {"myapp-web"},
		},
		Services: map[string][]string{
			"web": {"myapp-web", "myapp-web-units"},
		},
		PodDisruptionBudgets: map[string][]string{
			"web": {"myapp-web"},
		},
	}, appCRD.Spec)
}

// This test is currently commented out because the "POST" verb of the mock client
// is always returning nil.
//
// func (s *S) TestUploadFile(c *check.C) {
// 	ctx := context.TODO()
// 	app, wait, rollback := s.mock.DefaultReactions(c)
// 	defer rollback()

// 	version := newSuccessfulVersion(c, app, map[string][]string{
// 		"web": {"python", "myapp.py"},
// 	})
// 	err := s.p.AddUnits(ctx, app, 1, "web", version, nil)
// 	require.NoError(s.t, err)
// 	wait()

// 	pods, err := s.client.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
// 	require.NoError(s.t, err)
// 	require.Len(s.t, pods.Items, 1)
// 	unit := pods.Items[0].Name

// 	filename := "file.txt"
// 	content := []byte("test file")
// 	path := fmt.Sprintf("/home/application/current/%s", filename)

// 	var file bytes.Buffer
// 	tarWriter := tar.NewWriter(&file)
// 	header := &tar.Header{
// 		Name: filename,
// 		Mode: 0644,
// 		Size: int64(len(content)),
// 	}
// 	tarWriter.WriteHeader(header)
// 	tarWriter.Write(content)
// 	err = tarWriter.Close()
// 	require.NoError(s.t, err)

// 	err = s.p.UploadFile(ctx, app, unit, file.Bytes(), path)
// 	require.NoError(s.t, err)

// 	actions := s.client.Actions()
// 	var execAction *ktesting.GetActionImpl
// 	for _, action := range actions {
// 		if action.GetSubresource() == "exec" {
// 			execAction = action.(*ktesting.GetActionImpl)
// 			break
// 		}
// 	}

// 	require.NotNil(s.t, execAction)
// 	require.Equal(s.t, unit, execAction.GetName())
// }
