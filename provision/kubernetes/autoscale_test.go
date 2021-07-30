// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"sort"
	"strconv"

	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
)

func (s *S) TestProvisionerSetAutoScale(c *check.C) {
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

	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
	})
	c.Assert(err, check.IsNil)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	hpa, err := s.client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	cpu := resource.MustParse("500m")
	c.Assert(hpa, check.DeepEquals, &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-web",
			Namespace: "default",
			Labels: map[string]string{
				"app":                          "myapp-web",
				"app.kubernetes.io/component":  "tsuru-app",
				"app.kubernetes.io/managed-by": "tsuru",
				"app.kubernetes.io/name":       "myapp",
				"app.kubernetes.io/version":    "v1",
				"app.kubernetes.io/instance":   "myapp-web",
				"tsuru.io/app-name":            "myapp",
				"tsuru.io/app-platform":        "python",
				"tsuru.io/app-team":            "",
				"tsuru.io/app-pool":            "test-default",
				"tsuru.io/app-process":         "web",
				"tsuru.io/app-version":         "1",
				"tsuru.io/builder":             "",
				"tsuru.io/is-build":            "false",
				"tsuru.io/is-deploy":           "false",
				"tsuru.io/is-service":          "true",
				"tsuru.io/is-stopped":          "false",
				"tsuru.io/is-tsuru":            "true",
				"tsuru.io/provisioner":         "kubernetes",
				"version":                      "v1",
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: func(i int32) *int32 { return &i }(1),
			MaxReplicas: int32(2),
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "myapp-web",
			},
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: "cpu",
						Target: autoscalingv2.MetricTarget{
							Type:         autoscalingv2.AverageValueMetricType,
							AverageValue: &cpu,
						},
					},
				},
			},
		},
	})
}

func (s *S) TestProvisionerSetAutoScaleMultipleVersions(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	versions := make([]appTypes.AppVersion, 4)
	for i := range versions {
		versions[i] = newSuccessfulVersion(c, a, map[string]interface{}{
			"processes": map[string]interface{}{
				"web": "python myapp.py",
			},
		})
	}

	err := s.p.AddUnits(context.TODO(), a, 1, "web", versions[0], nil)
	c.Assert(err, check.IsNil)
	wait()
	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
	})
	c.Assert(err, check.IsNil)

	tests := []struct {
		scenario           func()
		expectedDeployment string
		expectedVersion    int
	}{
		{
			scenario:           func() {},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.AddUnits(context.TODO(), a, 1, "web", versions[1], nil)
				c.Assert(err, check.IsNil)
				wait()
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.AddUnits(context.TODO(), a, 1, "web", versions[2], nil)
				c.Assert(err, check.IsNil)
				wait()
				err = s.p.ToggleRoutable(context.TODO(), a, versions[2], true)
				c.Assert(err, check.IsNil)
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.AddUnits(context.TODO(), a, 1, "web", versions[3], nil)
				c.Assert(err, check.IsNil)
				wait()
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[0], &bytes.Buffer{})
				c.Assert(err, check.IsNil)
				wait()
			},
			expectedDeployment: "myapp-web-v3",
			expectedVersion:    3,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[2], &bytes.Buffer{})
				c.Assert(err, check.IsNil)
				wait()
			},
			expectedDeployment: "myapp-web-v2",
			expectedVersion:    2,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[1], &bytes.Buffer{})
				c.Assert(err, check.IsNil)
				wait()
			},
			expectedDeployment: "myapp-web-v4",
			expectedVersion:    4,
		},
	}

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)

	for i, tt := range tests {
		c.Logf("test %d", i)
		tt.scenario()

		hpas, err := s.client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).List(context.TODO(), metav1.ListOptions{})
		c.Assert(err, check.IsNil)
		for _, hpa := range hpas.Items {
			dep, err := s.client.AppsV1().Deployments(ns).Get(context.TODO(), hpa.Spec.ScaleTargetRef.Name, metav1.GetOptions{})
			c.Assert(err, check.IsNil)
			if dep.Spec.Replicas != nil && *dep.Spec.Replicas > 0 {
				c.Assert(hpa.Spec.ScaleTargetRef.Name, check.Equals, tt.expectedDeployment)
				c.Assert(hpa.Labels["tsuru.io/app-version"], check.Equals, strconv.Itoa(tt.expectedVersion))
			}
		}
	}
}

func (s *S) TestProvisionerRemoveAutoScale(c *check.C) {
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

	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
	})
	c.Assert(err, check.IsNil)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	_, err = s.client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)

	err = s.p.RemoveAutoScale(context.TODO(), a, "web")
	c.Assert(err, check.IsNil)
	_, err = s.client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
}

func (s *S) TestProvisionerGetAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "python worker.py",
		},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	err = s.p.AddUnits(context.TODO(), a, 1, "worker", version, nil)
	c.Assert(err, check.IsNil)
	wait()

	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
		Process:    "web",
	})
	c.Assert(err, check.IsNil)

	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   2,
		MaxUnits:   4,
		AverageCPU: "200m",
		Process:    "worker",
	})
	c.Assert(err, check.IsNil)

	scales, err := s.p.GetAutoScale(context.TODO(), a)
	c.Assert(err, check.IsNil)
	sort.Slice(scales, func(i, j int) bool {
		return scales[i].Process < scales[j].Process
	})
	c.Assert(scales, check.DeepEquals, []provision.AutoScaleSpec{
		{
			MinUnits:   1,
			MaxUnits:   2,
			AverageCPU: "500m",
			Version:    1,
			Process:    "web",
		},
		{
			MinUnits:   2,
			MaxUnits:   4,
			AverageCPU: "200m",
			Version:    1,
			Process:    "worker",
		},
	})
}

