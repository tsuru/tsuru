// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kr/pretty"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	eventTypes "github.com/tsuru/tsuru/types/event"
	provTypes "github.com/tsuru/tsuru/types/provision"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	check "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	ktesting "k8s.io/client-go/testing"
	backendconfigv1 "k8s.io/ingress-gce/pkg/apis/backendconfig/v1"
	"k8s.io/utils/ptr"
)

func (s *S) TestServiceManagerDeployService(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	one := int32(1)
	ten := int32(10)
	maxSurge := intstr.FromString("100%")
	maxUnavailable := intstr.FromInt(0)
	expectedUID := int64(1000)
	depLabels := map[string]string{
		"tsuru.io/is-tsuru":            "true",
		"tsuru.io/is-service":          "true",
		"tsuru.io/is-build":            "false",
		"tsuru.io/is-stopped":          "false",
		"tsuru.io/is-isolated-run":     "false",
		"tsuru.io/is-routable":         "true",
		"tsuru.io/app-name":            "myapp",
		"tsuru.io/app-process":         "p1",
		"tsuru.io/app-team":            "admin",
		"tsuru.io/app-platform":        "",
		"tsuru.io/app-pool":            "test-default",
		"app":                          "myapp-p1",
		"app.kubernetes.io/component":  "tsuru-app",
		"app.kubernetes.io/managed-by": "tsuru",
		"app.kubernetes.io/name":       "myapp",
		"app.kubernetes.io/instance":   "myapp-p1",
	}
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	testBaseImage, err := version.BaseImageName()
	require.NoError(s.t, err)
	expected := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: nsName,
			Labels:    depLabels,
			Annotations: map[string]string{
				secretHashAnnotationKey: "f3f68faf3959035e45fe666a94854e2a76ff017ec775568944352586ca3d3fc5",
			},
		},
		Status: appsv1.DeploymentStatus{
			UpdatedReplicas: 1,
			Replicas:        1,
		},
		Spec: appsv1.DeploymentSpec{
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       &maxSurge,
					MaxUnavailable: &maxUnavailable,
				},
			},
			Replicas:             &one,
			RevisionHistoryLimit: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"tsuru.io/app-name":        "myapp",
					"tsuru.io/app-process":     "p1",
					"tsuru.io/is-build":        "false",
					"tsuru.io/is-isolated-run": "false",
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"tsuru.io/is-tsuru":        "true",
						"tsuru.io/is-build":        "false",
						"tsuru.io/is-isolated-run": "false",
						"tsuru.io/is-routable":     "true",
						"tsuru.io/app-name":        "myapp",
						"tsuru.io/app-process":     "p1",
						"tsuru.io/app-platform":    "",
						"tsuru.io/app-team":        "admin",
						"tsuru.io/app-pool":        "test-default",
						"tsuru.io/app-version":     "1",
					},
					Annotations: map[string]string{
						secretHashAnnotationKey: "f3f68faf3959035e45fe666a94854e2a76ff017ec775568944352586ca3d3fc5",
					},
				},
				Spec: apiv1.PodSpec{
					EnableServiceLinks: func(b bool) *bool { return &b }(false),
					ServiceAccountName: "app-myapp",
					SecurityContext: &apiv1.PodSecurityContext{
						RunAsUser: &expectedUID,
					},
					NodeSelector: map[string]string{
						"tsuru.io/pool": "test-default",
					},
					RestartPolicy:                 "Always",
					Subdomain:                     "myapp-p1-units",
					TerminationGracePeriodSeconds: func(v int64) *int64 { return &v }(40),
					Containers: []apiv1.Container{
						{
							Name:  "myapp-p1",
							Image: testBaseImage,
							Command: []string{
								"/bin/sh",
								"-lc",
								"[ -d /home/application/current ] && cd /home/application/current; exec cm1",
							},
							Env: []apiv1.EnvVar{
								{Name: "TSURU_APPDIR", Value: "/home/application/current"},
								{Name: "TSURU_APPNAME", Value: a.Name},
								{Name: "TSURU_SERVICES", ValueFrom: &apiv1.EnvVarSource{
									SecretKeyRef: &apiv1.SecretKeySelector{
										LocalObjectReference: apiv1.LocalObjectReference{
											Name: appSecretPrefix + "myapp-p1",
										},
										Key: "TSURU_SERVICES",
									},
								}},
								{Name: "TSURU_PROCESSNAME", Value: "p1"},
								{Name: "TSURU_APPVERSION", Value: "1"},
								{Name: "TSURU_HOST", Value: ""},
								{Name: "port", Value: "8888"},
								{Name: "PORT", Value: "8888"},
								{Name: "PORT_p1", Value: "8888"},
							},
							Resources: apiv1.ResourceRequirements{
								Limits: apiv1.ResourceList{
									apiv1.ResourceEphemeralStorage: defaultEphemeralStorageLimit,
									apiv1.ResourceCPU:              resource.MustParse("1000m"),
									apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
								},
								Requests: apiv1.ResourceList{
									apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
									apiv1.ResourceCPU:              resource.MustParse("1000m"),
									apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
								},
							},
							Ports: []apiv1.ContainerPort{
								{ContainerPort: 8888},
							},
							Lifecycle: &apiv1.Lifecycle{
								PreStop: &apiv1.LifecycleHandler{
									Exec: &apiv1.ExecAction{
										Command: []string{"sh", "-c", "sleep 10 || true"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.EqualValues(s.t, expected, dep)
	srv, err := s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: nsName,
			Labels: map[string]string{
				"app":                          "myapp-p1",
				"app.kubernetes.io/component":  "tsuru-app",
				"app.kubernetes.io/managed-by": "tsuru",
				"app.kubernetes.io/name":       "myapp",
				"app.kubernetes.io/instance":   "myapp-p1",
				"tsuru.io/is-tsuru":            "true",
				"tsuru.io/is-service":          "true",
				"tsuru.io/is-build":            "false",
				"tsuru.io/is-stopped":          "false",
				"tsuru.io/is-routable":         "true",
				"tsuru.io/app-name":            "myapp",
				"tsuru.io/app-team":            "admin",
				"tsuru.io/app-process":         "p1",
				"tsuru.io/app-platform":        "",
				"tsuru.io/app-pool":            "test-default",
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/app-process": "p1",
				"tsuru.io/is-build":    "false",
				"tsuru.io/is-routable": "true",
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(8888),
					TargetPort: intstr.FromInt(8888),
					Name:       "http-default-1",
				},
			},
			Type: apiv1.ServiceTypeClusterIP,
		},
	}, srv)
	_, err = s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	srvHeadless, err := s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1-units", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-units",
			Namespace: nsName,
			Labels: map[string]string{
				"app":                          "myapp-p1",
				"app.kubernetes.io/component":  "tsuru-app",
				"app.kubernetes.io/managed-by": "tsuru",
				"app.kubernetes.io/name":       "myapp",
				"app.kubernetes.io/instance":   "myapp-p1",
				"tsuru.io/is-tsuru":            "true",
				"tsuru.io/is-service":          "true",
				"tsuru.io/is-build":            "false",
				"tsuru.io/is-stopped":          "false",
				"tsuru.io/is-routable":         "true",
				"tsuru.io/is-headless-service": "true",
				"tsuru.io/app-name":            "myapp",
				"tsuru.io/app-team":            "admin",
				"tsuru.io/app-process":         "p1",
				"tsuru.io/app-platform":        "",
				"tsuru.io/app-pool":            "test-default",
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/app-process": "p1",
				"tsuru.io/is-build":    "false",
				"tsuru.io/is-routable": "true",
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(8888),
					TargetPort: intstr.FromInt(8888),
					Name:       "http-headless-1",
				},
			},
			ClusterIP: "None",
			Type:      apiv1.ServiceTypeClusterIP,
		},
	}, srvHeadless)
	account, err := s.client.CoreV1().ServiceAccounts(nsName).Get(context.TODO(), "app-myapp", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-myapp",
			Namespace: nsName,
			Labels: map[string]string{
				"tsuru.io/is-tsuru": "true",
				"tsuru.io/app-name": "myapp",
			},
		},
	}, account)
}

func (s *S) TestServiceManagerDeployServiceWithCustomAnnotations(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	s.clusterClient.CustomData[baseServicesAnnotations] = `{"myannotation.io/name": "test"}`
	s.clusterClient.CustomData[allServicesAnnotations] = `{"myannotation.io/name2": "test"}`
	defer func() {
		delete(s.clusterClient.CustomData, baseServicesAnnotations)
		delete(s.clusterClient.CustomData, allServicesAnnotations)
	}()
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	srv, err := s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, map[string]string{
		"myannotation.io/name":  "test",
		"myannotation.io/name2": "test",
	}, srv.Annotations)
}

func (s *S) TestServiceManagerDeployServiceWithCustomServiceAccountAnnotations(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name, Metadata: appTypes.Metadata{
		Annotations: []appTypes.MetadataItem{
			{
				Name:  AnnotationServiceAccountAppAnnotations,
				Value: `{"a1": "v1", "a2": "v2"}`,
			},
		},
	}}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	account, err := s.client.CoreV1().ServiceAccounts(nsName).Get(context.TODO(), "app-myapp", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-myapp",
			Namespace: nsName,
			Labels: map[string]string{
				"tsuru.io/is-tsuru": "true",
				"tsuru.io/app-name": "myapp",
			},
			Annotations: map[string]string{
				"a1": "v1",
				"a2": "v2",
			},
		},
	}, account)
}

func (s *S) TestServiceManagerDeployServiceWithCustomServiceAccountAnnotationsWithMetadataPrefix(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name, Metadata: appTypes.Metadata{
		Annotations: []appTypes.MetadataItem{
			{
				Name:  ResourceMetadataPrefix + "service-account",
				Value: `{"a1": "v1", "a2": "v2"}`,
			},
		},
	}}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	account, err := s.client.CoreV1().ServiceAccounts(nsName).Get(context.TODO(), "app-myapp", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-myapp",
			Namespace: nsName,
			Labels: map[string]string{
				"tsuru.io/is-tsuru": "true",
				"tsuru.io/app-name": "myapp",
			},
			Annotations: map[string]string{
				"a1": "v1",
				"a2": "v2",
			},
		},
	}, account)
}

func (s *S) TestServiceManagerDeployServiceWithCustomAnnotationsFromDeployment(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name, Metadata: appTypes.Metadata{
		Annotations: []appTypes.MetadataItem{
			{
				Name:  ResourceMetadataPrefix + "service",
				Value: `{"a1": "v1", "a2": "v2"}`,
			},
		},
	}}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	svc1, err := s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, map[string]string{"a1": "v1", "a2": "v2"}, svc1.Annotations)

	svc2, err := s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, map[string]string{"a1": "v1", "a2": "v2"}, svc2.Annotations)
}

func (s *S) TestServiceManagerDeployServiceWithNodeAffinity(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	err := pool.PoolUpdate(context.TODO(), "test-default", pool.UpdatePoolOptions{Labels: map[string]string{"affinity": `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"kubernetes.io/hostname","operator":"In","values":["minikube"]}]}]}}}`}})
	require.NoError(s.t, err)
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	expectedAffinity := &apiv1.Affinity{
		NodeAffinity: &apiv1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
				NodeSelectorTerms: []apiv1.NodeSelectorTerm{
					{
						MatchExpressions: []apiv1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: "In",
								Values:   []string{"minikube"},
							},
						},
					},
				},
			},
		},
	}
	require.Nil(s.t, dep.Spec.Template.Spec.NodeSelector)
	require.EqualValues(s.t, expectedAffinity, dep.Spec.Template.Spec.Affinity)
	err = pool.PoolUpdate(context.TODO(), "test-default", pool.UpdatePoolOptions{Labels: map[string]string{}})
	require.NoError(s.t, err)
}

func (s *S) TestServiceManagerDeployServiceWithPodAffinity(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	err := pool.PoolUpdate(context.TODO(), "test-default", pool.UpdatePoolOptions{Labels: map[string]string{"affinity": `{"podAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[{"labelSelector":{"matchExpressions":[{"key":"security","operator":"In","values":["S1"]}]},"topologyKey":"topology.kubernetes.io/zone"}]}}`}})
	require.NoError(s.t, err)
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	expectedAffinity := &apiv1.Affinity{
		PodAffinity: &apiv1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []apiv1.PodAffinityTerm{
				{
					TopologyKey: "topology.kubernetes.io/zone",
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key:      "security",
							Operator: "In",
							Values:   []string{"S1"},
						}},
					},
				},
			},
		},
	}
	require.EqualValues(s.t, map[string]string{"tsuru.io/pool": "test-default"}, dep.Spec.Template.Spec.NodeSelector)
	require.EqualValues(s.t, expectedAffinity, dep.Spec.Template.Spec.Affinity)
	err = pool.PoolUpdate(context.TODO(), "test-default", pool.UpdatePoolOptions{Labels: map[string]string{}})
	require.NoError(s.t, err)
}

func (s *S) TestServiceManagerDeployServiceWithAffinityAndClusterNodeSelectorDisabled(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	m.client.CustomData[disableDefaultNodeSelectorKey] = "true"
	err := pool.PoolUpdate(context.TODO(), "test-default", pool.UpdatePoolOptions{Labels: map[string]string{"affinity": `{"podAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[{"labelSelector":{"matchExpressions":[{"key":"security","operator":"In","values":["S1"]}]},"topologyKey":"topology.kubernetes.io/zone"}]}}`}})
	require.NoError(s.t, err)
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)

	expectedAffinity := &apiv1.Affinity{
		PodAffinity: &apiv1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []apiv1.PodAffinityTerm{
				{
					TopologyKey: "topology.kubernetes.io/zone",
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key:      "security",
							Operator: "In",
							Values:   []string{"S1"},
						}},
					},
				},
			},
		},
	}
	require.Nil(s.t, dep.Spec.Template.Spec.NodeSelector)
	require.EqualValues(s.t, expectedAffinity, dep.Spec.Template.Spec.Affinity)
	err = pool.PoolUpdate(context.TODO(), "test-default", pool.UpdatePoolOptions{Labels: map[string]string{}})
	require.NoError(s.t, err)
}

