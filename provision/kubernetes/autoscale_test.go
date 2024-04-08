// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"sort"
	"strconv"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/kr/pretty"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
)

func toInt32Ptr(i int32) *int32 {
	return &i
}

func testHPAWithTarget(tg autoscalingv2.MetricTarget) *autoscalingv2.HorizontalPodAutoscaler {
	policyMin := autoscalingv2.MinChangePolicySelect
	return &autoscalingv2.HorizontalPodAutoscaler{
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
				"tsuru.io/is-build":            "false",
				"tsuru.io/is-service":          "true",
				"tsuru.io/is-stopped":          "false",
				"tsuru.io/is-tsuru":            "true",
				"version":                      "v1",
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: toInt32Ptr(1),
			MaxReplicas: int32(2),
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "myapp-web",
			},
			Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
				ScaleDown: &autoscalingv2.HPAScalingRules{
					SelectPolicy: &policyMin,
					Policies: []autoscalingv2.HPAScalingPolicy{
						{
							Type:          autoscalingv2.PercentScalingPolicy,
							Value:         10,
							PeriodSeconds: 60,
						},
						{
							Type:          autoscalingv2.PodsScalingPolicy,
							Value:         3,
							PeriodSeconds: 60,
						},
					},
				},
			},
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name:   "cpu",
						Target: tg,
					},
				},
			},
		},
	}
}

func testKEDAScaledObject(cpuTrigger *kedav1alpha1.ScaleTriggers, scheduleSpecs []provision.AutoScaleSchedule) *kedav1alpha1.ScaledObject {
	triggers := []kedav1alpha1.ScaleTriggers{}

	if cpuTrigger != nil {
		triggers = append(triggers, *cpuTrigger)
	}

	for _, schedule := range scheduleSpecs {
		scheduleTrigger := kedav1alpha1.ScaleTriggers{
			Type: "cron",
			Metadata: map[string]string{
				"desiredReplicas": strconv.Itoa(schedule.MinReplicas),
				"start":           schedule.Start,
				"end":             schedule.End,
				"timezone":        "UTC",
			},
		}
		triggers = append(triggers, scheduleTrigger)
	}
	policyMin := autoscalingv2.MinChangePolicySelect

	return &kedav1alpha1.ScaledObject{
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
				"tsuru.io/is-build":            "false",
				"tsuru.io/is-service":          "true",
				"tsuru.io/is-stopped":          "false",
				"tsuru.io/is-tsuru":            "true",
				"version":                      "v1",
			},
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			MinReplicaCount: toInt32Ptr(1),
			MaxReplicaCount: toInt32Ptr(2),
			ScaleTargetRef: &kedav1alpha1.ScaleTarget{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "myapp-web",
			},
			Triggers: triggers,
			Advanced: &kedav1alpha1.AdvancedConfig{
				HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleDown: &autoscalingv2.HPAScalingRules{
							SelectPolicy: &policyMin,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          autoscalingv2.PercentScalingPolicy,
									Value:         10,
									PeriodSeconds: 60,
								},
								{
									Type:          autoscalingv2.PodsScalingPolicy,
									Value:         3,
									PeriodSeconds: 60,
								},
							},
						},
					},
				},
			},
		},
	}
}

