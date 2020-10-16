// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"sync/atomic"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/kr/pretty"
	"github.com/tsuru/config"
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	kTesting "github.com/tsuru/tsuru/provision/kubernetes/testing"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/provision/servicecommon"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) prepareMultiCluster(c *check.C) (*kTesting.ClientWrapper, *kTesting.ClientWrapper) {
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
		ClusterInterface:       clusterClient2,
	}
	clusterClient2.Interface = client2

	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return []provTypes.Cluster{*cluster1, *cluster2}, nil
	}

	s.mockService.Cluster.OnFindByPool = func(provName, poolName string) (*provTypes.Cluster, error) {
		if poolName == "pool2" {
			return cluster2, nil
		}
		return cluster1, nil
	}

	ClientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
		if conf.Host == "https://clusteraddr1" {
			return client1, nil
		}
		return client2, nil
	}

	return client1, client2
}

func (s *S) TestManagerDeployNodeContainerMultiClusterNoApp(c *check.C) {
	client1, client2 := s.prepareMultiCluster(c)
	nc := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err := nodecontainer.AddNewContainer("", &nc)
	c.Assert(err, check.IsNil)

	m := nodeContainerManager{}
	err = m.DeployNodeContainer(&nc, "", servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)

	daemons, err := client1.AppsV1().DaemonSets("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Check(daemons.Items, check.HasLen, 1)

	daemons, err = client2.AppsV1().DaemonSets("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Check(daemons.Items, check.HasLen, 1)
}

func (s *S) TestManagerDeployNodeContainerMultiClusterWithApp(c *check.C) {
	client1, client2 := s.prepareMultiCluster(c)
	nc := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err := nodecontainer.AddNewContainer("", &nc)
	c.Assert(err, check.IsNil)

	app1 := provisiontest.NewFakeAppWithPool("myapp", "xxx", "pool2", 1)
	m := nodeContainerManager{
		app: app1,
	}
	err = m.DeployNodeContainer(&nc, "", servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)

	daemons, err := client1.AppsV1().DaemonSets("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Check(daemons.Items, check.HasLen, 0)

	daemons, err = client2.AppsV1().DaemonSets("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Check(daemons.Items, check.HasLen, 1)

	app2 := provisiontest.NewFakeAppWithPool("myapp", "xxx", "poolX", 1)
	m = nodeContainerManager{
		app: app2,
	}
	err = m.DeployNodeContainer(&nc, "", servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)

	daemons, err = client1.AppsV1().DaemonSets("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Check(daemons.Items, check.HasLen, 1)

	daemons, err = client2.AppsV1().DaemonSets("default").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Check(daemons.Items, check.HasLen, 1)
}

func (s *S) TestManagerDeployNodeContainer(c *check.C) {
	s.mock.MockfakeNodes(c)
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image:      "bsimg",
			Env:        []string{"a=b"},
			Entrypoint: []string{"cmd0"},
			Cmd:        []string{"cmd1"},
		},
		HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			Binds:         []string{"/xyz:/abc:ro"},
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	poolName := "mypool"
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: poolName, Provisioner: provisionerName})
	c.Assert(err, check.IsNil)
	m := nodeContainerManager{}
	err = m.DeployNodeContainer(&c1, poolName, servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)
	ns := s.client.PoolNamespace(poolName)
	daemons, err := s.client.AppsV1().DaemonSets(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 1)
	daemon, err := s.client.AppsV1().DaemonSets(ns).Get(context.TODO(), "node-container-bs-pool-mypool", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	trueVar := true
	maxUnavailable := intstr.FromString("20%")
	expectedAffinity := &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{{
					MatchExpressions: []apiv1.NodeSelectorRequirement{
						{
							Key:      "tsuru.io/pool",
							Operator: apiv1.NodeSelectorOpExists,
						},
					},
				}},
			},
		},
	}
	c.Assert(daemon, check.DeepEquals, &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-container-bs-pool-mypool",
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":            "true",
				"tsuru.io/is-node-container":   "true",
				"tsuru.io/provisioner":         "kubernetes",
				"tsuru.io/node-container-name": "bs",
				"tsuru.io/node-container-pool": poolName,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"tsuru.io/node-container-name": "bs",
					"tsuru.io/node-container-pool": poolName,
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"tsuru.io/is-tsuru":            "true",
						"tsuru.io/is-node-container":   "true",
						"tsuru.io/provisioner":         "kubernetes",
						"tsuru.io/node-container-name": "bs",
						"tsuru.io/node-container-pool": poolName,
					},
				},
				Spec: apiv1.PodSpec{
					EnableServiceLinks: func(b bool) *bool { return &b }(false),
					ServiceAccountName: "node-container-bs",
					Affinity:           expectedAffinity,
					Volumes: []apiv1.Volume{
						{
							Name: "volume-0",
							VolumeSource: apiv1.VolumeSource{
								HostPath: &apiv1.HostPathVolumeSource{
									Path: "/xyz",
								},
							},
						},
					},
					RestartPolicy: apiv1.RestartPolicyAlways,
					Containers: []apiv1.Container{
						{
							Name:    "bs",
							Image:   "bsimg",
							Command: []string{"cmd0"},
							Args:    []string{"cmd1"},
							Env: []apiv1.EnvVar{
								{Name: "a", Value: "b"},
							},
							VolumeMounts: []apiv1.VolumeMount{
								{Name: "volume-0", MountPath: "/abc", ReadOnly: true},
							},
							SecurityContext: &apiv1.SecurityContext{
								Privileged: &trueVar,
							},
						},
					},
					Tolerations: []apiv1.Toleration{
						{
							Key:      "tsuru.io/disabled",
							Operator: apiv1.TolerationOpExists,
						},
					},
				},
			},
		},
	})
	account, err := s.client.CoreV1().ServiceAccounts(ns).Get(context.TODO(), "node-container-bs", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(account, check.DeepEquals, &apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-container-bs",
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":            "true",
				"tsuru.io/node-container-name": "bs",
				"tsuru.io/provisioner":         "kubernetes",
			},
		},
	})
}

