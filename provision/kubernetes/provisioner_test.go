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

	"github.com/kr/pretty"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	"github.com/tsuru/tsuru/provision/kubernetes/testing"
	kTesting "github.com/tsuru/tsuru/provision/kubernetes/testing"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	bindTypes "github.com/tsuru/tsuru/types/bind"
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

func (s *S) prepareMultiCluster(c *check.C) (*kTesting.ClientWrapper, *kTesting.ClientWrapper, *kTesting.ClientWrapper) {
	cluster1 := &provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"https://clusteraddr1"},
		Default:     true,
		Provisioner: provisionerName,
		CustomData:  map[string]string{},
	}
	clusterClient1, err := NewClusterClient(cluster1)
	c.Assert(err, check.IsNil)
	client1 := &kTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
		MetricsClientset:       fakemetrics.NewSimpleClientset(),
		VPAClientset:           fakevpa.NewSimpleClientset(),
		BackendClientset:       fakeBackendConfig.NewSimpleClientset(),
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
	c.Assert(err, check.IsNil)
	client2 := &kTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
		MetricsClientset:       fakemetrics.NewSimpleClientset(),
		VPAClientset:           fakevpa.NewSimpleClientset(),
		BackendClientset:       fakeBackendConfig.NewSimpleClientset(),
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
	c.Assert(err, check.IsNil)
	client3 := &kTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
		MetricsClientset:       fakemetrics.NewSimpleClientset(),
		VPAClientset:           fakevpa.NewSimpleClientset(),
		BackendClientset:       fakeBackendConfig.NewSimpleClientset(),
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
	c.Assert(err, check.IsNil)
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	s.client.PrependReactor("create", "services", s.mock.ServiceWithPortReaction(c, []apiv1.ServicePort{
		{NodePort: int32(30001)},
		{NodePort: int32(30002)},
		{NodePort: int32(30003)},
	}))
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "myworker",
		},
	})
	c.Assert(err, check.IsNil)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 2)
	sort.Slice(units, func(i, j int) bool {
		return units[i].ProcessName < units[j].ProcessName
	})
	for i, u := range units {
		splittedName := strings.Split(u.ID, "-")
		c.Assert(splittedName, check.HasLen, 5)
		c.Assert(splittedName[0], check.Equals, "myapp")
		units[i].ID = ""
		units[i].Name = ""
		c.Assert(units[i].CreatedAt, check.Not(check.IsNil))
		units[i].CreatedAt = nil
	}
	restarts := int32(0)
	ready := false
	c.Assert(units, check.DeepEquals, []provision.Unit{
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
	})
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
	for _, a := range []provision.App{a1, a2} {
		for i := 1; i <= 2; i++ {
			s.waitPodUpdate(c, func() {
				_, err := s.client.CoreV1().Pods("default").Create(context.TODO(), &apiv1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%s-%d", a.GetName(), i),
						Labels: map[string]string{
							"tsuru.io/app-name":     a.GetName(),
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
				c.Assert(err, check.IsNil)
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
	c.Assert(err, check.IsNil)
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
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(context.TODO(), a1, a2)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 4)
	sort.Slice(units, func(i, j int) bool {
		return units[i].ID < units[j].ID
	})
	restarts := int32(0)
	ready := false
	c.Assert(units, check.DeepEquals, []provision.Unit{
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
	})
}

func (s *S) TestUnitsSkipTerminating(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "myworker",
		},
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	c.Assert(err, check.IsNil)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(podlist.Items), check.Equals, 2)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			if p.Labels["tsuru.io/app-process"] == "worker" {
				deadline := int64(10)
				p.Spec.ActiveDeadlineSeconds = &deadline
				_, err = s.client.CoreV1().Pods("default").Update(context.TODO(), &p, metav1.UpdateOptions{})
				c.Assert(err, check.IsNil)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 1)
	c.Assert(units[0].ProcessName, check.DeepEquals, "web")
}

func (s *S) TestUnitsSkipEvicted(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "myworker",
		},
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	c.Assert(err, check.IsNil)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(podlist.Items), check.Equals, 2)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			if p.Labels["tsuru.io/app-process"] == "worker" {
				p.Status.Phase = apiv1.PodFailed
				p.Status.Reason = "Evicted"
				_, err = s.client.CoreV1().Pods("default").Update(context.TODO(), &p, metav1.UpdateOptions{})
				c.Assert(err, check.IsNil)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 1)
	c.Assert(units[0].ProcessName, check.DeepEquals, "web")
}

func (s *S) TestUnitsStarting(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	c.Assert(err, check.IsNil)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(podlist.Items), check.Equals, 1)
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
				c.Assert(err, check.IsNil)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 1)
	c.Assert(units[0].Status, check.DeepEquals, provision.StatusStarting)
}

func (s *S) TestUnitsStartingError(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	c.Assert(err, check.IsNil)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(podlist.Items), check.Equals, 1)
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
				c.Assert(err, check.IsNil)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 1)
	c.Assert(units[0].Status, check.DeepEquals, provision.StatusError)
	c.Assert(units[0].StatusReason, check.DeepEquals, "OOMKilled")

}

func (s *S) TestUnitsCrashLoopBackOff(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	c.Assert(err, check.IsNil)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(podlist.Items), check.Equals, 1)
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
				c.Assert(err, check.IsNil)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 1)
	c.Assert(units[0].Status, check.DeepEquals, provision.StatusError)
	c.Assert(units[0].StatusReason, check.DeepEquals, "OOMKilled")

}

func (s *S) TestUnitsCrashLoopBackOffWithExitCode(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	c.Assert(err, check.IsNil)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(podlist.Items), check.Equals, 1)
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
				c.Assert(err, check.IsNil)
			}
		}
	})
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 1)
	c.Assert(units[0].Status, check.DeepEquals, provision.StatusError)
	c.Assert(units[0].StatusReason, check.DeepEquals, "exitCode: 1")

}