func testKEDAHPA(scaledObjectName string) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "keda-hpa" + scaledObjectName,
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
				"tsuru.io/is-build":            "false",
				"tsuru.io/is-service":          "true",
				"tsuru.io/is-stopped":          "false",
				"tsuru.io/is-tsuru":            "true",
				"version":                      "v1",
				"scaledobject.keda.sh/name":    scaledObjectName,
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{},
	}
}

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

	cpu := resource.MustParse("500m")
	tests := []struct {
		scenario       func()
		expectedTarget autoscalingv2.MetricTarget
	}{
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "500m",
				})
				c.Assert(err, check.IsNil)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:         autoscalingv2.AverageValueMetricType,
				AverageValue: &cpu,
			},
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50%",
				})
				c.Assert(err, check.IsNil)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:         autoscalingv2.AverageValueMetricType,
				AverageValue: &cpu,
			},
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50",
				})
				c.Assert(err, check.IsNil)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:         autoscalingv2.AverageValueMetricType,
				AverageValue: &cpu,
			},
		},
		{
			scenario: func() {
				a.MilliCPU = 700
				defer func() { a.MilliCPU = 0 }()
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "500m",
				})
				c.Assert(err, check.IsNil)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: toInt32Ptr(50),
			},
		},
		{
			scenario: func() {
				a.MilliCPU = 700
				defer func() { a.MilliCPU = 0 }()
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50%",
				})
				c.Assert(err, check.IsNil)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: toInt32Ptr(50),
			},
		},
		{
			scenario: func() {
				a.MilliCPU = 700
				defer func() { a.MilliCPU = 0 }()
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50",
				})
				c.Assert(err, check.IsNil)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: toInt32Ptr(50),
			},
		},
	}
	for _, tt := range tests {
		tt.scenario()

		ns, err := s.client.AppNamespace(context.TODO(), a)
		c.Assert(err, check.IsNil)
		hpa, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		c.Assert(err, check.IsNil)
		expected := testHPAWithTarget(tt.expectedTarget)
		c.Assert(hpa, check.DeepEquals, expected, check.Commentf("diff: %v", pretty.Diff(hpa, expected)))
	}
}

func (s *S) TestProvisionerSetKEDAAutoScale(c *check.C) {
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

	schedulesList := []provision.AutoScaleSchedule{
		{
			MinReplicas: 2,
			Start:       "0 6 * * *",
			End:         "0 18 * * *",
			Timezone:    "UTC",
		},
		{
			MinReplicas: 1,
			Start:       "0 18 * * *",
			End:         "0 0 * * *",
			Timezone:    "UTC",
		},
		{
			MinReplicas: 2,
			Start:       "0 0 * * *",
			End:         "0 6 * * *",
			Timezone:    "UTC",
		},
	}

	tests := []struct {
		scenario      func()
		cpuTrigger    *kedav1alpha1.ScaleTriggers
		scheduleSpecs []provision.AutoScaleSchedule
	}{
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "500m",
					Schedules:  schedulesList[:1],
				})
				c.Assert(err, check.IsNil)
			},
			cpuTrigger: &kedav1alpha1.ScaleTriggers{
				Type:       "cpu",
				MetricType: autoscalingv2.AverageValueMetricType,
				Metadata: map[string]string{
					"value": strconv.Itoa(500),
				},
			},
			scheduleSpecs: schedulesList[:1],
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50%",
					Schedules:  schedulesList[:2],
				})
				c.Assert(err, check.IsNil)
			},
			cpuTrigger: &kedav1alpha1.ScaleTriggers{
				Type:       "cpu",
				MetricType: autoscalingv2.AverageValueMetricType,
				Metadata: map[string]string{
					"value": strconv.Itoa(500),
				},
			},
			scheduleSpecs: schedulesList[:2],
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50",
					Schedules:  schedulesList[:3],
				})
				c.Assert(err, check.IsNil)
			},
			cpuTrigger: &kedav1alpha1.ScaleTriggers{
				Type:       "cpu",
				MetricType: autoscalingv2.AverageValueMetricType,
				Metadata: map[string]string{
					"value": strconv.Itoa(500),
				},
			},
			scheduleSpecs: schedulesList[:3],
		},
		{
			scenario: func() {
				a.MilliCPU = 700
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "500m",
					Schedules:  schedulesList[:3],
				})
				c.Assert(err, check.IsNil)
			},
			cpuTrigger: &kedav1alpha1.ScaleTriggers{
				Type:       "cpu",
				MetricType: autoscalingv2.UtilizationMetricType,
				Metadata: map[string]string{
					"value": strconv.Itoa(50),
				},
			},
			scheduleSpecs: schedulesList[:3],
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
					MinUnits:  1,
					MaxUnits:  2,
					Schedules: schedulesList[:1],
				})
				c.Assert(err, check.IsNil)
			},
			cpuTrigger:    nil,
			scheduleSpecs: schedulesList[:1],
		},
	}
	for _, tt := range tests {
		tt.scenario()

		ns, err := s.client.AppNamespace(context.TODO(), a)
		c.Assert(err, check.IsNil)
		scaledObject, err := s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		c.Assert(err, check.IsNil)
		expected := testKEDAScaledObject(tt.cpuTrigger, tt.scheduleSpecs)
		c.Assert(scaledObject, check.DeepEquals, expected, check.Commentf("diff: %v", pretty.Diff(scaledObject, expected)))
	}
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

		hpas, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(context.TODO(), metav1.ListOptions{})
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