func (s *S) TestManagerDeployNodeContainerOnSinglePool(c *check.C) {
	s.mock.MockfakeNodes(c)
	s.clusterClient.CustomData[singlePoolKey] = "true"
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image:      "bsimg",
			Env:        []string{"a=b"},
			Entrypoint: []string{"cmd0"},
			Cmd:        []string{"cmd1"},
		},
		HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			Binds:         []string{"/xyz:/abc:ro"},
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	poolName := "mypool"
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: poolName, Provisioner: provisionerName})
	c.Assert(err, check.IsNil)
	m := nodeContainerManager{}
	err = m.DeployNodeContainer(&c1, poolName, servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)
	ns := s.client.PoolNamespace(poolName)
	daemons, err := s.client.AppsV1().DaemonSets(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 1)
	daemon, err := s.client.AppsV1().DaemonSets(ns).Get(context.TODO(), "node-container-bs-pool-mypool", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedAffinity := &apiv1.Affinity{}
	c.Assert(daemon.Spec.Template.Spec.Affinity, check.DeepEquals, expectedAffinity)
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{Exclude: []string{poolName}}, false)
	c.Assert(err, check.IsNil)
	daemon, err = s.client.AppsV1().DaemonSets(ns).Get(context.TODO(), "node-container-bs-all", metav1.GetOptions{})
	c.Assert(err, check.ErrorMatches, "daemonsets.apps \"node-container-bs-all\" not found")
	c.Assert(daemon, check.IsNil)
}

func (s *S) TestManagerDeployNodeContainerIgnoreInvalidPools(c *check.C) {
	s.mock.MockfakeNodes(c)
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "anotherpool", Provisioner: "docker"})
	c.Assert(err, check.IsNil)
	m := nodeContainerManager{}
	err = m.DeployNodeContainer(&c1, "anotherpool", servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)
	ns := "default"
	daemons, err := s.client.AppsV1().DaemonSets(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 0)
}

func (s *S) TestManagerDeployNodeContainerWithPoolNamespaces(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	s.mock.MockfakeNodes(c)
	poolName := "mypool"
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: poolName, Provisioner: provisionerName})
	c.Assert(err, check.IsNil)
	var counter int32
	s.client.PrependReactor("create", "daemonsets", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		atomic.AddInt32(&counter, 1)
		s.client.AppsV1()
		ns, ok := action.(ktesting.CreateAction).GetObject().(*appsv1.DaemonSet)
		c.Assert(ok, check.Equals, true)
		c.Assert(ns.ObjectMeta.Namespace, check.Equals, s.client.PoolNamespace(poolName))
		return false, nil, nil
	})
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image:      "bsimg",
			Env:        []string{"a=b"},
			Entrypoint: []string{"cmd0"},
			Cmd:        []string{"cmd1"},
		},
		HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			Binds:         []string{"/xyz:/abc:ro"},
		},
	}
	err = nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	m := nodeContainerManager{}
	err = m.DeployNodeContainer(&c1, poolName, servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&counter), check.Equals, int32(1))
}