func (s *S) TestUnitsEmpty(c *check.C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.FormValue("labelSelector"), check.Equals, "tsuru.io/app-name in (myapp)")
		output := `{"items": []}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(output))
	}))
	defer srv.Close()
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestUnitsNoApps(c *check.C) {
	units, err := s.p.Units(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestRegisterUnit(c *check.C) {
	s.mock.DefaultHook = func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.FormValue("labelSelector"), check.Equals, "tsuru.io/app-name in (myapp)")
		output := `{"items": [
		{"metadata": {"name": "myapp-web-pod-1-1", "labels": {"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "web", "tsuru.io/app-platform": "python"}}, "status": {"phase": "Running"}}
	]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(output))
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	err = s.p.RegisterUnit(context.TODO(), a, units[0].ID, nil)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
}

func (s *S) TestAddUnits(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 3, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
}

func (s *S) TestAddUnitsNotProvisionedRecreateAppCRD(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Destroy(context.TODO(), a)
	c.Assert(err, check.IsNil)
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	a.Deploys = 1
	err = s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestRemoveUnits(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 3, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	err = s.p.RemoveUnits(context.TODO(), a, 2, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err = s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestRemoveUnits_SetUnitsToZero(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 5, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 5)
	var buffer bytes.Buffer
	err = s.p.RemoveUnits(context.TODO(), a, 5, "web", version, &buffer)
	c.Assert(err, check.IsNil)
	wait()
	units, err = s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
	c.Assert(buffer.String(), check.Matches, "(?s).*---- Calling app stop internally as the number of units is zero ----.*")
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	dep, err := s.client.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep, check.NotNil)
	c.Assert(dep.Labels["tsuru.io/is-stopped"], check.Equals, "true")
	svcs, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "tsuru.io/app-name=myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(len(svcs.Items), check.Equals, 2)
}

func (s *S) TestRestart(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	id := units[0].ID
	err = s.p.Restart(context.TODO(), a, "", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	units, err = s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units[0].ID, check.Not(check.Equals), id)
}

func (s *S) TestRestartNotProvisionedRecreateAppCRD(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Destroy(context.TODO(), a)
	c.Assert(err, check.IsNil)
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	a.Deploys = 1
	err = s.p.Restart(context.TODO(), a, "", version, nil)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRestart_ShouldNotRestartBaseVersionWhenStopped_StoppedDueToScaledToZero(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	v1 := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})

	evt1, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.GetName()},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)

	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{
		App:     a,
		Version: v1,
		Event:   evt1,
	})
	c.Assert(err, check.IsNil)
	err = evt1.Done(nil)
	c.Assert(err, check.IsNil)

	wait()

	v2 := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "./my/app.sh",
		},
	})

	evt2, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.GetName()},
		Kind:     permission.PermAppDeploy,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)

	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{
		App:              a,
		Version:          v2,
		Event:            evt2,
		PreserveVersions: true,
	})
	c.Assert(err, check.IsNil)

	err = evt2.Done(nil)
	c.Assert(err, check.IsNil)

	wait()

	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)

	err = s.p.RemoveUnits(context.TODO(), a, 1, "", v1, nil)
	c.Assert(err, check.IsNil)

	units, err = s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units[0].Version, check.Equals, 2)

	err = s.p.Restart(context.TODO(), a, "", nil, nil)
	c.Assert(err, check.IsNil)

	units, err = s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units[0].Version, check.Equals, 2)
}

func (s *S) TestStopStart(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 2, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	err = s.p.Stop(context.TODO(), a, "", version, &bytes.Buffer{})
	c.Assert(err, check.IsNil)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	_, err = s.client.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	svcs, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "tsuru.io/app-name=myapp",
	})
	c.Assert(err, check.IsNil)
	c.Assert(len(svcs.Items), check.Equals, 2)
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
	err = s.p.Start(context.TODO(), a, "", version, &bytes.Buffer{})
	c.Assert(err, check.IsNil)
	wait()
	svcs, err = s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "tsuru.io/app-name=myapp",
	})
	c.Assert(err, check.IsNil)
	c.Assert(len(svcs.Items), check.Equals, 2)
}

func (s *S) TestProvisionerDestroy(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	err = s.p.Destroy(context.TODO(), a)
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 0)
	services, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(services.Items, check.HasLen, 0)
	serviceAccounts, err := s.client.CoreV1().ServiceAccounts(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(serviceAccounts.Items, check.HasLen, 0)
	pdbList, err := s.client.PolicyV1().PodDisruptionBudgets(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pdbList.Items, check.HasLen, 0)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 0)
}

func (s *S) TestProvisionerDestroyVersion(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	deployEvent, err := event.New(&event.Opts{
		Target:      event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:        permission.PermAppDeploy,
		Owner:       s.token,
		Allowed:     event.Allowed(permission.PermAppDeploy),
		DisableLock: true,
	})
	c.Assert(err, check.IsNil)
	customData1 := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version1 := newCommittedVersion(c, a, customData1)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: deployEvent})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	wait()

	customData2 := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version2 := newCommittedVersion(c, a, customData2)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version2, Event: deployEvent, PreserveVersions: true})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	wait()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	services, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(services.Items, check.HasLen, 4)
	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-web-v2", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	err = s.p.DestroyVersion(context.TODO(), a, version2)
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	services, err = s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(services.Items, check.HasLen, 3)
	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-web-v2", metav1.GetOptions{})
	c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
	serviceAccounts, err := s.client.CoreV1().ServiceAccounts(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(serviceAccounts.Items, check.HasLen, 1)
	pdbList, err := s.client.PolicyV1().PodDisruptionBudgets(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pdbList.Items, check.HasLen, 1)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 1)
}