func (s *S) TestProvisionerSetKEDAAutoScaleMultipleVersions(c *check.C) {
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
		Schedules: []provision.AutoScaleSchedule{
			{
				MinReplicas: 2,
				Start:       "0 6 * * *",
				End:         "0 18 * * *",
				Timezone:    "UTC",
			},
		},
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

		hpas, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(context.TODO(), metav1.ListOptions{})
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
		MinUnits:   5,
		MaxUnits:   20,
		AverageCPU: "500m",
	})
	c.Assert(err, check.IsNil)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	existingPDB, err := s.client.PolicyV1().PodDisruptionBudgets(ns).Get(context.TODO(), pdbNameForApp(a, "web"), metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	pdb_expected := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbNameForApp(a, "web"),
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/app-process": "web",
				"tsuru.io/app-team":    "",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &intstr.IntOrString{Type: 1, StrVal: "10%"},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"tsuru.io/app-name":    "myapp",
					"tsuru.io/app-process": "web",
					"tsuru.io/is-routable": "true",
				},
			},
		},
	}
	c.Assert(existingPDB, check.DeepEquals, pdb_expected)
	err = s.p.RemoveAutoScale(context.TODO(), a, "web")
	c.Assert(err, check.IsNil)
	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(k8sErrors.IsNotFound(err), check.Equals, true)
	existingPDB, err = s.client.PolicyV1().PodDisruptionBudgets(ns).Get(context.TODO(), pdbNameForApp(a, "web"), metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	pdb_expected = &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbNameForApp(a, "web"),
			Namespace: ns,
			Labels: map[string]string{
				"tsuru.io/is-tsuru":    "true",
				"tsuru.io/app-name":    "myapp",
				"tsuru.io/app-team":    "",
				"tsuru.io/app-process": "web",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: intOrStringPtr(intstr.FromString("10%")),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"tsuru.io/app-name":    "myapp",
					"tsuru.io/app-process": "web",
					"tsuru.io/is-routable": "true",
				},
			},
		},
	}
	c.Assert(existingPDB, check.DeepEquals, pdb_expected)
}

func (s *S) TestProvisionerRemoveKEDAAutoScale(c *check.C) {
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
		MinUnits:   5,
		MaxUnits:   20,
		AverageCPU: "500m",
		Schedules: []provision.AutoScaleSchedule{
			{
				MinReplicas: 2,
				Start:       "0 6 * * *",
				End:         "0 18 * * *",
				Timezone:    "UTC",
			},
		},
	})
	c.Assert(err, check.IsNil)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	_, err = s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)

	err = s.p.RemoveAutoScale(context.TODO(), a, "web")
	c.Assert(err, check.IsNil)
	_, err = s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
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