func (s *S) TestManagerDeployNodeContainerWithFilter(c *check.C) {
	s.mock.MockfakeNodes(c)
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image:      "bsimg",
			Env:        []string{"a=b"},
			Entrypoint: []string{"cmd0"},
			Cmd:        []string{"cmd1"},
		},
		HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			Binds:         []string{"/xyz:/abc:ro"},
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	m := nodeContainerManager{}
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{Exclude: []string{"p1", "p2"}}, false)
	c.Assert(err, check.IsNil)
	ns := s.client.PoolNamespace("")
	daemon, err := s.client.AppsV1().DaemonSets(ns).Get(context.TODO(), "node-container-bs-all", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedAffinity := &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{{
					MatchExpressions: []apiv1.NodeSelectorRequirement{
						{
							Key:      "tsuru.io/pool",
							Operator: apiv1.NodeSelectorOpExists,
						},
						{
							Key:      "tsuru.io/pool",
							Operator: apiv1.NodeSelectorOpNotIn,
							Values:   []string{"p1", "p2"},
						},
					},
				}},
			},
		},
	}
	c.Assert(daemon.Spec.Template.Spec.Affinity, check.DeepEquals, expectedAffinity)
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{Include: []string{"p1"}}, false)
	c.Assert(err, check.IsNil)
	daemon, err = s.client.AppsV1().DaemonSets(ns).Get(context.TODO(), "node-container-bs-all", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedAffinity = &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{{
					MatchExpressions: []apiv1.NodeSelectorRequirement{
						{
							Key:      "tsuru.io/pool",
							Operator: apiv1.NodeSelectorOpExists,
						},
						{
							Key:      "tsuru.io/pool",
							Operator: apiv1.NodeSelectorOpIn,
							Values:   []string{"p1"},
						},
					},
				}},
			},
		},
	}
	c.Assert(daemon.Spec.Template.Spec.Affinity, check.DeepEquals, expectedAffinity)
}

func (s *S) TestManagerDeployNodeContainerBSSpecialMount(c *check.C) {
	s.mock.MockfakeNodes(c)
	c1 := nodecontainer.NodeContainerConfig{
		Name: nodecontainer.BsDefaultName,
		Config: docker.Config{
			Image: "img1",
		},
		HostConfig: docker.HostConfig{},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	m := nodeContainerManager{}
	poolName := "main"
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: poolName, Provisioner: provisionerName})
	c.Assert(err, check.IsNil)
	err = m.DeployNodeContainer(&c1, poolName, servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)
	ns := s.client.PoolNamespace(poolName)
	daemons, err := s.client.AppsV1().DaemonSets(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 1)
	daemon, err := s.client.AppsV1().DaemonSets(ns).Get(context.TODO(), "node-container-big-sibling-pool-main", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemon.Spec.Template.Spec.Volumes, check.DeepEquals, []apiv1.Volume{
		{
			Name: "volume-0",
			VolumeSource: apiv1.VolumeSource{
				HostPath: &apiv1.HostPathVolumeSource{
					Path: "/var/log",
				},
			},
		},
		{
			Name: "volume-1",
			VolumeSource: apiv1.VolumeSource{
				HostPath: &apiv1.HostPathVolumeSource{
					Path: "/var/lib/docker/containers",
				},
			},
		},
		{
			Name: "volume-2",
			VolumeSource: apiv1.VolumeSource{
				HostPath: &apiv1.HostPathVolumeSource{
					Path: "/mnt/sda1/var/lib/docker/containers",
				},
			},
		},
	})
	c.Assert(daemon.Spec.Template.Spec.Containers[0].VolumeMounts, check.DeepEquals, []apiv1.VolumeMount{
		{Name: "volume-0", MountPath: "/var/log", ReadOnly: false},
		{Name: "volume-1", MountPath: "/var/lib/docker/containers", ReadOnly: true},
		{Name: "volume-2", MountPath: "/mnt/sda1/var/lib/docker/containers", ReadOnly: true},
	})
}