func (s *S) TestProvisionerRoutableAddressesMultipleProcs(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":   "run mycmd arg1",
			"other": "my other cmd",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()
	addrs, err := s.p.RoutableAddresses(context.TODO(), a)
	c.Assert(err, check.IsNil)
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
	c.Assert(addrs, check.DeepEquals, expected)
}

func (s *S) TestProvisionerRoutableAddresses(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()
	addrs, err := s.p.RoutableAddresses(context.TODO(), a)
	c.Assert(err, check.IsNil)
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
	c.Assert(addrs, check.DeepEquals, expected)
}

func (s *S) TestDeploy(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)

	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	c.Assert(deps.Items[0].Name, check.Equals, "myapp-web")
	containers := deps.Items[0].Spec.Template.Spec.Containers
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].Command[len(containers[0].Command)-3:], check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; exec run mycmd arg1",
	})
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(units, check.HasLen, 1)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(len(appList.Items), check.Equals, 1)
	c.Assert(appList.Items[0].Spec, check.DeepEquals, tsuruv1.AppSpec{
		NamespaceName:        "default",
		ServiceAccountName:   "app-myapp",
		Deployments:          map[string][]string{"web": {"myapp-web"}},
		Services:             map[string][]string{"web": {"myapp-web", "myapp-web-units"}},
		PodDisruptionBudgets: map[string][]string{"web": {"myapp-web"}},
	})
}

func (s *S) TestDeployCreatesAppCR(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err := s.p.Destroy(context.TODO(), a)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
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
		c.Assert(ok, check.Equals, true)
		if new == 2 {
			c.Assert(ns.ObjectMeta.Name, check.Equals, "tsuru-test-default")
		} else {
			c.Assert(ns.ObjectMeta.Name, check.Equals, s.client.Namespace())
		}
		return false, nil, nil
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	c.Assert(atomic.LoadInt32(&counter), check.Equals, int32(3))
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(len(appList.Items), check.Equals, 1)
	c.Assert(appList.Items[0].Spec, check.DeepEquals, tsuruv1.AppSpec{
		NamespaceName:        "tsuru-test-default",
		ServiceAccountName:   "app-myapp",
		Deployments:          map[string][]string{"web": {"myapp-web"}},
		Services:             map[string][]string{"web": {"myapp-web", "myapp-web-units"}},
		PodDisruptionBudgets: map[string][]string{"web": {"myapp-web"}},
	})
}

func (s *S) TestInternalAddresses(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	s.client.PrependReactor("create", "services", func(action ktesting.Action) (bool, runtime.Object, error) {
		srv := action.(ktesting.CreateAction).GetObject().(*apiv1.Service)
		if srv.Name == "myapp-web" {
			srv.Spec.Ports = []apiv1.ServicePort{
				{
					Port:     int32(80),
					NodePort: int32(30002),
					Protocol: "TCP",
				},
				{
					Port:     int32(443),
					NodePort: int32(30003),
					Protocol: "TCP",
				},
			}
		} else if srv.Name == "myapp-jobs" {
			srv.Spec.Ports = []apiv1.ServicePort{
				{
					Port:     int32(12201),
					NodePort: int32(30004),
					Protocol: "UDP",
				},
			}
		}

		return false, nil, nil
	})

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":  "run mycmd web",
			"jobs": "run mycmd jobs",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))

	addrs, err := s.p.InternalAddresses(context.Background(), a)
	c.Assert(err, check.IsNil)
	wait()

	c.Assert(addrs, check.DeepEquals, []provision.AppInternalAddress{
		{Domain: "myapp-web.default.svc.cluster.local", Protocol: "TCP", Port: 80, Process: "web"},
		{Domain: "myapp-web.default.svc.cluster.local", Protocol: "TCP", Port: 443, Process: "web"},
		{Domain: "myapp-jobs.default.svc.cluster.local", Protocol: "UDP", Port: 12201, Process: "jobs"},
	})
}

func (s *S) TestInternalAddressesNoService(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd web",
		},
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
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))

	addrs, err := s.p.InternalAddresses(context.Background(), a)
	c.Assert(err, check.IsNil)
	wait()

	c.Assert(addrs, check.HasLen, 0)
}

func (s *S) TestDeployWithCustomConfig(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
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
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	c.Assert(deps.Items[0].Name, check.Equals, "myapp-web")
	containers := deps.Items[0].Spec.Template.Spec.Containers
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].Command[len(containers[0].Command)-3:], check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; exec run mycmd arg1",
	})
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(units, check.HasLen, 1)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(len(appList.Items), check.Equals, 1)
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
	c.Assert(appList.Items[0].Spec, check.DeepEquals, expected, check.Commentf("diff:\n%s", strings.Join(pretty.Diff(appList.Items[0].Spec, expected), "\n")))
}