func (s *S) TestServiceManagerDeployServiceRaceWithHPA(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"web": {"cm1"},
	})
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	reaction := func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		dep := obj.(*appsv1.Deployment)
		dep.Status.UnavailableReplicas = 1
		depCopy := *dep
		go func() {
			time.Sleep(time.Second)
			depCopy.Status.UnavailableReplicas = 0
			replicas := *depCopy.Spec.Replicas
			replicas++
			depCopy.Spec.Replicas = &replicas
			depCopy.Status.UpdatedReplicas = replicas
			depCopy.Status.Replicas = replicas
			s.client.AppsV1().Deployments(ns).Update(context.TODO(), &depCopy, metav1.UpdateOptions{})
		}()
		return false, nil, nil
	}
	s.client.PrependReactor("create", "deployments", reaction)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, int32(2), *dep.Spec.Replicas)
}

func (s *S) TestServiceManagerDeployServiceWithPoolNamespaces(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	var counter int32
	s.client.PrependReactor("create", "namespaces", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		new := atomic.AddInt32(&counter, 1)
		ns, ok := action.(ktesting.CreateAction).GetObject().(*apiv1.Namespace)
		require.True(s.t, ok)
		if new == 2 {
			require.Equal(s.t, "tsuru-test-default", ns.ObjectMeta.Name)
		} else if new < 2 {
			require.Equal(s.t, s.client.Namespace(), ns.ObjectMeta.Name)
		}
		return false, nil, nil
	})
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	processes := map[string][]string{
		"p1": {"cmd1"},
		"p2": {"cmd2"},
		"p3": {"cmd3"},
	}
	version := newCommittedVersion(c, a, processes)
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	require.Equal(s.t, int32(len(processes)+1), atomic.LoadInt32(&counter))
}

func (s *S) TestServiceManagerDeployServiceCustomPorts(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, nil)
	err = version.AddData(appTypes.AddVersionDataArgs{
		ExposedPorts: []string{"7777/tcp", "7778/udp"},
		Processes:    map[string][]string{"p1": {"cmd1"}},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	srv, err := s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: nsName,
			Labels: map[string]string{
				"app":                          "myapp-p1",
				"app.kubernetes.io/component":  "tsuru-app",
				"app.kubernetes.io/managed-by": "tsuru",
				"app.kubernetes.io/name":       "myapp",
				"app.kubernetes.io/instance":   "myapp-p1",
				"tsuru.io/is-tsuru":            "true",
				"tsuru.io/is-service":          "true",
				"tsuru.io/is-build":            "false",
				"tsuru.io/is-stopped":          "false",
				"tsuru.io/is-routable":         "true",
				"tsuru.io/app-name":            "myapp",
				"tsuru.io/app-team":            "admin",
				"tsuru.io/app-process":         "p1",
				"tsuru.io/app-platform":        "",
				"tsuru.io/app-pool":            "test-default",
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/app-process": "p1",
				"tsuru.io/is-build":    "false",
				"tsuru.io/is-routable": "true",
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(7777),
					TargetPort: intstr.FromInt(7777),
					Name:       "http-default-1",
				},
				{
					Protocol:   "UDP",
					Port:       int32(7778),
					TargetPort: intstr.FromInt(7778),
					Name:       "udp-default-2",
				},
			},
			Type: apiv1.ServiceTypeClusterIP,
		},
	}, srv)
	srv, err = s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1-units", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-units",
			Namespace: nsName,
			Labels: map[string]string{
				"app":                          "myapp-p1",
				"app.kubernetes.io/component":  "tsuru-app",
				"app.kubernetes.io/managed-by": "tsuru",
				"app.kubernetes.io/name":       "myapp",
				"app.kubernetes.io/instance":   "myapp-p1",
				"tsuru.io/is-tsuru":            "true",
				"tsuru.io/is-service":          "true",
				"tsuru.io/is-build":            "false",
				"tsuru.io/is-stopped":          "false",
				"tsuru.io/is-routable":         "true",
				"tsuru.io/is-headless-service": "true",
				"tsuru.io/app-name":            "myapp",
				"tsuru.io/app-team":            "admin",
				"tsuru.io/app-process":         "p1",
				"tsuru.io/app-platform":        "",
				"tsuru.io/app-pool":            "test-default",
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/app-process": "p1",
				"tsuru.io/is-build":    "false",
				"tsuru.io/is-routable": "true",
			},
			Ports: []apiv1.ServicePort{
				{
					Name:       "http-headless-1",
					Protocol:   "TCP",
					Port:       int32(7777),
					TargetPort: intstr.FromInt(7777),
				},
				{
					Name:       "http-headless-2",
					Protocol:   "UDP",
					Port:       int32(7778),
					TargetPort: intstr.FromInt(7778),
				},
			},
			ClusterIP: "None",
			Type:      apiv1.ServiceTypeClusterIP,
		},
	}, srv)
}

func (s *S) TestServiceManagerDeployServiceNoExposedPorts(c *check.C) {
	config.Set("kubernetes:headless-service-port", 8889)
	defer config.Unset("kubernetes:headless-service-port")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a,
		map[string][]string{
			"p1": {"cmd1"},
		},
		map[string]interface{}{
			"kubernetes": provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"pod1": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"p1": {
							Ports: nil,
						},
					},
				},
			},
		})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1-units", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
}

func (s *S) TestServiceManagerDeployServiceNoExposedPortsRemoveExistingService(c *check.C) {
	config.Set("kubernetes:headless-service-port", 8889)
	defer config.Unset("kubernetes:headless-service-port")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cmd1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)

	version = newCommittedVersion(c, a, map[string][]string{
		"p1": {"cmd1"},
	},
		map[string]interface{}{
			"kubernetes": provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"pod1": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"p1": {
							Ports: nil,
						},
					},
				},
			},
		})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	_, err = s.client.CoreV1().Services(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
}

func (s *S) TestServiceManagerDeployServiceUpdateStates(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	require.NoError(s.t, err)
	tests := []struct {
		states []servicecommon.ProcessState
		fn     func(dep *appsv1.Deployment)
	}{
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 1},
			},
			fn: func(dep *appsv1.Deployment) {
				require.Equal(s.t, int32(2), *dep.Spec.Replicas)
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true},
			},
			fn: func(dep *appsv1.Deployment) {
				require.NotNil(s.t, dep)
				require.Zero(s.t, *dep.Spec.Replicas)
				ls := labelSetFromMeta(&dep.ObjectMeta)
				require.True(s.t, ls.IsStopped())
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Start: true},
			},
			fn: func(dep *appsv1.Deployment) {
				ls := labelSetFromMeta(&dep.ObjectMeta)
				require.False(s.t, ls.IsStopped())
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Restart: true},
			},
			fn: func(dep *appsv1.Deployment) {
				ls := labelSetFromMeta(&dep.ObjectMeta)
				require.False(s.t, ls.IsStopped())
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {},
			},
			fn: func(dep *appsv1.Deployment) {
				require.NotNil(s.t, dep)
				require.Zero(s.t, *dep.Spec.Replicas)
				ls := labelSetFromMeta(&dep.ObjectMeta)
				require.True(s.t, ls.IsStopped())
			},
		},
		{
			states: []servicecommon.ProcessState{
				{Start: true}, {Restart: true}, {Restart: true},
			},
			fn: func(dep *appsv1.Deployment) {
				require.Equal(s.t, int32(1), *dep.Spec.Replicas)
				ls := labelSetFromMeta(&dep.ObjectMeta)
				require.Equal(s.t, 2, ls.Restarts())
			},
		},
	}
	for i, tt := range tests {
		c.Logf("test %d", i)
		for _, state := range tt.states {
			err = servicecommon.RunServicePipeline(context.TODO(), &m, version.Version(), provision.DeployArgs{
				App:     a,
				Version: version,
			}, servicecommon.ProcessSpec{
				"p1": state,
			})
			require.NoError(s.t, err)
			waitDep()
		}
		var dep *appsv1.Deployment
		nsName, err := s.client.AppNamespace(context.TODO(), a)
		require.NoError(s.t, err)
		dep, err = s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
		require.True(s.t, err == nil || k8sErrors.IsNotFound(err))
		_, err = s.client.Clientset.CoreV1().Secrets(nsName).Get(context.TODO(), appSecretPrefix+"myapp-p1", metav1.GetOptions{})
		require.True(s.t, err == nil || k8sErrors.IsNotFound(err))
		waitDep()
		tt.fn(dep)
		err = cleanupDeployment(context.TODO(), s.clusterClient, a, "p1", version.Version())
		require.NoError(s.t, err)
		err = cleanupDeployment(context.TODO(), s.clusterClient, a, "p2", version.Version())
		require.NoError(s.t, err)
	}
}

func (s *S) TestServiceManagerDeployServiceWithHC(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	tests := []struct {
		hc                provTypes.TsuruYamlHealthcheck
		expectedLiveness  *apiv1.Probe
		expectedReadiness *apiv1.Probe
	}{
		{},
		{
			hc: provTypes.TsuruYamlHealthcheck{
				Path:   "/hc",
				Scheme: "https",
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    10,
				FailureThreshold: 3,
				TimeoutSeconds:   60,
				ProbeHandler: apiv1.ProbeHandler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:        "/hc",
						Port:        intstr.FromInt(8888),
						Scheme:      apiv1.URISchemeHTTPS,
						HTTPHeaders: []apiv1.HTTPHeader{},
					},
				},
			},
		},
		{
			hc: provTypes.TsuruYamlHealthcheck{
				Path:   "/hc",
				Scheme: "https",
				Command: []string{
					"cat",
					"/tmp/healthy",
				},
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    10,
				FailureThreshold: 3,
				TimeoutSeconds:   60,
				ProbeHandler: apiv1.ProbeHandler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:        "/hc",
						Port:        intstr.FromInt(8888),
						Scheme:      apiv1.URISchemeHTTPS,
						HTTPHeaders: []apiv1.HTTPHeader{},
					},
				},
			},
		},
		{
			hc: provTypes.TsuruYamlHealthcheck{
				Command: []string{
					"cat",
					"/tmp/healthy",
				},
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    10,
				FailureThreshold: 3,
				TimeoutSeconds:   60,
				ProbeHandler: apiv1.ProbeHandler{
					Exec: &apiv1.ExecAction{
						Command: []string{
							"cat",
							"/tmp/healthy",
						},
					},
				},
			},
		},
		{
			hc: provTypes.TsuruYamlHealthcheck{
				Path:            "/hc",
				Scheme:          "https",
				IntervalSeconds: 9,
				TimeoutSeconds:  2,
				AllowedFailures: 4,
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    9,
				FailureThreshold: 4,
				TimeoutSeconds:   2,
				ProbeHandler: apiv1.ProbeHandler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:        "/hc",
						Port:        intstr.FromInt(8888),
						Scheme:      apiv1.URISchemeHTTPS,
						HTTPHeaders: []apiv1.HTTPHeader{},
					},
				},
			},
		},
		{
			hc: provTypes.TsuruYamlHealthcheck{
				Path:            "/hc",
				Scheme:          "https",
				IntervalSeconds: 9,
				TimeoutSeconds:  2,
				AllowedFailures: 4,
				ForceRestart:    true,
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    9,
				FailureThreshold: 4,
				TimeoutSeconds:   2,
				ProbeHandler: apiv1.ProbeHandler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:        "/hc",
						Port:        intstr.FromInt(8888),
						Scheme:      apiv1.URISchemeHTTPS,
						HTTPHeaders: []apiv1.HTTPHeader{},
					},
				},
			},
			expectedLiveness: &apiv1.Probe{
				PeriodSeconds:    9,
				FailureThreshold: 4,
				TimeoutSeconds:   2,
				ProbeHandler: apiv1.ProbeHandler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:        "/hc",
						Port:        intstr.FromInt(8888),
						Scheme:      apiv1.URISchemeHTTPS,
						HTTPHeaders: []apiv1.HTTPHeader{},
					},
				},
			},
		},
		{
			hc: provTypes.TsuruYamlHealthcheck{
				Path:   "/hc",
				Scheme: "https",
				Headers: map[string]string{
					"Host":            "test.com",
					"X-Custom-Header": "test",
				},
				IntervalSeconds: 9,
				TimeoutSeconds:  2,
				AllowedFailures: 4,
				ForceRestart:    true,
			},
			expectedReadiness: &apiv1.Probe{
				PeriodSeconds:    9,
				FailureThreshold: 4,
				TimeoutSeconds:   2,
				ProbeHandler: apiv1.ProbeHandler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:        "/hc",
						Port:        intstr.FromInt(8888),
						Scheme:      apiv1.URISchemeHTTPS,
						HTTPHeaders: []apiv1.HTTPHeader{{Name: "Host", Value: "test.com"}, {Name: "X-Custom-Header", Value: "test"}},
					},
				},
			},
			expectedLiveness: &apiv1.Probe{
				PeriodSeconds:    9,
				FailureThreshold: 4,
				TimeoutSeconds:   2,
				ProbeHandler: apiv1.ProbeHandler{
					HTTPGet: &apiv1.HTTPGetAction{
						Path:        "/hc",
						Port:        intstr.FromInt(8888),
						Scheme:      apiv1.URISchemeHTTPS,
						HTTPHeaders: []apiv1.HTTPHeader{{Name: "Host", Value: "test.com"}, {Name: "X-Custom-Header", Value: "test"}},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		version := newCommittedVersion(c, a,
			map[string][]string{
				"web": {"cm1"},
				"p2":  {"cmd2"},
			},
			map[string]interface{}{
				"healthcheck": tt.hc,
			},
		)
		require.NoError(s.t, err)
		err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
			App:     a,
			Version: version,
		}, servicecommon.ProcessSpec{
			"web": servicecommon.ProcessState{Start: true},
			"p2":  servicecommon.ProcessState{Start: true},
		})
		require.NoError(s.t, err)
		waitDep()
		nsName, err := s.client.AppNamespace(context.TODO(), a)
		require.NoError(s.t, err)
		dep, err := s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		require.NoError(s.t, err)
		require.EqualValues(s.t, tt.expectedReadiness, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)
		require.EqualValues(s.t, tt.expectedLiveness, dep.Spec.Template.Spec.Containers[0].LivenessProbe)
		dep, err = s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
		require.NoError(s.t, err)
		require.Nil(s.t, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)
		require.Nil(s.t, dep.Spec.Template.Spec.Containers[0].LivenessProbe)
	}
}