func (s *S) TestEnsureVPAIfEnabled(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newCommittedVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "cm1",
		},
	})
	err := s.p.AddUnits(context.Background(), a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	wait()
	vpaUpdateOff := vpav1.UpdateModeOff

	tests := []struct {
		name        string
		scenario    func()
		expectedVPA *vpav1.VerticalPodAutoscaler
	}{
		{
			name:        "no crd, no annotation",
			expectedVPA: nil,
		},
		{
			name: "with crd, no annotation",
			scenario: func() {
				vpaCRD := &v1beta1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "verticalpodautoscalers.autoscaling.k8s.io"},
				}
				_, err := s.client.ApiextensionsV1beta1().CustomResourceDefinitions().Create(context.TODO(), vpaCRD, metav1.CreateOptions{})
				c.Assert(err, check.IsNil)
			},
			expectedVPA: nil,
		},
		{
			name: "with crd and annotation",
			scenario: func() {
				a.Metadata.Update(appTypes.Metadata{
					Annotations: []appTypes.MetadataItem{
						{Name: annotationEnableVPA, Value: "true"},
					},
				})
			},
			expectedVPA: &vpav1.VerticalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myapp-web",
					Namespace: "default",
					Labels: map[string]string{
						"app":                          "myapp-web",
						"app.kubernetes.io/component":  "tsuru-app",
						"app.kubernetes.io/managed-by": "tsuru",
						"app.kubernetes.io/name":       "myapp",
						"app.kubernetes.io/version":    "v1",
						"app.kubernetes.io/instance":   "myapp-web",
						"tsuru.io/app-name":            "myapp",
						"tsuru.io/app-platform":        "python",
						"tsuru.io/app-team":            "",
						"tsuru.io/app-pool":            "test-default",
						"tsuru.io/app-process":         "web",
						"tsuru.io/app-version":         "1",
						"tsuru.io/builder":             "",
						"tsuru.io/is-build":            "false",
						"tsuru.io/is-deploy":           "false",
						"tsuru.io/is-service":          "true",
						"tsuru.io/is-stopped":          "false",
						"tsuru.io/is-tsuru":            "true",
						"tsuru.io/provisioner":         "kubernetes",
						"version":                      "v1",
					},
				},
				Spec: vpav1.VerticalPodAutoscalerSpec{
					TargetRef: &autoscalingv1.CrossVersionObjectReference{
						APIVersion: appsv1.SchemeGroupVersion.String(),
						Kind:       "Deployment",
						Name:       "myapp-web",
					},
					UpdatePolicy: &vpav1.PodUpdatePolicy{
						UpdateMode: &vpaUpdateOff,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		c.Log(tt.name)
		if tt.scenario != nil {
			tt.scenario()
		}
		err := ensureVPAIfEnabled(context.TODO(), s.clusterClient, a, "web")
		c.Assert(err, check.IsNil)
		ns, err := s.client.AppNamespace(context.TODO(), a)
		c.Assert(err, check.IsNil)
		vpa, err := s.client.VPAClientset.AutoscalingV1().VerticalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		if tt.expectedVPA == nil {
			c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
		} else {
			c.Assert(vpa, check.DeepEquals, tt.expectedVPA)
		}
	}
}

func (s *S) TestGetVerticalAutoScaleRecommendations(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)

	rec, err := s.p.GetVerticalAutoScaleRecommendations(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(rec, check.IsNil)

	vpa := &vpav1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-web",
			Namespace: "default",
			Labels: map[string]string{
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/app-process": "web",
			},
		},
		Status: vpav1.VerticalPodAutoscalerStatus{
			Recommendation: &vpav1.RecommendedPodResources{
				ContainerRecommendations: []vpav1.RecommendedContainerResources{
					{
						ContainerName: "myapp-web",
						Target: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("99Mi"),
						},
						UncappedTarget: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("101m"),
							corev1.ResourceMemory: resource.MustParse("98Mi"),
						},
						LowerBound: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("102m"),
							corev1.ResourceMemory: resource.MustParse("97Mi"),
						},
						UpperBound: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("103m"),
							corev1.ResourceMemory: resource.MustParse("96Mi"),
						},
					},
				},
			},
		},
	}
	_, err = s.client.VPAClientset.AutoscalingV1().VerticalPodAutoscalers(ns).Create(context.TODO(), vpa, metav1.CreateOptions{})
	c.Assert(err, check.IsNil)

	rec, err = s.p.GetVerticalAutoScaleRecommendations(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(rec, check.IsNil)

	vpaCRD := &v1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "verticalpodautoscalers.autoscaling.k8s.io"},
	}
	_, err = s.client.ApiextensionsV1beta1().CustomResourceDefinitions().Create(context.TODO(), vpaCRD, metav1.CreateOptions{})
	c.Assert(err, check.IsNil)

	rec, err = s.p.GetVerticalAutoScaleRecommendations(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(rec, check.DeepEquals, []provision.RecommendedResources{
		{
			Process: "web",
			Recommendations: []provision.RecommendedProcessResources{
				{Type: "target", CPU: "100m", Memory: "99Mi"},
				{Type: "uncappedTarget", CPU: "101m", Memory: "98Mi"},
				{Type: "lowerBound", CPU: "102m", Memory: "97Mi"},
				{Type: "upperBound", CPU: "103m", Memory: "96Mi"},
			},
		},
	})
}