func (s *S) TestDeployRollback(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	deployEvt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version1 := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: deployEvt})
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	customData = map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg2",
		},
	}
	version2 := newCommittedVersion(c, a, customData)
	img, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version2, Event: deployEvt})
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, "tsuru/app-myapp:v2")
	deployEvt.Done(err)
	c.Assert(err, check.IsNil)
	rollbackEvt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeployRollback,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeployRollback),
	})
	c.Assert(err, check.IsNil)
	img, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: rollbackEvt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	testBaseImage, err := version1.BaseImageName()
	c.Assert(err, check.IsNil)
	c.Assert(img, check.Equals, testBaseImage)
	wait()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)
	c.Assert(deps.Items[0].Name, check.Equals, "myapp-web")
	containers := deps.Items[0].Spec.Template.Spec.Containers
	c.Assert(containers, check.HasLen, 1)
	c.Assert(containers[0].Command[len(containers[0].Command)-3:], check.DeepEquals, []string{
		"/bin/sh",
		"-lc",
		"[ -d /home/application/current ] && cd /home/application/current; exec run mycmd arg1",
	})
	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestDeployBuilderImageWithRegistryAuth(c *check.C) {
	config.Set("docker:registry", "registry.example.com")
	defer config.Unset("docker:registry")
	config.Set("docker:registry-auth:username", "user")
	defer config.Unset("docker:registry-auth:username")
	config.Set("docker:registry-auth:password", "pwd")
	defer config.Unset("docker:registry-auth:password")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		containers := pod.Spec.Containers
		if containers[0].Name == "myapp-v1-deploy" {
			c.Assert(containers, check.HasLen, 2)
			sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
			cmds := cleanCmds(containers[0].Command[2])
			c.Assert(cmds, check.Equals, `end() { touch /tmp/intercontainer/done; }
trap end EXIT
mkdir -p $(dirname /dev/null) && cat >/dev/null && tsuru_unit_agent   myapp deploy-only`)
			c.Assert(containers[0].Env, check.DeepEquals, []apiv1.EnvVar{
				{Name: "DEPLOYAGENT_RUN_AS_SIDECAR", Value: "true"},
				{Name: "DEPLOYAGENT_DESTINATION_IMAGES", Value: "registry.example.com/tsuru/app-myapp:v1,registry.example.com/tsuru/app-myapp:latest"},
				{Name: "DEPLOYAGENT_SOURCE_IMAGE", Value: ""},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_USER", Value: "user"},
				{Name: "DEPLOYAGENT_REGISTRY_AUTH_PASS", Value: "pwd"},
				{Name: "DEPLOYAGENT_REGISTRY_ADDRESS", Value: "registry.example.com"},
				{Name: "DEPLOYAGENT_INPUT_FILE", Value: "/dev/null"},
				{Name: "DEPLOYAGENT_RUN_AS_USER", Value: "1000"},
				{Name: "DEPLOYAGENT_DOCKERFILE_BUILD", Value: "false"},
				{Name: "DEPLOYAGENT_INSECURE_REGISTRY", Value: "false"},
				{Name: "BUILDKITD_FLAGS", Value: "--oci-worker-no-process-sandbox"},
				{Name: "BUILDCTL_CONNECT_RETRIES_MAX", Value: "50"},
			})
		}
		return false, nil, nil
	})
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	version := newVersion(c, a, nil)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	wait()
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "registry.example.com/tsuru/app-myapp:v1")
}

func (s *S) TestExecuteCommandWithStdin(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
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
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	rollback()
	c.Assert(s.mock.Stream["myapp-web"].Stdin, check.Equals, "echo test")
	var sz remotecommand.TerminalSize
	err = json.Unmarshal([]byte(s.mock.Stream["myapp-web"].Resize), &sz)
	c.Assert(err, check.IsNil)
	c.Assert(sz, check.DeepEquals, remotecommand.TerminalSize{Width: 99, Height: 42})
	c.Assert(s.mock.Stream["myapp-web"].Urls, check.HasLen, 1)
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec")
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Query()["command"], check.DeepEquals, []string{"/usr/bin/env", "TERM=xterm", "mycmd", "arg1"})
}

func (s *S) TestExecuteCommandWithStdinNoSize(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
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
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	rollback()
	c.Assert(s.mock.Stream["myapp-web"].Stdin, check.Equals, "echo test")
	c.Assert(s.mock.Stream["myapp-web"].Urls, check.HasLen, 1)
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec")
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Query()["command"], check.DeepEquals, []string{"/usr/bin/env", "TERM=xterm", "mycmd", "arg1"})
}

func (s *S) TestExecuteCommandUnitNotFound(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	buf := bytes.NewBuffer(nil)
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: buf,
		Width:  99,
		Height: 42,
		Units:  []string{"invalid-unit"},
	})
	c.Assert(err, check.DeepEquals, &provision.UnitNotFoundError{ID: "invalid-unit"})
}

func (s *S) TestExecuteCommand(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 2, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Units:  []string{"myapp-web-pod-1-1", "myapp-web-pod-2-2"},
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	rollback()
	c.Assert(stdout.String(), check.Equals, "stdout datastdout data")
	c.Assert(stderr.String(), check.Equals, "stderr datastderr data")
	c.Assert(s.mock.Stream["myapp-web"].Urls, check.HasLen, 2)
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec")
	c.Assert(s.mock.Stream["myapp-web"].Urls[1].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-web-pod-2-2/exec")
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Query()["command"], check.DeepEquals, []string{"mycmd", "arg1", "arg2"})
	c.Assert(s.mock.Stream["myapp-web"].Urls[1].Query()["command"], check.DeepEquals, []string{"mycmd", "arg1", "arg2"})
}

func (s *S) TestExecuteCommandSingleUnit(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 2, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Units:  []string{"myapp-web-pod-1-1"},
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	rollback()
	c.Assert(stdout.String(), check.Equals, "stdout data")
	c.Assert(stderr.String(), check.Equals, "stderr data")
	c.Assert(s.mock.Stream["myapp-web"].Urls, check.HasLen, 1)
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-web-pod-1-1/exec")
	c.Assert(s.mock.Stream["myapp-web"].Urls[0].Query()["command"], check.DeepEquals, []string{"mycmd", "arg1", "arg2"})
}

func (s *S) TestExecuteCommandNoUnits(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err := s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(stdout.String(), check.Equals, "stdout data")
	c.Assert(stderr.String(), check.Equals, "stderr data")
	c.Assert(s.mock.Stream["myapp-isolated-run"].Urls, check.HasLen, 1)
	c.Assert(s.mock.Stream["myapp-isolated-run"].Urls[0].Path, check.DeepEquals, "/api/v1/namespaces/default/pods/myapp-isolated-run/attach")
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	pods, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 0)
	account, err := s.client.CoreV1().ServiceAccounts(ns).Get(context.TODO(), "app-myapp", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(account, check.DeepEquals, &apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-myapp",
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/provisioner": "kubernetes",
			},
		},
	})
}