func (s *S) TestEnsureBackendConfigIfEnabled(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	backendConfigCRD := &extensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: backendConfigCRDName},
	}
	_, err := s.client.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), backendConfigCRD, metav1.CreateOptions{})
	require.NoError(s.t, err)
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	expectedReadiness := &apiv1.Probe{
		PeriodSeconds:    9,
		FailureThreshold: 4,
		TimeoutSeconds:   10,
		ProbeHandler: apiv1.ProbeHandler{
			HTTPGet: &apiv1.HTTPGetAction{
				Path:        "/hc",
				Port:        intstr.FromInt(8888),
				Scheme:      apiv1.URISchemeHTTPS,
				HTTPHeaders: []apiv1.HTTPHeader{},
			},
		},
	}
	hc := provTypes.TsuruYamlHealthcheck{
		Path:            "/hc",
		Scheme:          "https",
		IntervalSeconds: 9,
		TimeoutSeconds:  10,
		AllowedFailures: 4,
		ForceRestart:    true,
	}

	intervalSec := int64PointerFromInt(hc.TimeoutSeconds + 1)
	timeoutSec := int64PointerFromInt(hc.TimeoutSeconds)
	protocolType := strings.ToUpper(hc.Scheme)
	expectedBackendConfig := backendconfigv1.BackendConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provision.AppProcessName(a, "web", 0, ""),
			Namespace: "default",
		},
		Spec: backendconfigv1.BackendConfigSpec{
			HealthCheck: &backendconfigv1.HealthCheckConfig{
				CheckIntervalSec:   intervalSec,
				TimeoutSec:         timeoutSec,
				Type:               &protocolType,
				RequestPath:        &hc.Path,
				HealthyThreshold:   int64PointerFromInt(1),
				UnhealthyThreshold: int64PointerFromInt(4),
			},
		},
	}

	version := newCommittedVersion(c, a,
		map[string][]string{
			"web": {"cm1"},
			"p2":  {"cmd2"},
		},
		map[string]interface{}{
			"healthcheck": hc,
		},
	)
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
		"p2":  servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedReadiness, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)
	dep, err = s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Nil(s.t, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)

	backendConfig, err := s.client.BackendClientset.CloudV1().BackendConfigs(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedBackendConfig.Spec.HealthCheck, backendConfig.Spec.HealthCheck)
}

func (s *S) TestEnsureBackendConfigIfEnabledWithDefaults(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	backendConfigCRD := &extensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: backendConfigCRDName},
	}
	_, err := s.client.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), backendConfigCRD, metav1.CreateOptions{})
	require.NoError(s.t, err)
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	expectedReadiness := &apiv1.Probe{
		PeriodSeconds:    9,
		FailureThreshold: 4,
		TimeoutSeconds:   60,
		ProbeHandler: apiv1.ProbeHandler{
			HTTPGet: &apiv1.HTTPGetAction{
				Path:        "/hc",
				Port:        intstr.FromInt(8888),
				Scheme:      apiv1.URISchemeHTTP,
				HTTPHeaders: []apiv1.HTTPHeader{},
			},
		},
	}
	hc := provTypes.TsuruYamlHealthcheck{
		Path:            "/hc",
		IntervalSeconds: 9,
		AllowedFailures: 4,
		ForceRestart:    true,
	}

	intervalSec := int64PointerFromInt(61)
	timeoutSec := int64PointerFromInt(60)
	protocolType := "HTTP"
	expectedBackendConfig := backendconfigv1.BackendConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provision.AppProcessName(a, "web", 0, ""),
			Namespace: "default",
		},
		Spec: backendconfigv1.BackendConfigSpec{
			HealthCheck: &backendconfigv1.HealthCheckConfig{
				CheckIntervalSec:   intervalSec,
				TimeoutSec:         timeoutSec,
				Type:               &protocolType,
				RequestPath:        &hc.Path,
				HealthyThreshold:   int64PointerFromInt(1),
				UnhealthyThreshold: int64PointerFromInt(4),
			},
		},
	}

	version := newCommittedVersion(c, a,
		map[string][]string{
			"web": {"cm1"},
			"p2":  {"cmd2"},
		},
		map[string]interface{}{
			"healthcheck": hc,
		},
	)
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
		"p2":  servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedReadiness, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)
	dep, err = s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Nil(s.t, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)

	backendConfig, err := s.client.BackendClientset.CloudV1().BackendConfigs(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedBackendConfig.Spec, backendConfig.Spec)
}

func (s *S) TestEnsureBackendConfigWithMissingSlash(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	backendConfigCRD := &extensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: backendConfigCRDName},
	}
	_, err := s.client.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), backendConfigCRD, metav1.CreateOptions{})
	require.NoError(s.t, err)
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	expectedReadiness := &apiv1.Probe{
		PeriodSeconds:    9,
		FailureThreshold: 4,
		TimeoutSeconds:   60,
		ProbeHandler: apiv1.ProbeHandler{
			HTTPGet: &apiv1.HTTPGetAction{
				Path:        "/healthcheck",
				Port:        intstr.FromInt(8888),
				Scheme:      apiv1.URISchemeHTTP,
				HTTPHeaders: []apiv1.HTTPHeader{},
			},
		},
	}
	hc := provTypes.TsuruYamlHealthcheck{
		Path:            "healthcheck",
		IntervalSeconds: 9,
		AllowedFailures: 4,
		ForceRestart:    true,
	}

	intervalSec := int64PointerFromInt(61)
	timeoutSec := int64PointerFromInt(60)
	protocolType := "HTTP"
	expectedBackendConfig := backendconfigv1.BackendConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provision.AppProcessName(a, "web", 0, ""),
			Namespace: "default",
		},
		Spec: backendconfigv1.BackendConfigSpec{
			HealthCheck: &backendconfigv1.HealthCheckConfig{
				CheckIntervalSec:   intervalSec,
				TimeoutSec:         timeoutSec,
				Type:               &protocolType,
				RequestPath:        ptr.To("/healthcheck"),
				HealthyThreshold:   int64PointerFromInt(1),
				UnhealthyThreshold: int64PointerFromInt(4),
			},
		},
	}

	version := newCommittedVersion(c, a,
		map[string][]string{
			"web": {"cm1"},
			"p2":  {"cmd2"},
		},
		map[string]interface{}{
			"healthcheck": hc,
		},
	)
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
		"p2":  servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedReadiness, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)
	dep, err = s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Nil(s.t, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)

	backendConfig, err := s.client.BackendClientset.CloudV1().BackendConfigs(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedBackendConfig.Spec, backendConfig.Spec)
}

func (s *S) TestEnsureBackendConfigWithCommandHC(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	backendConfigCRD := &extensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: backendConfigCRDName},
	}
	_, err := s.client.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), backendConfigCRD, metav1.CreateOptions{})
	require.NoError(s.t, err)
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)

	httpProto := "HTTP"
	hcPath := "/"
	expectedBackendConfig := backendconfigv1.BackendConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provision.AppProcessName(a, "web", 0, ""),
			Namespace: "default",
		},
		Spec: backendconfigv1.BackendConfigSpec{
			HealthCheck: &backendconfigv1.HealthCheckConfig{
				CheckIntervalSec:   int64PointerFromInt(61),
				TimeoutSec:         int64PointerFromInt(60),
				Type:               &httpProto,
				RequestPath:        &hcPath,
				HealthyThreshold:   int64PointerFromInt(1),
				UnhealthyThreshold: int64PointerFromInt(3),
			},
		},
	}

	hc := provTypes.TsuruYamlHealthcheck{
		Command: []string{"curl", "x"},
	}
	version := newCommittedVersion(c, a,
		map[string][]string{
			"web": {"cm1"},
			"p2":  {"cmd2"},
		},
		map[string]interface{}{
			"healthcheck": hc,
		},
	)
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
		"p2":  servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.NotNil(s.t, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)
	dep, err = s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Nil(s.t, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)
	backendConfig, err := s.client.BackendClientset.CloudV1().BackendConfigs(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedBackendConfig.Spec.HealthCheck, backendConfig.Spec.HealthCheck)
}

func (s *S) TestEnsureBackendConfigWithNoHC(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	backendConfigCRD := &extensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: backendConfigCRDName},
	}
	_, err := s.client.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), backendConfigCRD, metav1.CreateOptions{})
	require.NoError(s.t, err)
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"web": {"cm1"},
		"p2":  {"cmd2"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
		"p2":  servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Nil(s.t, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)
	dep, err = s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Nil(s.t, dep.Spec.Template.Spec.Containers[0].ReadinessProbe)

	httpProto := "HTTP"
	hcPath := "/"
	expectedBackendConfig := backendconfigv1.BackendConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provision.AppProcessName(a, "web", 0, ""),
			Namespace: "default",
		},
		Spec: backendconfigv1.BackendConfigSpec{
			HealthCheck: &backendconfigv1.HealthCheckConfig{
				CheckIntervalSec:   int64PointerFromInt(61),
				TimeoutSec:         int64PointerFromInt(60),
				Type:               &httpProto,
				RequestPath:        &hcPath,
				HealthyThreshold:   int64PointerFromInt(1),
				UnhealthyThreshold: int64PointerFromInt(3),
			},
		},
	}
	backendConfig, err := s.client.BackendClientset.CloudV1().BackendConfigs(nsName).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedBackendConfig.Spec.HealthCheck, backendConfig.Spec.HealthCheck)
}

func (s *S) TestServiceManagerDeployServiceWithRestartHooks(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a,
		map[string][]string{
			"web": {"proc1"},
			"p2":  {"proc2"},
		},
		map[string]interface{}{
			"hooks": provTypes.TsuruYamlHooks{
				Restart: provTypes.TsuruYamlRestartHooks{
					Before: []string{"before cmd1", "before cmd2"},
					After:  []string{"after cmd1", "after cmd2"},
				},
			},
		},
	)
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
		"p2":  servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	expectedLifecycle := &apiv1.Lifecycle{
		PostStart: &apiv1.LifecycleHandler{
			Exec: &apiv1.ExecAction{
				Command: []string{"sh", "-c", "after cmd1 && after cmd2"},
			},
		},
		PreStop: &apiv1.LifecycleHandler{
			Exec: &apiv1.ExecAction{
				Command: []string{"sh", "-c", "sleep 10 || true"},
			},
		},
	}
	require.EqualValues(s.t, expectedLifecycle, dep.Spec.Template.Spec.Containers[0].Lifecycle)
	cmd := dep.Spec.Template.Spec.Containers[0].Command
	require.Len(s.t, cmd, 3)
	require.Contains(s.t, cmd[2], "before cmd1 && before cmd2 && exec proc1")
	dep, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedLifecycle, dep.Spec.Template.Spec.Containers[0].Lifecycle)
	cmd = dep.Spec.Template.Spec.Containers[0].Command
	require.Len(s.t, cmd, 3)
	require.Contains(s.t, cmd[2], "before cmd1 && before cmd2 && exec proc2")
}

func (s *S) TestServiceManagerDeployServiceWithCustomSleep(c *check.C) {
	tests := []struct {
		value         string
		expectedLife  *apiv1.Lifecycle
		expectedGrace int64
	}{
		{
			value:         "",
			expectedGrace: 40,
			expectedLife: &apiv1.Lifecycle{
				PreStop: &apiv1.LifecycleHandler{
					Exec: &apiv1.ExecAction{
						Command: []string{"sh", "-c", "sleep 10 || true"},
					},
				},
			},
		},
		{
			value:         "invalid",
			expectedGrace: 40,
			expectedLife: &apiv1.Lifecycle{
				PreStop: &apiv1.LifecycleHandler{
					Exec: &apiv1.ExecAction{
						Command: []string{"sh", "-c", "sleep 10 || true"},
					},
				},
			},
		},
		{
			value:         "7",
			expectedGrace: 37,
			expectedLife: &apiv1.Lifecycle{
				PreStop: &apiv1.LifecycleHandler{
					Exec: &apiv1.ExecAction{
						Command: []string{"sh", "-c", "sleep 7 || true"},
					},
				},
			},
		},
		{
			value:         "0",
			expectedGrace: 30,
			expectedLife:  &apiv1.Lifecycle{},
		},
	}

	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"web": {"proc1"},
	})
	require.NoError(s.t, err)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	for _, tt := range tests {
		s.clusterClient.CustomData[preStopSleepKey] = tt.value
		err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
			App:     a,
			Version: version,
		}, servicecommon.ProcessSpec{
			"web": servicecommon.ProcessState{Start: true},
		})
		require.NoError(s.t, err)
		waitDep()
		dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		require.NoError(s.t, err)
		require.EqualValues(s.t, tt.expectedLife, dep.Spec.Template.Spec.Containers[0].Lifecycle)
		require.EqualValues(s.t, &tt.expectedGrace, dep.Spec.Template.Spec.TerminationGracePeriodSeconds)
	}
}

func (s *S) TestServiceManagerDeployServiceWithKubernetesPorts(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a,
		map[string][]string{
			"web": {"proc1"},
			"p2":  {"proc2"},
		},
		map[string]interface{}{
			"kubernetes": provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"mypod1": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"web": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{
									Name:       "port1",
									Protocol:   "UDP",
									TargetPort: 8080,
								},
								{
									Protocol: "TCP",
									Port:     9000,
								},
								{
									Port:       8000,
									TargetPort: 8001,
								},
							},
						},
					},
					"mypod2": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"p2": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{Name: "myport"},
							},
						},
					},
				},
			},
		},
	)
	require.NoError(s.t, err)

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
		"p2":  servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, []apiv1.ContainerPort{
		{ContainerPort: 8080},
		{ContainerPort: 9000},
		{ContainerPort: 8001},
	}, dep.Spec.Template.Spec.Containers[0].Ports)
	dep, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, []apiv1.ContainerPort{{ContainerPort: 8888}}, dep.Spec.Template.Spec.Containers[0].Ports)

	srv, err := s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, []apiv1.ServicePort{
		{
			Name:       "port1",
			Protocol:   "UDP",
			Port:       int32(8080),
			TargetPort: intstr.FromInt(8080),
		},
		{
			Name:       "http-default-2",
			Protocol:   "TCP",
			Port:       int32(9000),
			TargetPort: intstr.FromInt(9000),
		},
		{
			Name:       "http-default-3",
			Protocol:   "TCP",
			Port:       int32(8000),
			TargetPort: intstr.FromInt(8001),
		},
	}, srv.Spec.Ports)
	srv, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, []apiv1.ServicePort{
		{
			Name:       "myport",
			Protocol:   "TCP",
			Port:       int32(8888),
			TargetPort: intstr.FromInt(8888),
		},
	}, srv.Spec.Ports)

	srv, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-web-units", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, []apiv1.ServicePort{
		{
			Name:       "http-headless-1",
			Protocol:   "UDP",
			Port:       int32(8080),
			TargetPort: intstr.FromInt(8080),
		},
		{
			Name:       "http-headless-2",
			Protocol:   "TCP",
			Port:       int32(9000),
			TargetPort: intstr.FromInt(9000),
		},
		{
			Name:       "http-headless-3",
			Protocol:   "TCP",
			Port:       int32(8000),
			TargetPort: intstr.FromInt(8001),
		},
	}, srv.Spec.Ports)
}