func (s *S) TestProvisionerGetKEDAAutoScale(c *check.C) {
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
		Schedules: []provision.AutoScaleSchedule{
			{
				MinReplicas: 2,
				Start:       "0 6 * * *",
				End:         "0 18 * * *",
				Timezone:    "UTC",
			},
		},
	})
	c.Assert(err, check.IsNil)

	err = s.p.SetAutoScale(context.TODO(), a, provision.AutoScaleSpec{
		MinUnits:   2,
		MaxUnits:   4,
		AverageCPU: "200m",
		Process:    "worker",
		Schedules: []provision.AutoScaleSchedule{
			{
				MinReplicas: 4,
				Start:       "0 12 * * *",
				End:         "0 15 * * *",
				Timezone:    "UTC",
			},
		},
	})
	c.Assert(err, check.IsNil)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)

	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), testKEDAHPA("myapp-web"), metav1.CreateOptions{})
	c.Assert(err, check.IsNil)

	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), testKEDAHPA("myapp-worker"), metav1.CreateOptions{})
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
			Schedules: []provision.AutoScaleSchedule{
				{
					MinReplicas: 2,
					Start:       "0 6 * * *",
					End:         "0 18 * * *",
					Timezone:    "UTC",
				},
			},
		},
		{
			MinUnits:   2,
			MaxUnits:   4,
			AverageCPU: "200m",
			Version:    1,
			Process:    "worker",
			Schedules: []provision.AutoScaleSchedule{
				{
					MinReplicas: 4,
					Start:       "0 12 * * *",
					End:         "0 15 * * *",
					Timezone:    "UTC",
				},
			},
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
				vpaCRD := &extensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "verticalpodautoscalers.autoscaling.k8s.io"},
				}
				_, err := s.client.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), vpaCRD, metav1.CreateOptions{})
				c.Assert(err, check.IsNil)
			},
			expectedVPA: nil,
		},
		{
			name: "with crd and annotation",
			scenario: func() {
				a.Metadata.Update(appTypes.Metadata{
					Annotations: []appTypes.MetadataItem{
						{Name: AnnotationEnableVPA, Value: "true"},
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
						"tsuru.io/is-build":            "false",
						"tsuru.io/is-service":          "true",
						"tsuru.io/is-stopped":          "false",
						"tsuru.io/is-tsuru":            "true",
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

	vpaCRD := &extensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "verticalpodautoscalers.autoscaling.k8s.io"},
	}
	_, err = s.client.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), vpaCRD, metav1.CreateOptions{})
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

func (s *S) TestEnsureHPA(c *check.C) {
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

	cpu := resource.MustParse("80000m")
	_ = cpu.String()
	initialHPA := testHPAWithTarget(autoscalingv2.MetricTarget{
		Type:         autoscalingv2.AverageValueMetricType,
		AverageValue: &cpu,
	})

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), initialHPA, metav1.CreateOptions{})
	c.Assert(err, check.IsNil)

	err = ensureHPA(context.TODO(), s.clusterClient, a, "web")
	c.Assert(err, check.IsNil)

	newHPA, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(newHPA, check.DeepEquals, initialHPA, check.Commentf("diff: %v", pretty.Diff(newHPA, initialHPA)))
}

func (s *S) TestEnsureHPAWithCPUPlan(c *check.C) {
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

	a.MilliCPU = 2000

	cpu := resource.MustParse("800m")
	_ = cpu.String()
	initialHPA := testHPAWithTarget(autoscalingv2.MetricTarget{
		Type:         autoscalingv2.AverageValueMetricType,
		AverageValue: &cpu,
	})

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), initialHPA, metav1.CreateOptions{})
	c.Assert(err, check.IsNil)

	err = ensureHPA(context.TODO(), s.clusterClient, a, "web")
	c.Assert(err, check.IsNil)

	expectedHPA := testHPAWithTarget(autoscalingv2.MetricTarget{
		Type:               autoscalingv2.UtilizationMetricType,
		AverageUtilization: toInt32Ptr(80),
	})

	newHPA, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(newHPA, check.DeepEquals, expectedHPA, check.Commentf("diff: %v", pretty.Diff(newHPA, expectedHPA)))

	err = ensureHPA(context.TODO(), s.clusterClient, a, "web")
	c.Assert(err, check.IsNil)

	newHPA, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(newHPA, check.DeepEquals, expectedHPA, check.Commentf("diff: %v", pretty.Diff(newHPA, expectedHPA)))
}

func (s *S) TestEnsureHPAWithCPUPlanInvalid(c *check.C) {
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

	a.MilliCPU = 2000

	cpu := resource.MustParse("80000m")
	_ = cpu.String()
	initialHPA := testHPAWithTarget(autoscalingv2.MetricTarget{
		Type:         autoscalingv2.AverageValueMetricType,
		AverageValue: &cpu,
	})

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), initialHPA, metav1.CreateOptions{})
	c.Assert(err, check.IsNil)

	err = ensureHPA(context.TODO(), s.clusterClient, a, "web")
	c.Assert(err, check.ErrorMatches, `autoscale cpu value cannot be greater than 95%`)
}