func (s *S) TestExecuteCommandNoUnitsCheckPodRequirements(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	a.MilliCPU = 250000
	a.Memory = 100000
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	shouldFail := true
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		shouldFail = false
		var ephemeral resource.Quantity
		ephemeral, err = s.clusterClient.ephemeralStorage(a.GetPool())
		c.Assert(err, check.IsNil)
		expectedLimits := &apiv1.ResourceList{
			apiv1.ResourceMemory:           *resource.NewQuantity(a.GetMemory(), resource.BinarySI),
			apiv1.ResourceCPU:              *resource.NewMilliQuantity(int64(a.GetMilliCPU()), resource.DecimalSI),
			apiv1.ResourceEphemeralStorage: ephemeral,
		}
		expectedRequests := &apiv1.ResourceList{
			apiv1.ResourceMemory:           *resource.NewQuantity(a.GetMemory(), resource.BinarySI),
			apiv1.ResourceCPU:              *resource.NewMilliQuantity(int64(a.GetMilliCPU()), resource.DecimalSI),
			apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
		}
		c.Assert(pod.Spec.Containers[0].Resources.Limits, check.DeepEquals, *expectedLimits)
		c.Assert(pod.Spec.Containers[0].Resources.Requests, check.DeepEquals, *expectedRequests)

		return false, nil, nil
	})
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err = s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	c.Assert(shouldFail, check.Equals, false)
	c.Assert(err, check.IsNil)
}

func (s *S) TestExecuteCommandNoUnitsPodFailed(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		pod, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(ok, check.Equals, true)
		pod.Status.Phase = apiv1.PodFailed
		return false, nil, nil
	})
	newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	stdout, stderr := safe.NewBuffer(nil), safe.NewBuffer(nil)
	err := s.p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    a,
		Stdout: stdout,
		Stderr: stderr,
		Cmds:   []string{"mycmd", "arg1", "arg2"},
	})
	c.Assert(err, check.ErrorMatches, `(?s)invalid pod phase "Failed".*`)
}

func (s *S) TestExecuteCommandIsolatedWithoutNodeSelector(c *check.C) {
	s.clusterClient.CustomData["disable-default-node-selector"] = "true"
	defer delete(s.clusterClient.CustomData, "disable-default-node-selector")
	s.mock.IgnorePool = true
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	var checked bool
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(pod.Labels["tsuru.io/app-name"], check.Equals, "myapp")
		c.Assert(pod.Labels["tsuru.io/is-isolated-run"], check.Equals, "true")
		c.Assert(pod.Spec.NodeSelector, check.IsNil)
		c.Assert(pod.Spec.Affinity, check.IsNil)
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
	c.Assert(err, check.IsNil)
	c.Assert(checked, check.Equals, true)
}

func (s *S) TestStartupMessage(c *check.C) {
	msg, err := s.p.StartupMessage()
	c.Assert(err, check.IsNil)
	c.Assert(msg, check.Equals, `Kubernetes provisioner on cluster "c1" - https://clusteraddr
`)
	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return nil, nil
	}
	msg, err = s.p.StartupMessage()
	c.Assert(err, check.IsNil)
	c.Assert(msg, check.Equals, "")
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
	c.Assert(kubeConf, check.DeepEquals, kubernetesConfig{
		APITimeout:                          10 * time.Second,
		PodReadyTimeout:                     6 * time.Second,
		PodRunningTimeout:                   2 * time.Minute,
		DeploymentProgressTimeout:           3 * time.Minute,
		AttachTimeoutAfterContainerFinished: 5 * time.Second,
		HeadlessServicePort:                 8889,
	})
}

func (s *S) TestGetKubeConfigDefaults(c *check.C) {
	config.Unset("kubernetes")
	kubeConf := getKubeConfig()
	c.Assert(kubeConf, check.DeepEquals, kubernetesConfig{
		APITimeout:                          60 * time.Second,
		PodReadyTimeout:                     time.Minute,
		PodRunningTimeout:                   10 * time.Minute,
		DeploymentProgressTimeout:           10 * time.Minute,
		AttachTimeoutAfterContainerFinished: time.Minute,
		HeadlessServicePort:                 8888,
	})
}

func (s *S) TestProvisionerProvision(c *check.C) {
	_, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err := s.p.Provision(context.TODO(), a)
	c.Assert(err, check.IsNil)
	crdList, err := s.client.ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(crdList.Items, check.HasLen, 1)
	c.Assert(crdList.Items[0].GetName(), check.DeepEquals, "apps.tsuru.io")
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 1)
	c.Assert(appList.Items[0].GetName(), check.DeepEquals, a.GetName())
	c.Assert(appList.Items[0].Spec.NamespaceName, check.DeepEquals, "default")
}

func (s *S) TestProvisionerUpdateApp(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err = rebuild.Initialize(func(appName string) (rebuild.RebuildApp, error) {
		return &app.App{
			Name:    appName,
			Pool:    "test-pool-2",
			Routers: a.GetRouters(),
		}, nil
	})
	c.Assert(err, check.IsNil)
	defer rebuild.Shutdown(context.Background())
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	sList, err := s.client.CoreV1().Services("tsuru-test-pool-2").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 2)
	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), "test-pool-2", 0)
	buf := new(bytes.Buffer)
	var recreatedPods bool
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(pod.Spec.NodeSelector, check.DeepEquals, map[string]string{
			"tsuru.io/pool": newApp.GetPool(),
		})
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-pool"], check.Equals, newApp.GetPool())
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
	c.Assert(err, check.IsNil)
	err = s.p.UpdateApp(context.TODO(), a, newApp, buf)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(buf.String(), "Done updating units"), check.Equals, true)
	c.Assert(recreatedPods, check.Equals, true)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 1)
	c.Assert(appList.Items[0].GetName(), check.DeepEquals, a.GetName())
	c.Assert(appList.Items[0].Spec.NamespaceName, check.DeepEquals, "tsuru-test-pool-2")
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-pool-2").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 2)
}