func (s *S) TestServiceManagerDeployServiceWithKubernetesPortsDuplicatedProcess(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a,
		map[string][]string{
			"web": {"proc1"},
		},
		map[string]interface{}{
			"kubernetes": provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"mypod1": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"web": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{TargetPort: 8080},
							},
						},
					},
					"mypod2": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"web": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{Name: "myport"},
							},
						},
					},
				},
			},
		},
	)
	require.NoError(s.t, err)

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	})
	require.ErrorContains(s.t, err, "duplicated process name: web")
}

func (s *S) TestServiceManagerDeployServiceWithZeroKubernetesPorts(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a,
		map[string][]string{
			"web": {"proc1"},
		},
		map[string]interface{}{
			"kubernetes": provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"mypod1": map[string]provTypes.TsuruYamlKubernetesProcessConfig{
						"web": {
							Ports: nil,
						},
					},
				},
			},
		},
	)
	require.NoError(s.t, err)

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, []apiv1.ContainerPort{}, dep.Spec.Template.Spec.Containers[0].Ports)

	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-web-units", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
}

func (s *S) TestServiceManagerDeployServiceWithRegistryAuth(c *check.C) {
	config.Set("docker:registry", "myreg.com")
	config.Set("docker:registry-auth:username", "user")
	config.Set("docker:registry-auth:password", "pass")
	defer config.Unset("docker:registry")
	defer config.Unset("docker:registry-auth")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"web": {"cmd1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, []apiv1.LocalObjectReference{{Name: "docker-config-tsuru"}}, dep.Spec.Template.Spec.ImagePullSecrets)
	secret, err := s.client.CoreV1().Secrets(ns).Get(context.TODO(), "docker-config-tsuru", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "docker-config-tsuru",
			Namespace: "default",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"myreg.com":{"username":"user","password":"pass","auth":"dXNlcjpwYXNz"}}}`),
		},
		Type: "kubernetes.io/dockerconfigjson",
	}, secret)
}

func (s *S) TestServiceManagerDeployServiceProgressMessages(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	fakePodWatcher := watch.NewFakeWithChanSize(2, false)
	fakePodWatcher.Add(&apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: "myapp-web-base.1",
		},
		InvolvedObject: apiv1.ObjectReference{
			Name: "pod-name-1",
		},
		Source: apiv1.EventSource{
			Component: "c1",
		},
		Message: "msg1",
	})
	fakePodWatcher.Add(&apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: "myapp-web-base.2",
		},
		InvolvedObject: apiv1.ObjectReference{
			Name: "pod-name-1",
		},
		Source: apiv1.EventSource{
			Component: "c1",
			Host:      "n1",
		},
		Message: "msg2",
	})
	watchPodCalled := make(chan struct{})
	watchRepCalled := make(chan struct{})
	watchDepCalled := make(chan struct{})
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		dep := obj.(*appsv1.Deployment)
		dep.Status.UnavailableReplicas = 1
		depCopy := *dep
		go func() {
			<-watchPodCalled
			<-watchRepCalled
			<-watchDepCalled
			time.Sleep(time.Second)
			depCopy.Status.UnavailableReplicas = 0
			s.client.AppsV1().Deployments(ns).Update(context.TODO(), &depCopy, metav1.UpdateOptions{})
		}()
		return false, nil, nil
	})
	s.client.PrependWatchReactor("events", func(action ktesting.Action) (handled bool, ret watch.Interface, err error) {
		requirements := action.(ktesting.WatchActionImpl).GetWatchRestrictions().Fields.Requirements()
		for _, req := range requirements {
			if req.Value == "Pod" {
				close(watchPodCalled)
			}
			if req.Value == "ReplicaSet" {
				close(watchRepCalled)
			}
			if req.Value == "Deployment" {
				close(watchDepCalled)
			}
		}
		return true, fakePodWatcher, nil
	})
	buf := bytes.NewBuffer(nil)
	m := serviceManager{client: s.clusterClient, writer: buf}
	version := newCommittedVersion(c, a, map[string][]string{
		"web": {"cmd1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Contains(s.t, buf.String(), "---> 1 of 1 new units created")
	require.Contains(s.t, buf.String(), "---> 0 of 1 new units ready")
	require.Contains(s.t, buf.String(), "---> 1 of 1 new units ready")
	require.Contains(s.t, buf.String(), "---> All units ready")
	require.Contains(s.t, buf.String(), "---> pod-name-1 - msg1 [c1]")
	require.Contains(s.t, buf.String(), "---> pod-name-1 - msg2 [c1, n1]")
}

func (s *S) TestServiceManagerDeployServiceFirstDeployDeleteDeploymentOnRollback(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	buf := bytes.NewBuffer(nil)
	m := serviceManager{client: s.clusterClient, writer: buf}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:        eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:          permission.PermAppDeploy,
		Owner:         s.token,
		Allowed:       event.Allowed(permission.PermAppDeploy),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents),
		Cancelable:    true,
	})
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"web": {"cmd1"},
	})
	require.NoError(s.t, err)
	var deleteCalled bool
	s.client.PrependReactor("delete", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		name := action.(ktesting.DeleteAction).GetName()
		require.Equal(s.t, "myapp-web", name)
		deleteCalled = true
		return false, nil, nil
	})
	deployCreated := make(chan struct{})
	s.client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if dep, ok := obj.(*appsv1.Deployment); ok {
			rev, _ := strconv.Atoi(dep.Annotations[replicaDepRevision])
			rev++
			dep.Annotations = map[string]string{
				replicaDepRevision: strconv.Itoa(rev),
			}
			dep.Status.UnavailableReplicas = 1
			deployCreated <- struct{}{}
		}
		return false, nil, nil
	})
	ctx, cancel := evt.CancelableContext(context.TODO())
	defer cancel()
	go func(id string) {
		<-deployCreated
		evtDB, errCancel := event.GetByHexID(context.TODO(), id)
		require.NoError(s.t, errCancel)
		errCancel = evtDB.TryCancel(context.TODO(), "Because i want.", "admin@admin.com")
		require.NoError(s.t, errCancel)
	}(evt.UniqueID.Hex())
	err = servicecommon.RunServicePipeline(ctx, &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
		Event:   evt,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	})
	require.NotNil(s.t, err)
	require.IsType(s.t, provision.ErrUnitStartup{}, err)
	require.ErrorContains(s.t, err, "canceled by user action")
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), appSecretPrefix+"myapp-web", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	require.Contains(s.t, buf.String(), "---> 1 of 1 new units created")
	require.Contains(s.t, buf.String(), "---> 0 of 1 new units ready")
	require.Contains(s.t, buf.String(), "DELETING CREATED DEPLOYMENT AFTER FAILURE")
	require.True(s.t, deleteCalled)
}

func (s *S) TestServiceManagerDeployServiceCancelRollback(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	buf := bytes.NewBuffer(nil)
	m := serviceManager{client: s.clusterClient, writer: buf}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:        eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:          permission.PermAppDeploy,
		Owner:         s.token,
		Allowed:       event.Allowed(permission.PermAppDeploy),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents),
		Cancelable:    true,
	})
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"web": {"cmd1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
		Event:   evt,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	deployCreated := make(chan struct{})
	s.client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if dep, ok := obj.(*appsv1.Deployment); ok {
			rev, _ := strconv.Atoi(dep.Annotations[replicaDepRevision])
			rev++
			dep.Annotations = map[string]string{
				replicaDepRevision: strconv.Itoa(rev),
			}
			dep.Status.UnavailableReplicas = 1
			deployCreated <- struct{}{}
		}
		return false, nil, nil
	})
	ctx, cancel := evt.CancelableContext(context.TODO())
	defer cancel()
	go func(id string) {
		<-deployCreated
		evtDB, errCancel := event.GetByHexID(context.TODO(), id)
		require.NoError(s.t, errCancel)
		errCancel = evtDB.TryCancel(context.TODO(), "Because i want.", "admin@admin.com")
		require.NoError(s.t, errCancel)
	}(evt.UniqueID.Hex())
	err = servicecommon.RunServicePipeline(ctx, &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
		Event:   evt,
	}, servicecommon.ProcessSpec{
		"web": servicecommon.ProcessState{Start: true},
	})
	require.NotNil(s.t, err)
	require.IsType(s.t, provision.ErrUnitStartup{}, err)
	require.ErrorContains(s.t, err, "canceled by user action")
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Contains(s.t, buf.String(), "---> 1 of 1 new units created")
	require.Contains(s.t, buf.String(), "---> 0 of 1 new units ready")
	require.Contains(s.t, buf.String(), "ROLLING BACK AFTER FAILURE")
}

func (s *S) TestServiceManagerDeployServiceWithUID(c *check.C) {
	config.Set("docker:uid", 1001)
	defer config.Unset("docker:uid")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{"p1": {"cm1"}})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	expectedUID := int64(1001)
	require.EqualValues(s.t, &apiv1.PodSecurityContext{RunAsUser: &expectedUID}, dep.Spec.Template.Spec.SecurityContext)
}

func (s *S) TestServiceManagerDeployServiceWithResourceRequirements(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	a.Plan = appTypes.Plan{Memory: 1024}
	version := newCommittedVersion(c, a, map[string][]string{"p1": {"cm1"}})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	expectedMemory := resource.NewQuantity(1024, resource.BinarySI)
	require.EqualValues(s.t, apiv1.ResourceRequirements{
		Limits: apiv1.ResourceList{
			apiv1.ResourceMemory:           *expectedMemory,
			apiv1.ResourceEphemeralStorage: defaultEphemeralStorageLimit,
		},
		Requests: apiv1.ResourceList{
			apiv1.ResourceMemory:           *expectedMemory,
			apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
		},
	}, dep.Spec.Template.Spec.Containers[0].Resources)
}

func (s *S) TestServiceManagerDeployServiceWithClusterWideOvercommitFactor(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	s.clusterClient.CustomData[overcommitClusterKey] = "3"
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	a.Plan = appTypes.Plan{Memory: 1024}
	version := newCommittedVersion(c, a, map[string][]string{"p1": {"cm1"}})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	expectedMemory := resource.NewQuantity(1024, resource.BinarySI)
	expectedMemoryRequest := resource.NewQuantity(341, resource.BinarySI)
	require.EqualValues(s.t, apiv1.ResourceRequirements{
		Limits: apiv1.ResourceList{
			apiv1.ResourceMemory:           *expectedMemory,
			apiv1.ResourceEphemeralStorage: defaultEphemeralStorageLimit,
		},
		Requests: apiv1.ResourceList{
			apiv1.ResourceMemory:           *expectedMemoryRequest,
			apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
		},
	}, dep.Spec.Template.Spec.Containers[0].Resources)
}

func (s *S) TestServiceManagerDeployServiceWithClusterPoolOvercommitFactor(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	s.clusterClient.CustomData[overcommitClusterKey] = "3"
	s.clusterClient.CustomData["test-default:"+overcommitClusterKey] = "2"
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	a.Plan = appTypes.Plan{Memory: 1024}
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	expectedMemory := resource.NewQuantity(1024, resource.BinarySI)
	expectedMemoryRequest := resource.NewQuantity(512, resource.BinarySI)
	require.EqualValues(s.t, apiv1.ResourceRequirements{
		Limits: apiv1.ResourceList{
			apiv1.ResourceMemory:           *expectedMemory,
			apiv1.ResourceEphemeralStorage: defaultEphemeralStorageLimit,
		},
		Requests: apiv1.ResourceList{
			apiv1.ResourceMemory:           *expectedMemoryRequest,
			apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
		},
	}, dep.Spec.Template.Spec.Containers[0].Resources)
}

func (s *S) TestServiceManagerDeployServiceWithCustomEphemeralStorageLimit(c *check.C) {
	tests := []struct {
		key      string
		value    string
		expected apiv1.ResourceRequirements
	}{
		{
			expected: apiv1.ResourceRequirements{
				Limits: apiv1.ResourceList{
					apiv1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
					apiv1.ResourceCPU:              resource.MustParse("1000m"),
					apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
				},
				Requests: apiv1.ResourceList{
					apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
					apiv1.ResourceCPU:              resource.MustParse("1000m"),
					apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
				},
			},
		},
		{
			key:   ephemeralStorageKey,
			value: "9Mi",
			expected: apiv1.ResourceRequirements{
				Limits: apiv1.ResourceList{
					apiv1.ResourceEphemeralStorage: resource.MustParse("9Mi"),
					apiv1.ResourceCPU:              resource.MustParse("1000m"),
					apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
				},
				Requests: apiv1.ResourceList{
					apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
					apiv1.ResourceCPU:              resource.MustParse("1000m"),
					apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
				},
			},
		},
		{
			key:   "test-default:" + ephemeralStorageKey,
			value: "1Mi",
			expected: apiv1.ResourceRequirements{
				Limits: apiv1.ResourceList{
					apiv1.ResourceEphemeralStorage: resource.MustParse("1Mi"),
					apiv1.ResourceCPU:              resource.MustParse("1000m"),
					apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
				},
				Requests: apiv1.ResourceList{
					apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
					apiv1.ResourceCPU:              resource.MustParse("1000m"),
					apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
				},
			},
		},
		{
			key:   "other:" + ephemeralStorageKey,
			value: "1Mi",
			expected: apiv1.ResourceRequirements{
				Limits: apiv1.ResourceList{
					apiv1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
					apiv1.ResourceCPU:              resource.MustParse("1000m"),
					apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
				},
				Requests: apiv1.ResourceList{
					apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
					apiv1.ResourceCPU:              resource.MustParse("1000m"),
					apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
				},
			},
		},
		{
			key:   ephemeralStorageKey,
			value: "0",
			expected: apiv1.ResourceRequirements{
				Limits: apiv1.ResourceList{
					apiv1.ResourceCPU:    resource.MustParse("1000m"),
					apiv1.ResourceMemory: *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
				},
				Requests: apiv1.ResourceList{
					apiv1.ResourceCPU:    resource.MustParse("1000m"),
					apiv1.ResourceMemory: *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
				},
			},
		},
	}
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	for i, tt := range tests {
		c.Logf("test %d", i)
		for k := range s.clusterClient.CustomData {
			delete(s.clusterClient.CustomData, k)
		}
		s.clusterClient.CustomData[tt.key] = tt.value
		m := serviceManager{client: s.clusterClient}
		err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
			App:     a,
			Version: version,
		}, servicecommon.ProcessSpec{
			"p1": servicecommon.ProcessState{Start: true},
		})
		require.NoError(s.t, err)
		waitDep()
		dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
		require.NoError(s.t, err)
		require.EqualValues(s.t, tt.expected, dep.Spec.Template.Spec.Containers[0].Resources)
	}
}

func (s *S) TestServiceManagerDeployServiceWithClusterWideMaxSurgeAndUnavailable(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	s.clusterClient.CustomData[maxSurgeKey] = "30%"
	s.clusterClient.CustomData[maxUnavailableKey] = "2"
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	a.Plan = appTypes.Plan{Memory: 1024}
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	maxSurge := intstr.FromString("30%")
	maxUnavailable := intstr.FromInt(2)
	require.EqualValues(s.t, &appsv1.RollingUpdateDeployment{
		MaxSurge:       &maxSurge,
		MaxUnavailable: &maxUnavailable,
	}, dep.Spec.Strategy.RollingUpdate)
}

func (s *S) TestServiceManagerDeploySinglePoolEnable(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	s.clusterClient.CustomData[singlePoolKey] = "true"
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, map[string]string(nil), dep.Spec.Template.Spec.NodeSelector)
}

func (s *S) TestServiceManagerDeployDnsConfigNdotsEnable(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	s.clusterClient.CustomData[dnsConfigNdotsKey] = "1"
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	dnsConfigNdotsValue := "1"
	require.EqualValues(s.t, &apiv1.PodDNSConfig{Options: []apiv1.PodDNSConfigOption{{Name: "ndots", Value: &dnsConfigNdotsValue}}}, dep.Spec.Template.Spec.DNSConfig)
}

func (s *S) TestServiceManagerDeployTopologySpreadConstraintEnable(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	s.clusterClient.CustomData[topologySpreadConstraintsKey] = "[{\"maxskew\":1, \"topologykey\":\"kubernetes.io/hostname\"}, {\"maxskew\":2, \"topologykey\":\"kubernetes.io/zone\"}]"
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	topologySpreadConstraints := []apiv1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       "kubernetes.io/hostname",
			WhenUnsatisfiable: apiv1.ScheduleAnyway,
			LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "p1", "tsuru.io/app-version": "1"}},
		},
		{
			MaxSkew:           2,
			TopologyKey:       "kubernetes.io/zone",
			WhenUnsatisfiable: apiv1.ScheduleAnyway,
			LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"tsuru.io/app-name": "myapp", "tsuru.io/app-process": "p1", "tsuru.io/app-version": "1"}},
		},
	}
	require.EqualValues(s.t, topologySpreadConstraints, dep.Spec.Template.Spec.TopologySpreadConstraints)
}

func (s *S) TestServiceManagerDeployServiceWithPreserveVersions(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version1 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	version2 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version1,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:              a,
		Version:          version2,
		PreserveVersions: true,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	v2Dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v1", metav1.GetOptions{})
	require.NoError(s.t, err)
	v2Svc, err := s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)

	depLabels := map[string]string{
		"tsuru.io/is-tsuru":                "true",
		"tsuru.io/is-service":              "true",
		"tsuru.io/is-build":                "false",
		"tsuru.io/is-stopped":              "false",
		"tsuru.io/is-isolated-run-version": "false",
		"tsuru.io/app-name":                "myapp",
		"tsuru.io/app-team":                "admin",
		"tsuru.io/app-process":             "p1",
		"tsuru.io/app-platform":            "",
		"tsuru.io/app-pool":                "test-default",
		"app":                              "myapp-p1",
		"app.kubernetes.io/component":      "tsuru-app",
		"app.kubernetes.io/managed-by":     "tsuru",
		"app.kubernetes.io/name":           "myapp",
		"app.kubernetes.io/instance":       "myapp-p1",
	}
	podLabels := map[string]string{
		"tsuru.io/is-tsuru":                "true",
		"tsuru.io/app-name":                "myapp",
		"tsuru.io/app-team":                "admin",
		"tsuru.io/app-process":             "p1",
		"tsuru.io/is-build":                "false",
		"tsuru.io/app-pool":                "test-default",
		"tsuru.io/app-platform":            "",
		"tsuru.io/app-version":             "2",
		"tsuru.io/is-isolated-run-version": "false",
	}
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	one := int32(1)
	ten := int32(10)
	maxSurge := intstr.FromString("100%")
	maxUnavailable := intstr.FromInt(0)
	expectedUID := int64(1000)
	testBaseImage, err := version2.BaseImageName()
	require.NoError(s.t, err)
	expected := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-v2",
			Namespace: nsName,
			Labels:    depLabels,
			Annotations: map[string]string{
				secretHashAnnotationKey: "f3f68faf3959035e45fe666a94854e2a76ff017ec775568944352586ca3d3fc5",
			},
		},
		Status: appsv1.DeploymentStatus{
			UpdatedReplicas: 1,
			Replicas:        1,
		},
		Spec: appsv1.DeploymentSpec{
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       &maxSurge,
					MaxUnavailable: &maxUnavailable,
				},
			},
			Replicas:             &one,
			RevisionHistoryLimit: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"tsuru.io/app-name":                "myapp",
					"tsuru.io/app-process":             "p1",
					"tsuru.io/is-build":                "false",
					"tsuru.io/is-isolated-run-version": "false",
					"tsuru.io/app-version":             "2",
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
					Annotations: map[string]string{
						secretHashAnnotationKey: "f3f68faf3959035e45fe666a94854e2a76ff017ec775568944352586ca3d3fc5",
					},
				},
				Spec: apiv1.PodSpec{
					EnableServiceLinks: func(b bool) *bool { return &b }(false),
					ServiceAccountName: "app-myapp",
					SecurityContext: &apiv1.PodSecurityContext{
						RunAsUser: &expectedUID,
					},
					NodeSelector: map[string]string{
						"tsuru.io/pool": "test-default",
					},
					RestartPolicy:                 "Always",
					Subdomain:                     "myapp-p1-units",
					TerminationGracePeriodSeconds: func(v int64) *int64 { return &v }(40),
					Containers: []apiv1.Container{
						{
							Name:  "myapp-p1-v2",
							Image: testBaseImage,
							Command: []string{
								"/bin/sh",
								"-lc",
								"[ -d /home/application/current ] && cd /home/application/current; exec cm1",
							},
							Env: []apiv1.EnvVar{
								{Name: "TSURU_APPDIR", Value: "/home/application/current"},
								{Name: "TSURU_APPNAME", Value: a.Name},
								{Name: "TSURU_SERVICES", ValueFrom: &apiv1.EnvVarSource{
									SecretKeyRef: &apiv1.SecretKeySelector{
										LocalObjectReference: apiv1.LocalObjectReference{
											Name: appSecretPrefix + "myapp-p1-v2",
										},
										Key: "TSURU_SERVICES",
									},
								}},
								{Name: "TSURU_PROCESSNAME", Value: "p1"},
								{Name: "TSURU_APPVERSION", Value: "2"},
								{Name: "TSURU_HOST", Value: ""},
								{Name: "port", Value: "8888"},
								{Name: "PORT", Value: "8888"},
								{Name: "PORT_p1", Value: "8888"},
							},
							Resources: apiv1.ResourceRequirements{
								Limits: apiv1.ResourceList{
									apiv1.ResourceEphemeralStorage: defaultEphemeralStorageLimit,
									apiv1.ResourceCPU:              resource.MustParse("1000m"),
									apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
								},
								Requests: apiv1.ResourceList{
									apiv1.ResourceEphemeralStorage: *resource.NewQuantity(0, resource.DecimalSI),
									apiv1.ResourceCPU:              resource.MustParse("1000m"),
									apiv1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
								},
							},
							Ports: []apiv1.ContainerPort{
								{ContainerPort: 8888},
							},
							Lifecycle: &apiv1.Lifecycle{
								PreStop: &apiv1.LifecycleHandler{
									Exec: &apiv1.ExecAction{
										Command: []string{"sh", "-c", "sleep 10 || true"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.EqualValues(s.t, expected, v2Dep, "Diff", strings.Join(pretty.Diff(expected, v2Dep), "\n"))

	expectedSvc := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-v2",
			Namespace: nsName,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":                "true",
				"tsuru.io/is-service":              "true",
				"tsuru.io/is-build":                "false",
				"tsuru.io/is-stopped":              "false",
				"tsuru.io/is-isolated-run-version": "false",
				"tsuru.io/app-name":                "myapp",
				"tsuru.io/app-team":                "admin",
				"tsuru.io/app-process":             "p1",
				"tsuru.io/app-version":             "2",
				"tsuru.io/app-platform":            "",
				"tsuru.io/app-pool":                "test-default",
				"version":                          "v2",
				"app":                              "myapp-p1",
				"app.kubernetes.io/component":      "tsuru-app",
				"app.kubernetes.io/managed-by":     "tsuru",
				"app.kubernetes.io/name":           "myapp",
				"app.kubernetes.io/instance":       "myapp-p1",
				"app.kubernetes.io/version":        "v2",
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":                "myapp",
				"tsuru.io/app-process":             "p1",
				"tsuru.io/app-version":             "2",
				"tsuru.io/is-build":                "false",
				"tsuru.io/is-isolated-run-version": "false",
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(8888),
					TargetPort: intstr.FromInt(8888),
					Name:       "http-default-1",
				},
			},
			Type: apiv1.ServiceTypeClusterIP,
		},
	}
	require.EqualValues(s.t, expectedSvc, v2Svc, "Diff", strings.Join(pretty.Diff(expectedSvc, v2Svc), "\n"))

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:              a,
		Version:          version2,
		PreserveVersions: true,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true, Restart: true},
	})
	require.NoError(s.t, err)
	waitDep()

	expected.Labels["tsuru.io/restarts"] = "1"
	expected.Spec.Template.ObjectMeta.Labels["tsuru.io/restarts"] = "1"
	expectedSvc.Labels["tsuru.io/restarts"] = "1"

	v2Dep, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expected, v2Dep, "Diff", strings.Join(pretty.Diff(expected, v2Dep), "\n"))
	v2Svc, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedSvc, v2Svc, "Diff", strings.Join(pretty.Diff(expectedSvc, v2Svc), "\n"))
}

func (s *S) TestServiceManagerDeployServiceWithRemovedOldVersion(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version1 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cm2"},
	})
	version2 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:              a,
		Version:          version1,
		PreserveVersions: false,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
		"p2": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	err = servicemanager.AppVersion.DeleteVersionIDs(context.TODO(), a.Name, []int{version1.Version()})
	require.NoError(s.t, err)

	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "1", dep.Spec.Template.Labels["tsuru.io/app-version"])

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:              a,
		Version:          version2,
		PreserveVersions: false,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	dep, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "2", dep.Spec.Template.Labels["tsuru.io/app-version"])

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	_, err = s.client.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), appSecretPrefix+"myapp-p2", metav1.GetOptions{})
	c.Check(k8sErrors.IsNotFound(err), check.Equals, true)

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
}

// NOTE:(ravilock) Regression test for app rollbacking into a state where the user cant manage the app
// This scenario creates a app with multiversion deployment:
// version 1 (base) with processes p1 and p2
// version 2 (versioned) with process p1
// version 3 (versioned) with process p1
// rollbacking the app to version2 from this state, used to create two deployments
// version 2 (base) with processes p1
// version 2 (versioned) with process p1
// now, rollbacking only keep the base deployment with correct version
// version 2 (base) with processes p1
func (s *S) TestServiceManagerDeployServiceRemovingOtherVersionsCleanup(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version1 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cm2"},
	})
	version2 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm3"},
	})
	version3 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm4"},
	})

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	// Create version 1 (base) with processes p1 and p2
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:              a,
		Version:          version1,
		PreserveVersions: false,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
		"p2": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	err = servicemanager.AppVersion.DeleteVersionIDs(context.TODO(), a.Name, []int{version1.Version()})
	require.NoError(s.t, err)

	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "1", dep.Spec.Template.Labels["tsuru.io/app-version"])
	dep, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "1", dep.Spec.Template.Labels["tsuru.io/app-version"])
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	// Add version 2 (versioned) with process p1
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:              a,
		Version:          version2,
		PreserveVersions: true,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)

	// Add version 3 (versioned) with process p1
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:              a,
		Version:          version3,
		PreserveVersions: true,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1-v3", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v3", metav1.GetOptions{})
	require.NoError(s.t, err)

	// Rollbacking to version 2 with PreserveVersions false
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:              a,
		Version:          version2,
		PreserveVersions: false,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	// assert that the base deployment is version 2
	dep, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "2", dep.Spec.Template.Labels["tsuru.io/app-version"])
	// assert that p2 process was removed
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	// Assert that versioned deployment for version 2 does not exists
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1-v3", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v3", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
}

func (s *S) TestServiceManagerDeployServiceWithRemovedProcess(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version1 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cm2"},
	})
	version2 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version1,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
		"p2": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "1", dep.Spec.Template.Labels["tsuru.io/app-version"])

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2-v1", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:              a,
		Version:          version2,
		PreserveVersions: true,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	dep, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "1", dep.Spec.Template.Labels["tsuru.io/app-version"])

	depP2, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "1", depP2.Spec.Template.Labels["tsuru.io/app-version"])

	depV2, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "2", depV2.Spec.Template.Labels["tsuru.io/app-version"])

	svcList, err := s.client.Clientset.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, svcList.Items, 6)

	// check all p1 services that should've been created
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v1", metav1.GetOptions{})
	require.NoError(s.t, err)

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-units", metav1.GetOptions{})
	require.NoError(s.t, err)

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.NoError(s.t, err)

	// check all p2 services that should still exist from first deployment
	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)

	_, err = s.client.Clientset.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2-units", metav1.GetOptions{})
	require.NoError(s.t, err)
}

func (s *S) TestServiceManagerDeployServiceWithEscapedEnvs(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	a.Env = map[string]bindTypes.EnvVar{
		"env1": {
			Name:  "env1",
			Value: "a$()b$$c",
		},
	}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:              a,
		Version:          version,
		PreserveVersions: true,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true, Restart: true},
	})
	require.NoError(s.t, err)
	waitDep()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, []apiv1.EnvVar{
		{Name: "TSURU_APPDIR", Value: "/home/application/current"},
		{Name: "TSURU_APPNAME", Value: a.Name},
		{Name: "TSURU_SERVICES", ValueFrom: &apiv1.EnvVarSource{
			SecretKeyRef: &apiv1.SecretKeySelector{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: appSecretPrefix + "myapp-p1",
				},
				Key: "TSURU_SERVICES",
			},
		}},
		{Name: "env1", ValueFrom: &apiv1.EnvVarSource{
			SecretKeyRef: &apiv1.SecretKeySelector{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: appSecretPrefix + "myapp-p1",
				},
				Key: "env1",
			},
		}},
		{Name: "TSURU_PROCESSNAME", Value: "p1"},
		{Name: "TSURU_APPVERSION", Value: "1"},
		{Name: "TSURU_HOST", Value: ""},
		{Name: "port", Value: "8888"},
		{Name: "PORT", Value: "8888"},
		{Name: "PORT_p1", Value: "8888"},
	}, dep.Spec.Template.Spec.Containers[0].Env)

	secret, err := s.client.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), appSecretPrefix+"myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, map[string][]byte{
		"TSURU_SERVICES": []byte("{}"),
		"env1":           []byte("a$()b$$c"),
	}, secret.Data)
}

func (s *S) TestServiceManagerDeployServiceWithVolumes(c *check.C) {
	config.Set("docker:uid", 1001)
	defer config.Unset("docker:uid")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	config.Set("volume-plans:p1:kubernetes:plugin", "nfs")
	defer config.Unset("volume-plans")
	v := volumeTypes.Volume{
		Name: "v1",
		Opts: map[string]string{
			"path":         "/exports",
			"server":       "192.168.1.1",
			"capacity":     "20Gi",
			"access-modes": string(apiv1.ReadWriteMany),
		},
		Plan: volumeTypes.VolumePlan{Name: "p1", Opts: map[string]interface{}{
			"plugin": "nfs",
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
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, []apiv1.Volume{
		{
			Name: "v1-tsuru",
			VolumeSource: apiv1.VolumeSource{
				PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
					ClaimName: "v1-tsuru-claim",
					ReadOnly:  false,
				},
			},
		},
	}, dep.Spec.Template.Spec.Volumes)
	require.EqualValues(s.t, []apiv1.VolumeMount{
		{
			Name:      "v1-tsuru",
			MountPath: "/mnt",
			ReadOnly:  false,
		},
	}, dep.Spec.Template.Spec.Containers[0].VolumeMounts)
}

func (s *S) TestServiceManagerDeployServiceRollbackFullTimeout(c *check.C) {
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	config.Set("kubernetes:deployment-progress-timeout", 2)
	defer config.Unset("kubernetes:deployment-progress-timeout")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	buf := bytes.Buffer{}
	m := serviceManager{client: s.clusterClient, writer: &buf}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)

	version1 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})

	version2 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})

	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version1,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	waitDep()
	reaction := func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		dep := obj.(*appsv1.Deployment)
		dep.Status.UnavailableReplicas = 2
		rev, _ := strconv.Atoi(dep.Annotations[replicaDepRevision])
		rev++
		dep.Annotations = map[string]string{
			replicaDepRevision: strconv.Itoa(rev),
		}
		return false, nil, nil
	}
	s.client.PrependReactor("create", "deployments", reaction)
	s.client.PrependReactor("update", "deployments", reaction)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		pod.Status.Conditions = append(pod.Status.Conditions, apiv1.PodCondition{
			Type:   apiv1.PodReady,
			Status: apiv1.ConditionFalse,
		})
		return false, nil, nil
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version2,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.ErrorContains(s.t, err, `Pod "myapp-p1-pod-2-1" not ready`)
	waitDep()

	dep, err := s.client.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "1", dep.Spec.Template.ObjectMeta.Labels["tsuru.io/app-version"])

	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)

	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))

	require.Contains(s.t, buf.String(), "---- Updating units [p1] [version 1] ----")
	require.Contains(s.t, buf.String(), "HEALTHCHECK TIMEOUT OF 1S EXCEEDED")
	require.Contains(s.t, buf.String(), "ROLLING BACK AFTER FAILURE")
	err = cleanupDeployment(context.TODO(), s.clusterClient, a, "p1", version1.Version())
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Events(ns).Create(context.TODO(), &apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod.evt1",
			Namespace: ns,
		},
		Reason:  "Unhealthy",
		Message: "my evt message",
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version2,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.ErrorContains(s.t, err, `Pod "myapp-p1-pod-4-1" not ready`)
	require.ErrorContains(s.t, err, `Pod "myapp-p1-pod-4-1" failed health check: my evt message`)
	waitDep()
}

func (s *S) TestServiceManagerDeployServiceRollbackPendingPod(c *check.C) {
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	config.Set("kubernetes:deployment-progress-timeout", 2)
	defer config.Unset("kubernetes:deployment-progress-timeout")
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	buf := bytes.Buffer{}
	m := serviceManager{client: s.clusterClient, writer: &buf}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version1 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cmd1"},
	})
	version2 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cmd1"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version1,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	s.client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		dep := obj.(*appsv1.Deployment)
		rev, _ := strconv.Atoi(dep.Annotations[replicaDepRevision])
		rev++
		dep.Annotations = map[string]string{
			replicaDepRevision: strconv.Itoa(rev),
		}
		dep.Status.UnavailableReplicas = 2
		dep.Status.ObservedGeneration = 12
		labelsCp := make(map[string]string, len(dep.Labels))
		for k, v := range dep.Spec.Template.Labels {
			labelsCp[k] = v
		}
		return false, nil, nil
	})
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		pod.Status.Conditions = append(pod.Status.Conditions, apiv1.PodCondition{
			Type:   apiv1.PodReady,
			Status: apiv1.ConditionFalse,
		})
		pod.Status.Phase = apiv1.PodPending
		pod.OwnerReferences = append(pod.OwnerReferences, metav1.OwnerReference{
			Kind: "ReplicaSet",
			Name: "replica-for-myapp-p1",
		})
		return false, nil, nil
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version2,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.ErrorContains(s.t, err, `Pod "myapp-p1-pod-2-1" not ready`)
	waitDep()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	dep, err := s.client.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "1", dep.Spec.Template.ObjectMeta.Labels["tsuru.io/app-version"])
}

func (s *S) TestServiceManagerDeployServiceProcessHealthcheckTimeoutExceeded(c *check.C) {
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	config.Set("kubernetes:deployment-progress-timeout", 10)
	defer config.Unset("kubernetes:deployment-progress-timeout")

	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()

	buf := bytes.Buffer{}
	m := serviceManager{client: s.clusterClient, writer: &buf}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)

	version1 := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cmd1"},
	})

	version2 := newCommittedVersion(c, a,
		map[string][]string{
			"p1": {"cmd1"},
		},
		map[string]interface{}{
			"processes": []provTypes.TsuruYamlProcess{
				{
					Name: "p1",
					Healthcheck: &provTypes.TsuruYamlHealthcheck{
						DeployTimeoutSeconds: 5,
					},
				},
			},
		},
	)

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version1,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	reaction := func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		dep := obj.(*appsv1.Deployment)
		rev, _ := strconv.Atoi(dep.Annotations[replicaDepRevision])
		rev++
		dep.Annotations = map[string]string{
			replicaDepRevision: strconv.Itoa(rev),
		}
		dep.Status.UnavailableReplicas = 2
		return false, nil, nil
	}

	s.client.PrependReactor("create", "deployments", reaction)
	s.client.PrependReactor("update", "deployments", reaction)
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		pod.Status.Conditions = append(pod.Status.Conditions, apiv1.PodCondition{
			Type:   apiv1.PodReady,
			Status: apiv1.ConditionFalse,
		})
		pod.OwnerReferences = append(pod.OwnerReferences, metav1.OwnerReference{
			Kind: "ReplicaSet",
			Name: "replica-for-myapp-p1",
		})
		return false, nil, nil
	})

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version2,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.ErrorContains(s.t, err, "Pod \"myapp-p1-pod-2-1\" not ready")
	waitDep()

	dep, err := s.client.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "1", dep.Spec.Template.ObjectMeta.Labels["tsuru.io/app-version"])

	require.Contains(s.t, buf.String(), "---- Updating units [p1] [version 1] ----")
	require.Contains(s.t, buf.String(), "HEALTHCHECK TIMEOUT OF 5S EXCEEDED")
	require.Contains(s.t, buf.String(), "ROLLING BACK AFTER FAILURE")
}

func (s *S) TestServiceManagerDeployServiceNoRollbackFullTimeoutSameRevision(c *check.C) {
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	config.Set("kubernetes:deployment-progress-timeout", 2)
	defer config.Unset("kubernetes:deployment-progress-timeout")
	buf := bytes.Buffer{}
	m := serviceManager{client: s.clusterClient, writer: &buf}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	require.NoError(s.t, err)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.AppsV1().Deployments(ns).Create(context.TODO(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "myapp-p1",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	var rollbackCalled bool
	s.client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(ktesting.CreateAction).GetObject()
		if action.GetSubresource() == "rollback" {
			rollbackCalled = true
			return false, nil, nil
		}
		dep := obj.(*appsv1.Deployment)
		dep.Status.UnavailableReplicas = 2
		return false, nil, nil
	})
	s.client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		pod := action.(ktesting.CreateAction).GetObject().(*apiv1.Pod)
		pod.Status.Conditions = append(pod.Status.Conditions, apiv1.PodCondition{
			Type:   apiv1.PodReady,
			Status: apiv1.ConditionFalse,
		})
		return false, nil, nil
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.ErrorContains(s.t, err, "Pod \"myapp-p1-pod-1-1\" not ready")
	waitDep()
	require.False(s.t, rollbackCalled)
	require.Contains(s.t, buf.String(), "---- Updating units [p1] [version 1] ----")
	require.Contains(s.t, buf.String(), "DEPLOYMENT PROGRESS TIMEOUT OF 2S EXCEEDED")
	require.Contains(s.t, buf.String(), "UPDATING BACK AFTER FAILURE")
}

func (s *S) TestServiceManagerDeployServiceNoChanges(c *check.C) {
	config.Set("docker:healthcheck:max-time", 1)
	defer config.Unset("docker:healthcheck:max-time")
	config.Set("kubernetes:deployment-progress-timeout", 2)
	defer config.Unset("kubernetes:deployment-progress-timeout")
	buf := bytes.Buffer{}
	m := serviceManager{client: s.clusterClient, writer: &buf}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	require.NoError(s.t, err)

	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	var rollbackCalled bool
	s.client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "rollback" {
			rollbackCalled = true
			return false, nil, nil
		}
		return false, nil, nil
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	buf.Reset()

	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	require.False(s.t, rollbackCalled)
	require.Contains(s.t, buf.String(), "---- Updating units [p1] [version 1] ----")
	require.Contains(s.t, buf.String(), "---- No changes on units [p2] [version 1] ----")
}

func (s *S) TestServiceManagerRemoveService(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, nil)
	require.NoError(s.t, err)
	waitDep()
	expectedLabels := map[string]string{
		"tsuru.io/is-tsuru":        "true",
		"tsuru.io/is-build":        "false",
		"tsuru.io/is-stopped":      "false",
		"tsuru.io/is-service":      "true",
		"tsuru.io/is-isolated-run": "false",
		"tsuru.io/app-name":        a.Name,
		"tsuru.io/app-team":        a.TeamOwner,
		"tsuru.io/app-process":     "p1",
		"tsuru.io/app-version":     "1",
		"tsuru.io/restarts":        "0",
		"tsuru.io/app-platform":    a.Platform,
		"tsuru.io/app-pool":        a.Pool,
	}
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	deps, err := s.client.Clientset.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, deps.Items, 1)
	rs, err := s.client.Clientset.AppsV1().ReplicaSets(ns).Create(context.TODO(), &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xxx",
			Namespace: ns,
			Labels:    expectedLabels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&deps.Items[0], appsv1.SchemeGroupVersion.WithKind("Deployment")),
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Pods(ns).Create(context.TODO(), &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1-xyz",
			Namespace: ns,
			Labels:    expectedLabels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(rs, appsv1.SchemeGroupVersion.WithKind("ReplicaSet")),
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(s.t, err)
	err = m.RemoveService(context.TODO(), a, "p1", version.Version())
	require.NoError(s.t, err)
	deps, err = s.client.Clientset.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, deps.Items, 0)
	srvs, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, srvs.Items, 0)
	replicas, err := s.client.Clientset.AppsV1().ReplicaSets(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, replicas.Items, 0)
	pods, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, pods.Items, 0)
}

func (s *S) TestServiceManagerRemoveServiceMiddleFailure(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, nil)
	require.NoError(s.t, err)
	waitDep()
	s.client.PrependReactor("delete", "deployments", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("my dep err")
	})
	err = m.RemoveService(context.TODO(), a, "p1", version.Version())
	require.ErrorContains(s.t, err, "my dep err")
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	deps, err := s.client.Clientset.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, deps.Items, 1)
	srvs, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, srvs.Items, 0)
}

func (s *S) TestDefineSelectorAndAffinity(c *check.C) {
	tt := []struct {
		name       string
		app        *appTypes.App
		poolLabels map[string]string
		customData map[string]string
		assertion  func(selector map[string]string, affinity *apiv1.Affinity, err error, c *check.C)
	}{
		{
			name:       "when cluster has a single pool",
			customData: map[string]string{singlePoolKey: "true"},
			app:        &appTypes.App{Name: "myapp", TeamOwner: s.team.Name, Pool: "test-default"},
			assertion: func(selector map[string]string, affinity *apiv1.Affinity, err error, c *check.C) {
				require.NoError(s.t, err)
				require.Nil(s.t, selector)
				require.Nil(s.t, affinity)
			},
		},
		{
			name:       "when pool has node affinity",
			app:        &appTypes.App{Name: "myapp", TeamOwner: s.team.Name, Pool: "test-default"},
			poolLabels: map[string]string{"affinity": `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"kubernetes.io/hostname","operator":"In","values":["minikube"]}]}]}}}`},
			assertion: func(selector map[string]string, affinity *apiv1.Affinity, err error, c *check.C) {
				require.NoError(s.t, err)
				require.Nil(s.t, selector)
				require.EqualValues(s.t, &apiv1.Affinity{
					NodeAffinity: &apiv1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
							NodeSelectorTerms: []apiv1.NodeSelectorTerm{
								{
									MatchExpressions: []apiv1.NodeSelectorRequirement{
										{
											Key:      "kubernetes.io/hostname",
											Operator: "In",
											Values:   []string{"minikube"},
										},
									},
								},
							},
						},
					},
				}, affinity)
			},
		},
		{
			name:       "when pool does not have node affinity and cluster disables default node selector",
			app:        &appTypes.App{Name: "myapp", TeamOwner: s.team.Name, Pool: "test-default"},
			customData: map[string]string{disableDefaultNodeSelectorKey: "true"},
			poolLabels: map[string]string{"affinity": `{"empty-affinity":"some-value"}`},
			assertion: func(selector map[string]string, affinity *apiv1.Affinity, err error, c *check.C) {
				require.NoError(s.t, err)
				require.Nil(s.t, selector)
				require.EqualValues(s.t, &apiv1.Affinity{}, affinity)
			},
		},
		{
			name: "when pool affinity is nil and cluster has default node selector",
			app:  &appTypes.App{Name: "myapp", TeamOwner: s.team.Name, Pool: "test-default"},
			assertion: func(selector map[string]string, affinity *apiv1.Affinity, err error, c *check.C) {
				require.NoError(s.t, err)
				require.EqualValues(s.t, map[string]string{"tsuru.io/pool": "test-default"}, selector)
				require.Nil(s.t, affinity)
			},
		},
		{
			name: "when app pool does not exist",
			app:  &appTypes.App{Name: "myapp", TeamOwner: s.team.Name, Pool: "invalid pool"},
			assertion: func(selector map[string]string, affinity *apiv1.Affinity, err error, c *check.C) {
				require.Nil(s.t, selector)
				require.Nil(s.t, affinity)
				require.Error(s.t, err)
				require.ErrorContains(s.t, err, "Pool does not exist.")
			},
		},
		{
			name:       "when cluster default node selector key in custom data is invalid",
			app:        &appTypes.App{Name: "myapp", TeamOwner: s.team.Name, Pool: "test-default"},
			customData: map[string]string{disableDefaultNodeSelectorKey: "invalid"},
			assertion: func(selector map[string]string, affinity *apiv1.Affinity, err error, c *check.C) {
				require.Nil(s.t, selector)
				require.Nil(s.t, affinity)
				require.Error(s.t, err)
				require.ErrorContains(s.t, err, fmt.Sprintf(`error while parsing cluster custom data entry: %s: strconv.ParseBool: parsing "invalid": invalid syntax`, disableDefaultNodeSelectorKey))
			},
		},
	}

	for _, t := range tt {
		err := pool.PoolUpdate(context.TODO(), "test-default", pool.UpdatePoolOptions{Labels: t.poolLabels})
		require.NoError(s.t, err)
		s.clusterClient.CustomData = t.customData
		selector, affinity, err := defineSelectorAndAffinity(context.TODO(), t.app, s.clusterClient)
		t.assertion(selector, affinity, err, c)
		err = pool.PoolUpdate(context.TODO(), "test-default", pool.UpdatePoolOptions{Labels: map[string]string{}})
		require.NoError(s.t, err)
	}
}

func (s *S) TestEnsureNamespace(c *check.C) {
	tests := []struct {
		name       string
		customData map[string]string
		expected   apiv1.Namespace
	}{
		{
			name: "myns",
			expected: apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: "myns",
				Labels: map[string]string{
					"name": "myns",
				},
			}},
		},
		{
			name: "myns",
			customData: map[string]string{
				"namespace-labels": "lb1= val1,lb2 =val2 ",
			},
			expected: apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: "myns",
				Labels: map[string]string{
					"lb1":  "val1",
					"lb2":  "val2",
					"name": "myns",
				},
			}},
		},
		{
			name: "myns",
			customData: map[string]string{
				"namespace-labels":       "lb1= val1,lb2 =val2 ",
				"myns:namespace-labels":  "lb3=val3",
				"other:namespace-labels": "lb4=val4",
			},
			expected: apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: "myns",
				Labels: map[string]string{
					"lb3":  "val3",
					"name": "myns",
				},
			}},
		},
		{
			name: "myns2",
			customData: map[string]string{
				"namespace-labels":      "lb1= val1,lb2 =val2 ",
				"myns:namespace-labels": "lb3=val3",
			},
			expected: apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: "myns2",
				Labels: map[string]string{
					"lb1":  "val1",
					"lb2":  "val2",
					"name": "myns2",
				},
			}},
		},
	}
	for _, tt := range tests {
		s.clusterClient.CustomData = tt.customData
		err := ensureNamespace(context.TODO(), s.clusterClient, tt.name)
		require.NoError(s.t, err)
		nss, err := s.client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
		require.NoError(s.t, err)
		require.EqualValues(s.t, []apiv1.Namespace{tt.expected}, nss.Items)
		err = s.client.CoreV1().Namespaces().Delete(context.TODO(), tt.name, metav1.DeleteOptions{})
		require.NoError(s.t, err)
	}
}

func (s *S) TestServiceManagerDeployServiceWithDisableHeadless(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	s.clusterClient.CustomData[disableHeadlessKey] = "true"
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	a.Plan = appTypes.Plan{Memory: 1024}
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-units", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	srv, err := s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: ns,
			Labels: map[string]string{
				"app":                          "myapp-p1",
				"app.kubernetes.io/component":  "tsuru-app",
				"app.kubernetes.io/managed-by": "tsuru",
				"app.kubernetes.io/name":       "myapp",
				"app.kubernetes.io/instance":   "myapp-p1",
				"tsuru.io/is-tsuru":            "true",
				"tsuru.io/is-service":          "true",
				"tsuru.io/is-build":            "false",
				"tsuru.io/is-stopped":          "false",
				"tsuru.io/is-routable":         "true",
				"tsuru.io/app-name":            "myapp",
				"tsuru.io/app-team":            "admin",
				"tsuru.io/app-process":         "p1",
				"tsuru.io/app-platform":        "",
				"tsuru.io/app-pool":            "test-default",
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/app-process": "p1",
				"tsuru.io/is-build":    "false",
				"tsuru.io/is-routable": "true",
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(8888),
					TargetPort: intstr.FromInt(8888),
					Name:       "http-default-1",
				},
			},
			Type: apiv1.ServiceTypeClusterIP,
		},
	}, srv)
}

func (s *S) TestServiceManagerDeployServicePartialRollback(c *check.C) {
	wgFunc := s.mock.DeploymentReactions(c)
	defer wgFunc()
	var rolloutFailureCalled bool
	var wg sync.WaitGroup
	f1 := func(action ktesting.Action) (bool, runtime.Object, error) {
		wg.Add(1)
		defer wg.Done()
		dep := action.(ktesting.CreateAction).GetObject().(*appsv1.Deployment)
		if dep == nil {
			dep = action.(ktesting.UpdateAction).GetObject().(*appsv1.Deployment)
		}
		if dep.Name == "myapp-p2" && dep.Spec.Template.Labels["tsuru.io/app-version"] == "2" {
			dep.Status.Conditions = append(dep.Status.Conditions, appsv1.DeploymentCondition{
				Type:   appsv1.DeploymentProgressing,
				Reason: deadlineExeceededProgressCond,
			})
			rolloutFailureCalled = true
			return true, dep, nil
		}
		if rolloutFailureCalled && dep.Name == "myapp-p1" && dep.Spec.Template.Labels["tsuru.io/app-version"] == "1" {
			dep.Status.Conditions = append(dep.Status.Conditions, appsv1.DeploymentCondition{
				Type:   appsv1.DeploymentProgressing,
				Reason: deadlineExeceededProgressCond,
			})
			return true, dep, nil
		}
		return false, nil, nil
	}
	s.client.PrependReactor("create", "deployments", f1)
	s.client.PrependReactor("update", "deployments", f1)
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	a.Plan = appTypes.Plan{Memory: 1024}
	manager := &serviceManager{client: s.clusterClient}
	firstVersion := newVersion(c, a, map[string][]string{"p1": {"cm1"}, "p2": {"cm2"}})
	err = servicecommon.RunServicePipeline(context.TODO(), manager, 0, provision.DeployArgs{App: a, Version: firstVersion}, nil)
	require.NoError(s.t, err)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:        eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:          permission.PermAppDeploy,
		Owner:         s.token,
		Allowed:       event.Allowed(permission.PermAppDeploy),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents),
		Cancelable:    true,
	})
	require.NoError(s.t, err)
	manager.writer = evt
	args := provision.DeployArgs{
		App:     a,
		Event:   evt,
		Version: newVersion(c, a, map[string][]string{"p1": {"CM1"}, "p2": {"CM2"}}),
	}
	err = servicecommon.RunServicePipeline(context.TODO(), manager, firstVersion.Version(), args, nil)
	require.NotNil(s.t, err)
	require.ErrorContains(s.t, err, `deployment "myapp-p2" exceeded its progress deadline`)
	require.ErrorContains(s.t, err, `error rolling back updated service for myapp[p1] [version 1]: deployment "myapp-p1" exceeded its progress deadline`)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "2", dep.Spec.Template.Labels["tsuru.io/app-version"])
	dep, err = s.client.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "1", dep.Spec.Template.Labels["tsuru.io/app-version"])
	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.True(s.t, rolloutFailureCalled)
	require.Nil(s.t, evt.Done(context.TODO(), err))
	require.Contains(s.t, evt.Log(), "UPDATING BACK AFTER FAILURE")
}

func (s *S) TestServiceManagerDeployServiceRollbackErrorSingleProcess(c *check.C) {
	wgFunc := s.mock.DeploymentReactions(c)
	defer wgFunc()
	var wg sync.WaitGroup
	counter := 0
	f1 := func(action ktesting.Action) (bool, runtime.Object, error) {
		wg.Add(1)
		defer wg.Done()
		counter++
		dep := action.(ktesting.CreateAction).GetObject().(*appsv1.Deployment)
		switch counter {
		case 2:
			dep.Status.Conditions = append(dep.Status.Conditions, appsv1.DeploymentCondition{
				Type:   appsv1.DeploymentProgressing,
				Reason: deadlineExeceededProgressCond,
			})
			return false, dep, nil
		case 3:
			dep.Spec.Template.Labels["tsuru.io/app-version"] = "2"
			return true, dep, errors.New("deploy error")
		}
		return false, nil, nil
	}
	s.client.PrependReactor("create", "deployments", f1)
	s.client.PrependReactor("update", "deployments", f1)
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	a.Plan = appTypes.Plan{Memory: 1024}
	manager := &serviceManager{client: s.clusterClient}
	firstVersion := newVersion(c, a, map[string][]string{"p1": {"cm1"}})
	err = servicecommon.RunServicePipeline(context.TODO(), manager, 0, provision.DeployArgs{App: a, Version: firstVersion}, nil)
	require.NoError(s.t, err)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:        eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:          permission.PermAppDeploy,
		Owner:         s.token,
		Allowed:       event.Allowed(permission.PermAppDeploy),
		AllowedCancel: event.Allowed(permission.PermAppUpdateEvents),
		Cancelable:    true,
	})
	require.NoError(s.t, err)
	manager.writer = evt
	args := provision.DeployArgs{
		App:     a,
		Event:   evt,
		Version: newVersion(c, a, map[string][]string{"p1": {"CM1"}}),
	}
	err = servicecommon.RunServicePipeline(context.TODO(), manager, firstVersion.Version(), args, nil)
	require.NotNil(s.t, err)
	require.ErrorContains(s.t, err, `deployment "myapp-p1" exceeded its progress deadline`)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, "2", dep.Spec.Template.Labels["tsuru.io/app-version"])
	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.CoreV1().Services(ns).Get(context.TODO(), "myapp-p1-v2", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	require.Nil(s.t, evt.Done(context.TODO(), err))
	require.Contains(s.t, evt.Log(), "UPDATING BACK AFTER FAILURE")
	require.Contains(s.t, evt.Log(), "ERROR DURING ROLLBACK")
}

func (s *S) TestServiceManagerDeployServiceWithCustomLabelsAndAnnotations(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
		Metadata: appTypes.Metadata{
			Annotations: []appTypes.MetadataItem{{Name: "tsuru.io/a", Value: "my custom annotation"}},
			Labels:      []appTypes.MetadataItem{{Name: "tsuru.io/logs", Value: "BACKUP"}},
		},
	}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	a.Plan = appTypes.Plan{Memory: 1024}
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	dep, err := s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, map[string]string{
		"tsuru.io/a":            "my custom annotation",
		secretHashAnnotationKey: "f3f68faf3959035e45fe666a94854e2a76ff017ec775568944352586ca3d3fc5",
	}, dep.Spec.Template.ObjectMeta.Annotations)
	require.Equal(s.t, "BACKUP", dep.Spec.Template.ObjectMeta.Labels["tsuru.io/logs"])
}

func (s *S) TestServiceManagerDeployServiceWithVPA(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	require.NoError(s.t, err)
	vpaCRD := &extensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "verticalpodautoscalers.autoscaling.k8s.io"},
	}
	_, err = s.client.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), vpaCRD, metav1.CreateOptions{})
	require.NoError(s.t, err)
	a.Metadata.Update(appTypes.Metadata{
		Annotations: []appTypes.MetadataItem{
			{Name: AnnotationEnableVPA, Value: "true"},
		},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.VPAClientset.AutoscalingV1().VerticalPodAutoscalers(ns).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
}

func (s *S) TestServiceManagerDeployServiceWithMinAvailablePDB(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	m := serviceManager{client: s.clusterClient}
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)
	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
		"p2": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()
	_, err = s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.Clientset.AppsV1().Deployments(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	pdb, err := s.client.PolicyV1().PodDisruptionBudgets(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p1",
			Namespace: nsName,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/app-process": "p1",
				"tsuru.io/app-team":    "admin",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"tsuru.io/app-name":    "myapp",
					"tsuru.io/app-process": "p1",
					"tsuru.io/is-routable": "true",
				},
			},
		},
	}, pdb)
	pdb, err = s.client.PolicyV1().PodDisruptionBudgets(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-p2",
			Namespace: nsName,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/app-process": "p2",
				"tsuru.io/app-team":    "admin",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"tsuru.io/app-name":    "myapp",
					"tsuru.io/app-process": "p2",
					"tsuru.io/is-routable": "true",
				},
			},
		},
	}, pdb)
}

func (s *S) TestServiceManagerDeployServiceRemovePDBFromRemovedProcess(c *check.C) {
	waitDep := s.mock.DeploymentReactions(c)
	defer waitDep()
	a := &appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), a, s.user)
	require.NoError(s.t, err)

	version := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
		"p2": {"cmd2"},
	})
	m := serviceManager{client: s.clusterClient}
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: version,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
		"p2": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	nsName, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.PolicyV1().PodDisruptionBudgets(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.PolicyV1().PodDisruptionBudgets(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.NoError(s.t, err)

	var buffer bytes.Buffer
	m.writer = &buffer

	newVersion := newCommittedVersion(c, a, map[string][]string{
		"p1": {"cm1"},
	})
	err = servicecommon.RunServicePipeline(context.TODO(), &m, 0, provision.DeployArgs{
		App:     a,
		Version: newVersion,
	}, servicecommon.ProcessSpec{
		"p1": servicecommon.ProcessState{Start: true},
	})
	require.NoError(s.t, err)
	waitDep()

	_, err = s.client.PolicyV1().PodDisruptionBudgets(nsName).Get(context.TODO(), "myapp-p1", metav1.GetOptions{})
	require.NoError(s.t, err)
	_, err = s.client.PolicyV1().PodDisruptionBudgets(nsName).Get(context.TODO(), "myapp-p2", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	require.Contains(s.t, buffer.String(), "Cleaning up PodDisruptionBudget myapp-p2")
}

func (s *S) TestGetImagePullSecrets(c *check.C) {
	tests := []struct {
		config      map[string]interface{}
		images      []string
		expectedRef []apiv1.LocalObjectReference
	}{
		{
			config: map[string]interface{}{
				"docker:registry":               "myreg1.com",
				"docker:registry-auth:username": "user",
				"docker:registry-auth:password": "pass",
			},
			images: []string{"myreg1.com/tsuru/go"},
			expectedRef: []apiv1.LocalObjectReference{
				{Name: "docker-config-tsuru"},
			},
		},
		{
			config: map[string]interface{}{
				"docker:registry":               "myreg1.com",
				"docker:registry-auth:username": "user",
				"docker:registry-auth:password": "pass",
			},
			images:      []string{"otherreg.com/tsuru/go"},
			expectedRef: nil,
		},
		{
			config: map[string]interface{}{
				"docker:registry":               "myreg1.com",
				"docker:registry-auth:username": "user",
				"docker:registry-auth:password": "pass",
			},
			images: []string{"otherreg.com/tsuru/go", "myreg1.com/tsuru/go"},
			expectedRef: []apiv1.LocalObjectReference{
				{Name: "docker-config-tsuru"},
			},
		},
		{
			config: map[string]interface{}{
				"docker:registry": "myreg1.com",
			},
			images:      []string{"myreg1.com/tsuru/go"},
			expectedRef: nil,
		},
		{
			config: map[string]interface{}{
				"docker:registry": "",
			},
			images:      []string{"tsuru/go"},
			expectedRef: nil,
		},
		{
			config: map[string]interface{}{
				"docker:registry":               "",
				"docker:registry-auth:username": "user",
				"docker:registry-auth:password": "pass",
			},
			images: []string{"tsuru/go"},
			expectedRef: []apiv1.LocalObjectReference{
				{Name: "docker-config-tsuru"},
			},
		},
	}

	for _, tt := range tests {
		for k, v := range tt.config {
			config.Set(k, v)
		}
		ref, err := getImagePullSecrets(context.TODO(), s.clusterClient, "ns1", tt.images...)
		require.NoError(s.t, err)
		require.EqualValues(s.t, tt.expectedRef, ref)
		for k := range tt.config {
			config.Unset(k)
		}
	}
}

func (s *S) TestGetPortsFromProcessesByProcessName(c *check.C) {
	tests := []struct {
		name             string
		processes        []provTypes.TsuruYamlProcess
		processName      string
		expectedFound    bool
		expectedPorts    []provTypes.TsuruYamlKubernetesProcessPortConfig
		expectedErrorMsg string
	}{
		{
			name: "process with ports configured",
			processes: []provTypes.TsuruYamlProcess{
				{
					Name: "web",
					Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
						{Port: 8080, TargetPort: 8080},
					},
				},
			},
			processName:   "web",
			expectedFound: true,
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 8080, TargetPort: 8080},
			},
		},
		{
			name: "process with nil ports - should not be found",
			processes: []provTypes.TsuruYamlProcess{
				{
					Name:  "web",
					Ports: nil,
				},
			},
			processName:   "web",
			expectedFound: false,
			expectedPorts: nil,
		},
		{
			name: "process with empty ports array - should not be found",
			processes: []provTypes.TsuruYamlProcess{
				{
					Name:  "web",
					Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{},
				},
			},
			processName:   "web",
			expectedFound: false,
			expectedPorts: nil,
		},
		{
			name: "process not in list",
			processes: []provTypes.TsuruYamlProcess{
				{
					Name: "worker",
					Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
						{Port: 8080},
					},
				},
			},
			processName:   "web",
			expectedFound: false,
			expectedPorts: nil,
		},
		{
			name: "duplicated process name - should error",
			processes: []provTypes.TsuruYamlProcess{
				{
					Name: "web",
					Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
						{Port: 8080},
					},
				},
				{
					Name: "web",
					Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
						{Port: 9090},
					},
				},
			},
			processName:      "web",
			expectedFound:    false,
			expectedErrorMsg: "duplicated process name: web",
		},
		{
			name: "multiple processes with one matching",
			processes: []provTypes.TsuruYamlProcess{
				{
					Name: "worker",
					Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
						{Port: 9090},
					},
				},
				{
					Name: "web",
					Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
						{Port: 8080, TargetPort: 8080, Protocol: "TCP"},
					},
				},
			},
			processName:   "web",
			expectedFound: true,
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 8080, TargetPort: 8080},
			},
		},
		{
			name: "process with UDP ports",
			processes: []provTypes.TsuruYamlProcess{
				{
					Name: "dns",
					Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
						{Port: 53, TargetPort: 53, Protocol: "udp"},
					},
				},
			},
			processName:   "dns",
			expectedFound: true,
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "udp-default-1", Protocol: "UDP", Port: 53, TargetPort: 53},
			},
		},
		{
			name: "process with multiple ports",
			processes: []provTypes.TsuruYamlProcess{
				{
					Name: "web",
					Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
						{Port: 8080, TargetPort: 8080},
						{Port: 9090, TargetPort: 9090},
					},
				},
			},
			processName:   "web",
			expectedFound: true,
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 8080, TargetPort: 8080},
				{Name: "http-default-2", Protocol: "TCP", Port: 9090, TargetPort: 9090},
			},
		},
	}

	for _, tt := range tests {
		c.Logf("Test: %s", tt.name)
		found, ports, err := getPortsFromProcessesByProcessName(tt.processes, tt.processName)
		if tt.expectedErrorMsg != "" {
			require.Error(s.t, err)
			require.Contains(s.t, err.Error(), tt.expectedErrorMsg)
		} else {
			require.NoError(s.t, err)
		}
		require.Equal(s.t, tt.expectedFound, found)
		require.EqualValues(s.t, tt.expectedPorts, ports)
	}
}

func (s *S) TestGetPortsFromTsuruYamlKubernetesByProcessName(c *check.C) {
	tests := []struct {
		name             string
		kubernetes       *provTypes.TsuruYamlKubernetesConfig
		processName      string
		expectedFound    bool
		expectedPorts    []provTypes.TsuruYamlKubernetesProcessPortConfig
		expectedErrorMsg string
	}{
		{
			name: "process with ports configured",
			kubernetes: &provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"group1": {
						"web": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{Port: 8080, TargetPort: 8080},
							},
						},
					},
				},
			},
			processName:   "web",
			expectedFound: true,
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 8080, TargetPort: 8080},
			},
		},
		{
			name: "process with nil ports - should not be found",
			kubernetes: &provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"group1": {
						"web": {
							Ports: nil,
						},
					},
				},
			},
			processName:   "web",
			expectedFound: false,
			expectedPorts: nil,
		},
		{
			name: "process with empty ports array - should be found (semantic for no service)",
			kubernetes: &provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"group1": {
						"web": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{},
						},
					},
				},
			},
			processName:   "web",
			expectedFound: true,
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{},
		},
		{
			name: "process not in list",
			kubernetes: &provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"group1": {
						"worker": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{Port: 8080},
							},
						},
					},
				},
			},
			processName:   "web",
			expectedFound: false,
			expectedPorts: nil,
		},
		{
			name: "duplicated process name in different groups - should error",
			kubernetes: &provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"group1": {
						"web": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{Port: 8080},
							},
						},
					},
					"group2": {
						"web": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{Port: 9090},
							},
						},
					},
				},
			},
			processName:      "web",
			expectedFound:    false,
			expectedErrorMsg: "duplicated process name: web",
		},
		{
			name: "multiple processes in different groups with one matching",
			kubernetes: &provTypes.TsuruYamlKubernetesConfig{
				Groups: map[string]provTypes.TsuruYamlKubernetesGroup{
					"group1": {
						"worker": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{Port: 9090},
							},
						},
					},
					"group2": {
						"web": {
							Ports: []provTypes.TsuruYamlKubernetesProcessPortConfig{
								{Port: 8080, TargetPort: 8080, Protocol: "TCP"},
							},
						},
					},
				},
			},
			processName:   "web",
			expectedFound: true,
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 8080, TargetPort: 8080},
			},
		},
	}

	for _, tt := range tests {
		c.Logf("Test: %s", tt.name)
		found, ports, err := getPortsFromTsuruYamlKubernetesByProcessName(tt.kubernetes, tt.processName)
		if tt.expectedErrorMsg != "" {
			require.Error(s.t, err)
			require.Contains(s.t, err.Error(), tt.expectedErrorMsg)
		} else {
			require.NoError(s.t, err)
		}
		require.Equal(s.t, tt.expectedFound, found)
		require.EqualValues(s.t, tt.expectedPorts, ports)
	}
}

func (s *S) TestApplyPortDefaults(c *check.C) {
	tests := []struct {
		name          string
		inputPorts    []provTypes.TsuruYamlKubernetesProcessPortConfig
		expectedPorts []provTypes.TsuruYamlKubernetesProcessPortConfig
	}{
		{
			name: "apply default protocol (TCP)",
			inputPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Port: 8080, TargetPort: 8080},
			},
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 8080, TargetPort: 8080},
			},
		},
		{
			name: "uppercase protocol conversion",
			inputPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Protocol: "tcp", Port: 8080, TargetPort: 8080},
			},
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 8080, TargetPort: 8080},
			},
		},
		{
			name: "UDP protocol with default name",
			inputPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Protocol: "udp", Port: 53, TargetPort: 53},
			},
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "udp-default-1", Protocol: "UDP", Port: 53, TargetPort: 53},
			},
		},
		{
			name: "preserve existing name",
			inputPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "custom-port", Protocol: "TCP", Port: 8080, TargetPort: 8080},
			},
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "custom-port", Protocol: "TCP", Port: 8080, TargetPort: 8080},
			},
		},
		{
			name: "set Port when only TargetPort is defined",
			inputPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{TargetPort: 8080},
			},
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 8080, TargetPort: 8080},
			},
		},
		{
			name: "set TargetPort when only Port is defined",
			inputPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Port: 8080},
			},
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 8080, TargetPort: 8080},
			},
		},
		{
			name: "multiple ports with different protocols",
			inputPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Port: 8080, TargetPort: 8080},
				{Protocol: "UDP", Port: 53, TargetPort: 53},
				{Protocol: "tcp", Port: 9090, TargetPort: 9090},
			},
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 8080, TargetPort: 8080},
				{Name: "udp-default-2", Protocol: "UDP", Port: 53, TargetPort: 53},
				{Name: "http-default-3", Protocol: "TCP", Port: 9090, TargetPort: 9090},
			},
		},
		{
			name: "port with different Port and TargetPort",
			inputPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Port: 80, TargetPort: 8080},
			},
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{
				{Name: "http-default-1", Protocol: "TCP", Port: 80, TargetPort: 8080},
			},
		},
		{
			name:          "empty ports array",
			inputPorts:    []provTypes.TsuruYamlKubernetesProcessPortConfig{},
			expectedPorts: []provTypes.TsuruYamlKubernetesProcessPortConfig{},
		},
	}

	for _, tt := range tests {
		c.Logf("Test: %s", tt.name)
		// Make a copy to avoid modifying test data
		ports := make([]provTypes.TsuruYamlKubernetesProcessPortConfig, len(tt.inputPorts))
		copy(ports, tt.inputPorts)

		applyPortDefaults(ports)
		require.EqualValues(s.t, tt.expectedPorts, ports)
	}
}
