// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"encoding/json"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"gopkg.in/check.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	ktesting "k8s.io/client-go/testing"
)

func (s *S) TestManagerDeployNodeContainer(c *check.C) {
	s.mockfakeNodes(c)
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
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)
	daemons, err := s.client.Extensions().DaemonSets(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 1)
	daemon, err := s.client.Extensions().DaemonSets(s.client.Namespace()).Get("node-container-bs-all", metav1.GetOptions{})
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
	affinityData, err := json.Marshal(expectedAffinity)
	c.Assert(err, check.IsNil)
	c.Assert(daemon, check.DeepEquals, &v1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-container-bs-all",
			Namespace: s.client.Namespace(),
		},
		Spec: v1beta1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"tsuru.io/node-container-name": "bs",
					"tsuru.io/node-container-pool": "",
				},
			},
			UpdateStrategy: v1beta1.DaemonSetUpdateStrategy{
				Type: v1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &v1beta1.RollingUpdateDaemonSet{
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
						"tsuru.io/node-container-pool": "",
					},
					Annotations: map[string]string{
						"scheduler.alpha.kubernetes.io/affinity": string(affinityData),
					},
				},
				Spec: apiv1.PodSpec{
					Affinity: expectedAffinity,
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
				},
			},
		},
	})
}

func (s *S) TestManagerDeployNodeContainerWithFilter(c *check.C) {
	s.mockfakeNodes(c)
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
	daemon, err := s.client.Extensions().DaemonSets(s.client.Namespace()).Get("node-container-bs-all", metav1.GetOptions{})
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
	affinityData, err := json.Marshal(expectedAffinity)
	c.Assert(err, check.IsNil)
	c.Assert(daemon.Spec.Template.ObjectMeta.Annotations, check.DeepEquals, map[string]string{
		"scheduler.alpha.kubernetes.io/affinity": string(affinityData),
	})
	c.Assert(daemon.Spec.Template.Spec.Affinity, check.DeepEquals, expectedAffinity)
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{Include: []string{"p1"}}, false)
	c.Assert(err, check.IsNil)
	daemon, err = s.client.Extensions().DaemonSets(s.client.Namespace()).Get("node-container-bs-all", metav1.GetOptions{})
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
	affinityData, err = json.Marshal(expectedAffinity)
	c.Assert(err, check.IsNil)
	c.Assert(daemon.Spec.Template.ObjectMeta.Annotations, check.DeepEquals, map[string]string{
		"scheduler.alpha.kubernetes.io/affinity": string(affinityData),
	})
	c.Assert(daemon.Spec.Template.Spec.Affinity, check.DeepEquals, expectedAffinity)
}

func (s *S) TestManagerDeployNodeContainerBSSpecialMount(c *check.C) {
	s.mockfakeNodes(c)
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
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{}, false)
	c.Assert(err, check.IsNil)
	daemons, err := s.client.Extensions().DaemonSets(s.client.Namespace()).List(metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemons.Items, check.HasLen, 1)
	daemon, err := s.client.Extensions().DaemonSets(s.client.Namespace()).Get("node-container-big-sibling-all", metav1.GetOptions{})
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

func (s *S) TestManagerDeployNodeContainerPlacementOnly(c *check.C) {
	reaction := func(action ktesting.Action) (bool, runtime.Object, error) {
		ds := action.(ktesting.CreateAction).GetObject().(*v1beta1.DaemonSet)
		ds.ObjectMeta.CreationTimestamp = metav1.Time{Time: time.Now()}
		return false, nil, nil
	}
	s.client.PrependReactor("create", "daemonsets", reaction)
	s.client.PrependReactor("update", "daemonsets", reaction)
	s.mockfakeNodes(c)
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
	daemon, err := s.client.Extensions().DaemonSets(s.client.Namespace()).Get("node-container-bs-all", metav1.GetOptions{})
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
	affinityData, err := json.Marshal(expectedAffinity)
	c.Assert(err, check.IsNil)
	c.Assert(daemon.Spec.Template.ObjectMeta.Annotations, check.DeepEquals, map[string]string{
		"scheduler.alpha.kubernetes.io/affinity": string(affinityData),
	})
	c.Assert(daemon.Spec.Template.Spec.Affinity, check.DeepEquals, expectedAffinity)
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{Exclude: []string{"p1"}}, true)
	c.Assert(err, check.IsNil)
	daemon, err = s.client.Extensions().DaemonSets(s.client.Namespace()).Get("node-container-bs-all", metav1.GetOptions{})
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
	affinityData, err = json.Marshal(expectedAffinity)
	c.Assert(err, check.IsNil)
	c.Assert(daemon.Spec.Template.ObjectMeta.Annotations, check.DeepEquals, map[string]string{
		"scheduler.alpha.kubernetes.io/affinity": string(affinityData),
	})
	c.Assert(daemon.Spec.Template.Spec.Affinity, check.DeepEquals, expectedAffinity)
	beforeCreation := daemon.CreationTimestamp
	err = m.DeployNodeContainer(&c1, "", servicecommon.PoolFilter{Exclude: []string{"p1"}}, true)
	c.Assert(err, check.IsNil)
	daemon, err = s.client.Extensions().DaemonSets(s.client.Namespace()).Get("node-container-bs-all", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(daemon.CreationTimestamp, check.DeepEquals, beforeCreation)
}