func (s *S) TestProvisionerUpdateAppCanaryDeploy(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err = rebuild.Initialize(func(appName string) (rebuild.RebuildApp, error) {
		return &app.App{
			Name:    appName,
			Pool:    "test-pool-2",
			Routers: a.GetRouters(),
		}, nil
	})
	c.Assert(err, check.IsNil)
	defer rebuild.Shutdown(context.Background())
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	{
		customData := map[string]interface{}{
			"processes": map[string]interface{}{
				"web": "run mycmd arg1",
			},
		}
		version1 := newCommittedVersion(c, a, customData)
		var img1 string
		img1, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: evt})
		c.Assert(err, check.IsNil, check.Commentf("%+v", err))
		c.Assert(img1, check.Equals, "tsuru/app-myapp:v1")
		wait()
		customData = map[string]interface{}{
			"processes": map[string]interface{}{
				"web": "run mycmd arg1",
			},
		}
		version2 := newCommittedVersion(c, a, customData)
		var img2 string
		img2, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version2, Event: evt, PreserveVersions: true})
		c.Assert(err, check.IsNil, check.Commentf("%+v", err))
		c.Assert(img2, check.Equals, "tsuru/app-myapp:v2")
		wait()
	}
	sList, err := s.client.CoreV1().Services("tsuru-test-pool-2").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 4)
	contains := func(name string, svcList []apiv1.Service) bool {
		for _, svc := range svcList {
			if svc.Name == name {
				return true
			}
		}
		return false
	}
	c.Assert(contains("myapp-web", sList.Items), check.Equals, true)
	c.Assert(contains("myapp-web-units", sList.Items), check.Equals, true)
	c.Assert(contains("myapp-web-v1", sList.Items), check.Equals, true)
	c.Assert(contains("myapp-web-v2", sList.Items), check.Equals, true)
	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), "test-pool-2", 0)
	buf := new(bytes.Buffer)
	err = s.p.UpdateApp(context.TODO(), a, newApp, buf)
	c.Assert(err, check.DeepEquals, &tsuruErrors.ValidationError{Message: "can't provision new app with multiple versions, please unify them and try again"})
}

func (s *S) TestProvisionerUpdateAppCanaryDeployWithStoppedBaseDep(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	err = rebuild.Initialize(func(appName string) (rebuild.RebuildApp, error) {
		return &app.App{
			Name:    appName,
			Pool:    "test-pool-2",
			Routers: a.GetRouters(),
		}, nil
	})
	c.Assert(err, check.IsNil)
	defer rebuild.Shutdown(context.Background())
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version1 := newCommittedVersion(c, a, customData)
	img1, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img1, check.Equals, "tsuru/app-myapp:v1")
	wait()
	customData = map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version2 := newCommittedVersion(c, a, customData)
	img2, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version2, Event: evt, PreserveVersions: true})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img2, check.Equals, "tsuru/app-myapp:v2")
	wait()

	sList, err := s.client.CoreV1().Services("tsuru-test-pool-2").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 0)
	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), "test-pool-2", 0)
	buf := new(bytes.Buffer)
	err = s.p.Stop(context.TODO(), a, "", version1, buf)
	c.Assert(err, check.IsNil)
	contains := func(name string, depList []appsv1.Deployment) bool {
		for _, dep := range depList {
			if dep.Name == name {
				return true
			}
		}
		return false
	}
	replicaCount := func(name string, depList []appsv1.Deployment, expectedReplicas int) bool {
		for _, dep := range depList {
			if dep.Name == name {
				return *dep.Spec.Replicas == int32(expectedReplicas)
			}
		}
		return false
	}
	depList, err := s.client.AppsV1().Deployments("tsuru-test-default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(depList.Items), check.Equals, 2)
	c.Assert(contains("myapp-web-v2", depList.Items), check.DeepEquals, true)
	c.Assert(contains("myapp-web", depList.Items), check.DeepEquals, true)
	c.Assert(replicaCount("myapp-web", depList.Items, 0), check.Equals, true)
	c.Assert(replicaCount("myapp-web-v2", depList.Items, 1), check.Equals, true)
	err = s.p.UpdateApp(context.TODO(), a, newApp, buf)
	c.Assert(err, check.DeepEquals, &tsuruErrors.ValidationError{Message: "can't provision new app with multiple versions, please unify them and try again"})
}

func (s *S) TestProvisionerUpdateAppWithCanaryOtherCluster(c *check.C) {
	client1, client2, _ := s.prepareMultiCluster(c)
	s.client = client1
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	s.factory = informers.NewSharedInformerFactory(s.client, s.defaultSharedInformerDuration)
	s.mock = testing.NewKubeMock(s.client, s.p, s.p, s.factory)
	a, wait, rollback := s.mock.NoNodeReactions(c)
	defer rollback()

	pool2 := client2.GetCluster().Pools[0]
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        pool2,
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	{
		customData := map[string]interface{}{
			"processes": map[string]interface{}{
				"web": "run mycmd arg1",
			},
		}
		version1 := newCommittedVersion(c, a, customData)
		var img1 string
		img1, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version1, Event: evt})
		c.Assert(err, check.IsNil, check.Commentf("%+v", err))
		c.Assert(img1, check.Equals, "tsuru/app-myapp:v1")
		wait()
		customData = map[string]interface{}{
			"processes": map[string]interface{}{
				"web": "run mycmd arg1",
			},
		}
		version2 := newCommittedVersion(c, a, customData)
		var img2 string
		img2, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version2, Event: evt, PreserveVersions: true})
		c.Assert(err, check.IsNil, check.Commentf("%+v", err))
		c.Assert(img2, check.Equals, "tsuru/app-myapp:v2")
		wait()
	}

	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), pool2, 0)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(pod.Spec.NodeSelector, check.DeepEquals, map[string]string{
			"tsuru.io/pool": newApp.GetPool(),
		})
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-pool"], check.Equals, newApp.GetPool())
		return true, nil, nil
	})
	err = s.p.UpdateApp(context.TODO(), a, newApp, new(bytes.Buffer))
	c.Assert(err, check.DeepEquals, &tsuruErrors.ValidationError{Message: "can't provision new app with multiple versions, please unify them and try again"})
}