func (s *S) TestManagerDeployNodeContainerBSMultiCluster(c *check.C) {
	s.mock.MockfakeNodes(c)
	clust := s.client.GetCluster()
	c.Assert(clust, check.NotNil)
	cluster2 := &provTypes.Cluster{
		Name:        "cluster2",
		Addresses:   []string{"https://clusteraddr"},
		Default:     true,
		Provisioner: provisionerName,
	}
	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return []provTypes.Cluster{*clust, *cluster2}, nil
	}
	c1 := nodecontainer.NodeContainerConfig{
		Name: nodecontainer.BsDefaultName,
		Config: docker.Config{
			Image: "img1",
		},
		HostConfig: docker.HostConfig{},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	m := nodeContainerManager{}
	poolName := "main"
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: poolName, Provisioner: provisionerName})
	c.Assert(err, check.IsNil)
	err = m.DeployNodeContainer(&c1, poolName, servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)
	ns := s.client.PoolNamespace(poolName)
	daemons, err := s.client.AppsV1().DaemonSets(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 1)
	daemon, err := s.client.AppsV1().DaemonSets(ns).Get(context.TODO(), "node-container-big-sibling-pool-main", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedVolumes := []apiv1.Volume{
		{
			Name: "volume-0",
			VolumeSource: apiv1.VolumeSource{
				HostPath: &apiv1.HostPathVolumeSource{
					Path: "/var/log",
				},
			},
		},
		{
			Name: "volume-1",
			VolumeSource: apiv1.VolumeSource{
				HostPath: &apiv1.HostPathVolumeSource{
					Path: "/var/lib/docker/containers",
				},
			},
		},
		{
			Name: "volume-2",
			VolumeSource: apiv1.VolumeSource{
				HostPath: &apiv1.HostPathVolumeSource{
					Path: "/mnt/sda1/var/lib/docker/containers",
				},
			},
		},
	}
	expectedMounts := []apiv1.VolumeMount{
		{Name: "volume-0", MountPath: "/var/log", ReadOnly: false},
		{Name: "volume-1", MountPath: "/var/lib/docker/containers", ReadOnly: true},
		{Name: "volume-2", MountPath: "/mnt/sda1/var/lib/docker/containers", ReadOnly: true},
	}
	c.Assert(daemon.Spec.Template.Spec.Volumes, check.DeepEquals, expectedVolumes,
		check.Commentf("Diff: %v", pretty.Diff(daemon.Spec.Template.Spec.Volumes, expectedVolumes)))
	c.Assert(daemon.Spec.Template.Spec.Containers[0].VolumeMounts, check.DeepEquals, expectedMounts,
		check.Commentf("Diff: %v", pretty.Diff(daemon.Spec.Template.Spec.Containers[0].VolumeMounts, expectedMounts)))
}

func (s *S) TestManagerDeployNodeContainerPlacementOnly(c *check.C) {
	reaction := func(action ktesting.Action) (bool, runtime.Object, error) {
		ds := action.(ktesting.CreateAction).GetObject().(*appsv1.DaemonSet)
		ds.ObjectMeta.CreationTimestamp = metav1.Time{Time: time.Now()}
		return false, nil, nil
	}
	s.client.PrependReactor("create", "daemonsets", reaction)
	s.client.PrependReactor("update", "daemonsets", reaction)
	s.mock.MockfakeNodes(c)
	c1 := nodecontainer.NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image:      "bsimg",
			Env:        []string{"a=b"},
			Entrypoint: []string{"cmd0"},
			Cmd:        []string{"cmd1"},
		},
		HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			Binds:         []string{"/xyz:/abc:ro"},
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	m := nodeContainerManager{}
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{}, true)
	c.Assert(err, check.IsNil)
	ns := s.client.PoolNamespace("")
	daemon, err := s.client.AppsV1().DaemonSets(ns).Get(context.TODO(), "node-container-bs-all", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedAffinity := &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{{
					MatchExpressions: []apiv1.NodeSelectorRequirement{
						{
							Key:      "tsuru.io/pool",
							Operator: apiv1.NodeSelectorOpExists,
						},
					},
				}},
			},
		},
	}
	c.Assert(daemon.Spec.Template.Spec.Affinity, check.DeepEquals, expectedAffinity)
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{Exclude: []string{"p1"}}, true)
	c.Assert(err, check.IsNil)
	daemon, err = s.client.AppsV1().DaemonSets(ns).Get(context.TODO(), "node-container-bs-all", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	expectedAffinity = &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{{
					MatchExpressions: []apiv1.NodeSelectorRequirement{
						{
							Key:      "tsuru.io/pool",
							Operator: apiv1.NodeSelectorOpExists,
						},
						{
							Key:      "tsuru.io/pool",
							Operator: apiv1.NodeSelectorOpNotIn,
							Values:   []string{"p1"},
						},
					},
				}},
			},
		},
	}
	c.Assert(daemon.Spec.Template.Spec.Affinity, check.DeepEquals, expectedAffinity)
	beforeCreation := daemon.CreationTimestamp
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{Exclude: []string{"p1"}}, true)
	c.Assert(err, check.IsNil)
	daemon, err = s.client.AppsV1().DaemonSets(ns).Get(context.TODO(), "node-container-bs-all", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemon.CreationTimestamp, check.DeepEquals, beforeCreation)
}