func (s *S) TestProvisionerUpdateAppWithVolumeSameClusterAndNamespace(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	config.Set("kubernetes:use-pool-namespaces", false)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	sList, err := s.client.CoreV1().Services("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 2)
	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), "test-pool-2", 0)
	buf := new(bytes.Buffer)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(pod.Spec.NodeSelector, check.DeepEquals, map[string]string{
			"tsuru.io/pool": newApp.GetPool(),
		})
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-pool"], check.Equals, newApp.GetPool())
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
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.GetName(),
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = s.p.UpdateApp(context.TODO(), a, newApp, buf)
	c.Assert(err, check.IsNil)
}

func (s *S) TestProvisionerUpdateAppWithVolumeSameClusterOtherNamespace(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-pool-2",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	img, err := s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil, check.Commentf("%+v", err))
	c.Assert(img, check.Equals, "tsuru/app-myapp:v1")
	wait()
	sList, err := s.client.CoreV1().Services("tsuru-test-pool-2").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 0)
	sList, err = s.client.CoreV1().Services("tsuru-test-default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(sList.Items), check.Equals, 2)
	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), "test-pool-2", 0)
	buf := new(bytes.Buffer)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		c.Assert(pod.Spec.NodeSelector, check.DeepEquals, map[string]string{
			"tsuru.io/pool": newApp.GetPool(),
		})
		c.Assert(pod.ObjectMeta.Labels["tsuru.io/app-pool"], check.Equals, newApp.GetPool())
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
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.GetName(),
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = s.p.UpdateApp(context.TODO(), a, newApp, buf)
	c.Assert(err, check.ErrorMatches, "can't change the pool of an app with binded volumes")
}

func (s *S) TestProvisionerUpdateAppWithVolumeOtherCluster(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	client1, client2, _ := s.prepareMultiCluster(c)
	s.client = client1
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	s.factory = informers.NewSharedInformerFactory(s.client, s.defaultSharedInformerDuration)
	s.mock = testing.NewKubeMock(s.client, s.p, s.p, s.factory)
	_, _, rollback1 := s.mock.NoNodeReactions(c)
	defer rollback1()

	pool2 := client2.GetCluster().Pools[0]
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        pool2,
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	s.client = client2
	s.factory = informers.NewSharedInformerFactory(s.client, s.defaultSharedInformerDuration)
	s.mock = testing.NewKubeMock(s.client, s.p, s.p, s.factory)
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
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.GetName(),
		MountPoint: "/mnt1",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.GetName(),
		MountPoint: "/mnt2",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	_, _, err = createVolumesForApp(context.TODO(), client1.ClusterInterface.(*ClusterClient), a)
	c.Assert(err, check.IsNil)

	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newSuccessfulVersion(c, a, customData)
	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), pool2, 0)
	pvcs, err := client1.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 1)

	err = s.p.Restart(context.TODO(), a, "web", version, nil)
	c.Assert(err, check.IsNil)

	err = s.p.UpdateApp(context.TODO(), a, newApp, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	// Check if old volume was removed
	pvcs, err = client1.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 0)
	// Check if new volume was created
	pvcs, err = client2.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 1)
}

func (s *S) TestProvisionerUpdateAppWithVolumeWithTwoBindsOtherCluster(c *check.C) {
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	client1, client2, _ := s.prepareMultiCluster(c)
	s.client = client1
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	s.factory = informers.NewSharedInformerFactory(s.client, s.defaultSharedInformerDuration)
	s.mock = testing.NewKubeMock(s.client, s.p, s.p, s.factory)
	_, _, rollback1 := s.mock.NoNodeReactions(c)
	defer rollback1()

	pool2 := client2.GetCluster().Pools[0]
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        pool2,
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	s.client = client2
	s.factory = informers.NewSharedInformerFactory(s.client, s.defaultSharedInformerDuration)
	s.mock = testing.NewKubeMock(s.client, s.p, s.p, s.factory)
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
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a.GetName(),
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	_, _, err = createVolumesForApp(context.TODO(), client1.ClusterInterface.(*ClusterClient), a)
	c.Assert(err, check.IsNil)
	a2 := provisiontest.NewFakeApp("myapp2", "python", 0)
	err = s.p.Provision(context.TODO(), a2)
	c.Assert(err, check.IsNil)
	client1.TsuruClientset.PrependReactor("create", "apps", s.mock.AppReaction(a2, c))
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v,
		AppName:    a2.GetName(),
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	_, _, err = createVolumesForApp(context.TODO(), client1.ClusterInterface.(*ClusterClient), a2)
	c.Assert(err, check.IsNil)

	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newSuccessfulVersion(c, a, customData)
	pvcs, err := client1.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 1)

	err = s.p.Restart(context.TODO(), a, "web", version, nil)
	c.Assert(err, check.IsNil)

	newApp := provisiontest.NewFakeAppWithPool(a.GetName(), a.GetPlatform(), pool2, 0)
	err = s.p.UpdateApp(context.TODO(), a, newApp, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	// Check if old volume was not removed
	pvcs, err = client1.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 1)
	// Check if new volume was created
	pvcs, err = client2.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pvcs.Items, check.HasLen, 1)
}

func (s *S) TestProvisionerInitialize(c *check.C) {
	_, ok := s.p.clusterControllers[s.clusterClient.Name]
	c.Assert(ok, check.Equals, false)
	err := s.p.Initialize()
	c.Assert(err, check.IsNil)
	_, ok = s.p.clusterControllers[s.clusterClient.Name]
	c.Assert(ok, check.Equals, true)
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
	c.Assert(strings.Contains(err.Error(), "when kubeConfig is set the use of cacert is not used"), check.Equals, true)
	c.Assert(strings.Contains(err.Error(), "when kubeConfig is set the use of clientcert is not used"), check.Equals, true)
	c.Assert(strings.Contains(err.Error(), "when kubeConfig is set the use of clientkey is not used"), check.Equals, true)

	err = s.p.ValidateCluster(&provTypes.Cluster{
		KubeConfig: &provTypes.KubeConfig{
			Cluster: clientcmdapi.Cluster{},
		},
	})
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err.Error(), check.Equals, "kubeConfig.cluster.server field is required")
}

func (s *S) TestProvisionerInitializeNoClusters(c *check.C) {
	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return nil, provTypes.ErrNoCluster
	}
	err := s.p.Initialize()
	c.Assert(err, check.IsNil)
}

func (s *S) TestEnvsForAppDefaultPort(c *check.C) {
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	version := newCommittedVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python proc1.py",
		},
	})
	c.Assert(err, check.IsNil)
	fa := provisiontest.NewFakeApp("myapp", "java", 1)
	fa.SetEnv(bindTypes.EnvVar{Name: "e1", Value: "v1"})

	envs := EnvsForApp(fa, "web", version, false)
	c.Assert(envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "web"},
		{Name: "TSURU_APPVERSION", Value: "1"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "port", Value: "8888"},
		{Name: "PORT", Value: "8888"},
		{Name: "PORT_web", Value: "8888"},
	})
}

func (s *S) TestEnvsForAppCustomPorts(c *check.C) {
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	version := newCommittedVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"proc1": "python proc1.py",
			"proc2": "python proc2.py",
			"proc3": "python proc3.py",
			"proc4": "python proc4.py",
			"proc5": "python worker.py",
			"proc6": "python proc6.py",
		},
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
	c.Assert(err, check.IsNil)
	fa := provisiontest.NewFakeApp("myapp", "java", 1)
	fa.SetEnv(bindTypes.EnvVar{Name: "e1", Value: "v1"})

	envs := EnvsForApp(fa, "proc1", version, false)
	c.Assert(envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc1"},
		{Name: "TSURU_APPVERSION", Value: "1"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "PORT_proc1", Value: "8080,9000"},
	})

	envs = EnvsForApp(fa, "proc2", version, false)
	c.Assert(envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc2"},
		{Name: "TSURU_APPVERSION", Value: "1"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "PORT_proc2", Value: "8000"},
	})

	envs = EnvsForApp(fa, "proc3", version, false)
	c.Assert(envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc3"},
		{Name: "TSURU_APPVERSION", Value: "1"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "PORT_proc3", Value: "8080"},
	})

	envs = EnvsForApp(fa, "proc4", version, false)
	c.Assert(envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc4"},
		{Name: "TSURU_APPVERSION", Value: "1"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "port", Value: "8888"},
		{Name: "PORT", Value: "8888"},
		{Name: "PORT_proc4", Value: "8888"},
	})

	envs = EnvsForApp(fa, "proc5", version, false)
	c.Assert(envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc5"},
		{Name: "TSURU_APPVERSION", Value: "1"},
		{Name: "TSURU_HOST", Value: ""},
	})

	envs = EnvsForApp(fa, "proc6", version, false)
	c.Assert(envs, check.DeepEquals, []bindTypes.EnvVar{
		{Name: "e1", Value: "v1"},
		{Name: "TSURU_PROCESSNAME", Value: "proc6"},
		{Name: "TSURU_APPVERSION", Value: "1"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "port", Value: "8888"},
		{Name: "PORT", Value: "8888"},
	})
}

func (s *S) TestProvisionerToggleRoutable(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()

	err = s.p.ToggleRoutable(context.TODO(), a, version, false)
	c.Assert(err, check.IsNil)
	wait()

	dep, err := s.client.AppsV1().Deployments("default").Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep.Spec.Paused, check.Equals, false)
	c.Assert(dep.Labels["tsuru.io/is-routable"], check.Equals, "false")
	c.Assert(dep.Spec.Template.Labels["tsuru.io/is-routable"], check.Equals, "false")

	rsList, err := s.client.AppsV1().ReplicaSets("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	for _, rs := range rsList.Items {
		c.Assert(rs.Labels["tsuru.io/is-routable"], check.Equals, "false")
		c.Assert(rs.Spec.Template.Labels["tsuru.io/is-routable"], check.Equals, "false")
	}

	pods, err := s.client.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	c.Assert(pods.Items[0].Labels["tsuru.io/is-routable"], check.Equals, "false")

	err = s.p.ToggleRoutable(context.TODO(), a, version, true)
	c.Assert(err, check.IsNil)
	wait()

	dep, err = s.client.AppsV1().Deployments("default").Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(dep.Spec.Paused, check.Equals, false)
	c.Assert(dep.Labels["tsuru.io/is-routable"], check.Equals, "true")
	c.Assert(dep.Spec.Template.Labels["tsuru.io/is-routable"], check.Equals, "true")

	rsList, err = s.client.AppsV1().ReplicaSets("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	for _, rs := range rsList.Items {
		c.Assert(rs.Labels["tsuru.io/is-routable"], check.Equals, "true")
		c.Assert(rs.Spec.Template.Labels["tsuru.io/is-routable"], check.Equals, "true")
	}

	pods, err = s.client.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(pods.Items, check.HasLen, 1)
	c.Assert(pods.Items[0].Labels["tsuru.io/is-routable"], check.Equals, "true")
}
