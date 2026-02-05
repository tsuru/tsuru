// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"sort"
	"strconv"
	"testing"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/require"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/servicecommon"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
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
					SelectPolicy:               &policyMin,
					StabilizationWindowSeconds: toInt32Ptr(300),
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

func testKEDAScaledObject(trigger *kedav1alpha1.ScaleTriggers, scheduleSpecs []provTypes.AutoScaleSchedule, prometheusSpecs []provTypes.AutoScalePrometheus, ns string) *kedav1alpha1.ScaledObject {
	triggers := []kedav1alpha1.ScaleTriggers{}

	if trigger != nil {
		triggers = append(triggers, *trigger)
	}

	for _, schedule := range scheduleSpecs {
		scheduleTrigger := kedav1alpha1.ScaleTriggers{
			Type: "cron",
			Metadata: map[string]string{
				"scheduleName":    schedule.Name,
				"desiredReplicas": strconv.Itoa(schedule.MinReplicas),
				"start":           schedule.Start,
				"end":             schedule.End,
				"timezone":        "UTC",
			},
		}
		triggers = append(triggers, scheduleTrigger)
	}

	for _, prometheus := range prometheusSpecs {
		prometheusTrigger, err := buildPrometheusTrigger(ns, prometheus)
		if err != nil {
			return nil
		}

		triggers = append(triggers, *prometheusTrigger)
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
							StabilizationWindowSeconds: toInt32Ptr(300),
							SelectPolicy:               &policyMin,
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
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	cpu := resource.MustParse("500m")
	tests := []struct {
		scenario       func()
		expectedTarget autoscalingv2.MetricTarget
	}{
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "500m",
				})
				require.NoError(s.t, err)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:         autoscalingv2.AverageValueMetricType,
				AverageValue: &cpu,
			},
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50%",
				})
				require.NoError(s.t, err)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:         autoscalingv2.AverageValueMetricType,
				AverageValue: &cpu,
			},
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50",
				})
				require.NoError(s.t, err)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:         autoscalingv2.AverageValueMetricType,
				AverageValue: &cpu,
			},
		},
		{
			scenario: func() {
				a.Plan.CPUMilli = 700
				defer func() { a.Plan.CPUMilli = 0 }()
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "500m",
				})
				require.NoError(s.t, err)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: toInt32Ptr(50),
			},
		},
		{
			scenario: func() {
				a.Plan.CPUMilli = 700
				defer func() { a.Plan.CPUMilli = 0 }()
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50%",
				})
				require.NoError(s.t, err)
			},
			expectedTarget: autoscalingv2.MetricTarget{
				Type:               autoscalingv2.UtilizationMetricType,
				AverageUtilization: toInt32Ptr(50),
			},
		},
		{
			scenario: func() {
				a.Plan.CPUMilli = 700
				defer func() { a.Plan.CPUMilli = 0 }()
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50",
				})
				require.NoError(s.t, err)
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
		require.NoError(s.t, err)
		hpa, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		require.NoError(s.t, err)
		expected := testHPAWithTarget(tt.expectedTarget)
		require.EqualValues(s.t, expected, hpa, "diff", pretty.Diff(hpa, expected))
	}
}

func (s *S) TestProvisionerSetScheduleKEDAAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	schedulesList := []provTypes.AutoScaleSchedule{
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
		scheduleSpecs []provTypes.AutoScaleSchedule
	}{
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "500m",
					Schedules:  schedulesList[:1],
				})
				require.NoError(s.t, err)
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
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50%",
					Schedules:  schedulesList[:2],
				})
				require.NoError(s.t, err)
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
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50",
					Schedules:  schedulesList[:3],
				})
				require.NoError(s.t, err)
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
				a.Plan.CPUMilli = 700
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "500m",
					Schedules:  schedulesList[:3],
				})
				require.NoError(s.t, err)
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
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:  1,
					MaxUnits:  2,
					Schedules: schedulesList[:1],
				})
				require.NoError(s.t, err)
			},
			cpuTrigger:    nil,
			scheduleSpecs: schedulesList[:1],
		},
	}
	for _, tt := range tests {
		tt.scenario()

		ns, err := s.client.AppNamespace(context.TODO(), a)
		require.NoError(s.t, err)
		scaledObject, err := s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		require.NoError(s.t, err)
		expected := testKEDAScaledObject(tt.cpuTrigger, tt.scheduleSpecs, []provTypes.AutoScalePrometheus{}, "default")
		require.EqualValues(s.t, expected, scaledObject, "diff", pretty.Diff(scaledObject, expected))
	}
}

func (s *S) TestProvisionerSetPrometheusKEDAAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	config.Set("kubernetes:keda:prometheus-address-template", "http://prometheus-address-test.{{.namespace}}")
	defer config.Unset("kubernetes:keda:prometheus-address-template")

	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	prometheusList := []provTypes.AutoScalePrometheus{
		{
			Name:      "prometheus_metric_1",
			Query:     "some_query_1",
			Threshold: 10.0,
		},
		{
			Name:                "prometheus_metric_2",
			Query:               "some_query_2",
			Threshold:           10.0,
			ActivationThreshold: 2.0,
		},
		{
			Name:              "prometheus_metric_3",
			Query:             "some_query_3",
			PrometheusAddress: "test.prometheus.address.exemple",
			Threshold:         30.0,
		},
		{
			Name:              "prometheus_metric_gcp",
			Query:             "some_gcp_query",
			PrometheusAddress: "https://monitoring.googleapis.com/v1/projects/my-gcp-project/location/global/prometheus/api/v1/query",
			Threshold:         1.0,
		},
	}

	tests := []struct {
		scenario        func()
		trigger         *kedav1alpha1.ScaleTriggers
		prometheusSpecs []provTypes.AutoScalePrometheus
	}{
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "500m",
					Prometheus: prometheusList[:1],
				})
				require.NoError(s.t, err)
			},
			trigger: &kedav1alpha1.ScaleTriggers{
				Type:       "cpu",
				MetricType: autoscalingv2.AverageValueMetricType,
				Metadata: map[string]string{
					"value": strconv.Itoa(500),
				},
			},
			prometheusSpecs: prometheusList[:1],
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50%",
					Prometheus: prometheusList[:2],
				})
				require.NoError(s.t, err)
			},
			trigger: &kedav1alpha1.ScaleTriggers{
				Type:       "cpu",
				MetricType: autoscalingv2.AverageValueMetricType,
				Metadata: map[string]string{
					"value": strconv.Itoa(500),
				},
			},
			prometheusSpecs: prometheusList[:2],
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "50",
					Prometheus: prometheusList[:3],
				})
				require.NoError(s.t, err)
			},
			trigger: &kedav1alpha1.ScaleTriggers{
				Type:       "cpu",
				MetricType: autoscalingv2.AverageValueMetricType,
				Metadata: map[string]string{
					"value": strconv.Itoa(500),
				},
			},
			prometheusSpecs: prometheusList[:3],
		},
		{
			scenario: func() {
				a.Plan.CPUMilli = 700
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					AverageCPU: "500m",
					Prometheus: prometheusList[:3],
				})
				require.NoError(s.t, err)
			},
			trigger: &kedav1alpha1.ScaleTriggers{
				Type:       "cpu",
				MetricType: autoscalingv2.UtilizationMetricType,
				Metadata: map[string]string{
					"value": strconv.Itoa(50),
				},
			},
			prometheusSpecs: prometheusList[:3],
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					Prometheus: prometheusList[:1],
				})
				require.NoError(s.t, err)
			},
			trigger:         nil,
			prometheusSpecs: prometheusList[:1],
		},
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits: 1,
					MaxUnits: 2,
					Prometheus: []provTypes.AutoScalePrometheus{
						prometheusList[3],
					},
				})
				require.NoError(s.t, err)
			},
			trigger: &kedav1alpha1.ScaleTriggers{
				Type: "prometheus",
				AuthenticationRef: &kedav1alpha1.AuthenticationRef{
					Name: "gcp-credentials",
					Kind: "ClusterTriggerAuthentication",
				},
				Metadata: map[string]string{
					"serverAddress":        "https://monitoring.googleapis.com/v1/projects/my-gcp-project/location/global/prometheus/api/v1/query",
					"query":                "some_gcp_query",
					"threshold":            "1",
					"activationThreshold":  "0",
					"prometheusMetricName": "prometheus_metric_gcp",
				},
			},
		},
	}
	for _, tt := range tests {
		tt.scenario()

		ns, err := s.client.AppNamespace(context.TODO(), a)
		require.NoError(s.t, err)
		scaledObject, err := s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		require.NoError(s.t, err)
		expected := testKEDAScaledObject(tt.trigger, []provTypes.AutoScaleSchedule{}, tt.prometheusSpecs, "default")
		require.EqualValues(s.t, expected, scaledObject, "diff", pretty.Diff(scaledObject, expected))
	}
}

func (s *S) TestProvisionerSetPrometheusKEDAAutoScaleWithoutTemplateConfig(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	prometheusList := []provTypes.AutoScalePrometheus{
		{
			Name:              "prometheus_metric_1",
			Query:             "some_query_1",
			PrometheusAddress: "test.prometheus.address.exemple",
			Threshold:         10.0,
		},
		{
			Name:      "prometheus_metric_2",
			Query:     "some_query_2",
			Threshold: 10.0,
		},
	}

	tests := []struct {
		scenario        func()
		prometheusSpecs []provTypes.AutoScalePrometheus
		assertion       func(error, *kedav1alpha1.ScaledObject)
	}{
		{
			scenario: func() {
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					Prometheus: prometheusList[:1],
				})
				require.NoError(s.t, err)
			},
			prometheusSpecs: prometheusList[:1],
			assertion: func(err error, scaledObject *kedav1alpha1.ScaledObject) {
				require.NoError(s.t, err)
				expected := testKEDAScaledObject(nil, []provTypes.AutoScaleSchedule{}, prometheusList[:1], "default")
				require.EqualValues(s.t, expected, scaledObject, "diff", pretty.Diff(scaledObject, expected))
			},
		},
		{
			scenario: func() {
				config.Unset("kubernetes:keda:prometheus-address-template")
				err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
					MinUnits:   1,
					MaxUnits:   2,
					Prometheus: prometheusList[1:],
				})
				expectedError := config.ErrKeyNotFound{Key: "kubernetes:keda:prometheus-address-template"}
				require.ErrorContains(s.t, err, expectedError.Error())
			},
			prometheusSpecs: prometheusList[:1],
			assertion: func(err error, scaledObject *kedav1alpha1.ScaledObject) {
				require.True(s.t, k8sErrors.IsNotFound(err), "expected NotFound error, got: %v", err)
			},
		},
	}
	for _, tt := range tests {
		tt.scenario()

		ns, err := s.client.AppNamespace(context.TODO(), a)
		require.NoError(s.t, err)
		scaledObject, err := s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		tt.assertion(err, scaledObject)
		s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Delete(context.TODO(), "myapp-web", metav1.DeleteOptions{})
	}
}

func (s *S) TestProvisionerSetAutoScaleMultipleVersions(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	versions := make([]appTypes.AppVersion, 4)
	for i := range versions {
		versions[i] = newSuccessfulVersion(c, a, map[string][]string{
			"web": {"python", "myapp.py"},
		})
	}

	err := s.p.AddUnits(context.TODO(), a, 1, "web", versions[0], nil)
	require.NoError(s.t, err)
	wait()
	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
	})
	require.NoError(s.t, err)

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
				require.NoError(s.t, err)
				wait()
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.AddUnits(context.TODO(), a, 1, "web", versions[2], nil)
				require.NoError(s.t, err)
				wait()
				err = s.p.ToggleRoutable(context.TODO(), a, versions[2], true)
				require.NoError(s.t, err)
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.AddUnits(context.TODO(), a, 1, "web", versions[3], nil)
				require.NoError(s.t, err)
				wait()
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[0], &bytes.Buffer{})
				require.NoError(s.t, err)
				wait()
			},
			expectedDeployment: "myapp-web-v3",
			expectedVersion:    3,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[2], &bytes.Buffer{})
				require.NoError(s.t, err)
				wait()
			},
			expectedDeployment: "myapp-web-v2",
			expectedVersion:    2,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[1], &bytes.Buffer{})
				require.NoError(s.t, err)
				wait()
			},
			expectedDeployment: "myapp-web-v4",
			expectedVersion:    4,
		},
	}

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	for i, tt := range tests {
		c.Logf("test %d", i)
		tt.scenario()

		hpas, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(context.TODO(), metav1.ListOptions{})
		require.NoError(s.t, err)
		for _, hpa := range hpas.Items {
			dep, err := s.client.AppsV1().Deployments(ns).Get(context.TODO(), hpa.Spec.ScaleTargetRef.Name, metav1.GetOptions{})
			require.NoError(s.t, err)
			if dep.Spec.Replicas != nil && *dep.Spec.Replicas > 0 {
				require.Equal(s.t, tt.expectedDeployment, hpa.Spec.ScaleTargetRef.Name)
				require.Equal(s.t, strconv.Itoa(tt.expectedVersion), hpa.Labels["tsuru.io/app-version"])
			}
		}
	}
}

func (s *S) TestProvisionerSetKEDAAutoScaleMultipleVersions(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	versions := make([]appTypes.AppVersion, 4)
	for i := range versions {
		versions[i] = newSuccessfulVersion(c, a, map[string][]string{
			"web": {"python", "myapp.py"},
		})
	}
	err := s.p.AddUnits(context.TODO(), a, 1, "web", versions[0], nil)
	require.NoError(s.t, err)
	wait()
	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
		Schedules: []provTypes.AutoScaleSchedule{
			{
				MinReplicas: 2,
				Start:       "0 6 * * *",
				End:         "0 18 * * *",
				Timezone:    "UTC",
			},
		},
		Prometheus: []provTypes.AutoScalePrometheus{
			{
				Name:              "prometheus_metric",
				Query:             "sum(some_metric{app='app_test'})",
				PrometheusAddress: "test.prometheus.address.exemple",
				Threshold:         10.0,
			},
		},
	})
	require.NoError(s.t, err)

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
				require.NoError(s.t, err)
				wait()
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.AddUnits(context.TODO(), a, 1, "web", versions[2], nil)
				require.NoError(s.t, err)
				wait()
				err = s.p.ToggleRoutable(context.TODO(), a, versions[2], true)
				require.NoError(s.t, err)
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.AddUnits(context.TODO(), a, 1, "web", versions[3], nil)
				require.NoError(s.t, err)
				wait()
			},
			expectedDeployment: "myapp-web",
			expectedVersion:    1,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[0], &bytes.Buffer{})
				require.NoError(s.t, err)
				wait()
			},
			expectedDeployment: "myapp-web-v3",
			expectedVersion:    3,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[2], &bytes.Buffer{})
				require.NoError(s.t, err)
				wait()
			},
			expectedDeployment: "myapp-web-v2",
			expectedVersion:    2,
		},
		{
			scenario: func() {
				err = s.p.Stop(context.TODO(), a, "web", versions[1], &bytes.Buffer{})
				require.NoError(s.t, err)
				wait()
			},
			expectedDeployment: "myapp-web-v4",
			expectedVersion:    4,
		},
	}

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	for i, tt := range tests {
		c.Logf("test %d", i)
		tt.scenario()

		hpas, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(context.TODO(), metav1.ListOptions{})
		require.NoError(s.t, err)
		for _, hpa := range hpas.Items {
			dep, err := s.client.AppsV1().Deployments(ns).Get(context.TODO(), hpa.Spec.ScaleTargetRef.Name, metav1.GetOptions{})
			require.NoError(s.t, err)
			if dep.Spec.Replicas != nil && *dep.Spec.Replicas > 0 {
				require.Equal(s.t, tt.expectedDeployment, hpa.Spec.ScaleTargetRef.Name)
				require.Equal(s.t, strconv.Itoa(tt.expectedVersion), hpa.Labels["tsuru.io/app-version"])
			}
		}
	}
}

func (s *S) TestProvisionerRemoveAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		MinUnits:   5,
		MaxUnits:   20,
		AverageCPU: "500m",
	})
	require.NoError(s.t, err)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	existingPDB, err := s.client.PolicyV1().PodDisruptionBudgets(ns).Get(context.TODO(), pdbNameForApp(a, "web"), metav1.GetOptions{})
	require.NoError(s.t, err)
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
	require.EqualValues(s.t, pdb_expected, existingPDB)
	err = s.p.RemoveAutoScale(context.TODO(), a, "web")
	require.NoError(s.t, err)
	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
	existingPDB, err = s.client.PolicyV1().PodDisruptionBudgets(ns).Get(context.TODO(), pdbNameForApp(a, "web"), metav1.GetOptions{})
	require.NoError(s.t, err)
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
	require.EqualValues(s.t, pdb_expected, existingPDB)
}

func (s *S) TestProvisionerRemoveKEDAAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		MinUnits:   5,
		MaxUnits:   20,
		AverageCPU: "500m",
		Schedules: []provTypes.AutoScaleSchedule{
			{
				MinReplicas: 2,
				Start:       "0 6 * * *",
				End:         "0 18 * * *",
				Timezone:    "UTC",
			},
		},
		Prometheus: []provTypes.AutoScalePrometheus{
			{
				Name:              "prometheus_metric",
				Query:             "sum(some_metric{app='app_test'})",
				PrometheusAddress: "test.prometheus.address.exemple",
				Threshold:         10.0,
			},
		},
	})
	require.NoError(s.t, err)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)

	err = s.p.RemoveAutoScale(context.TODO(), a, "web")
	require.NoError(s.t, err)
	_, err = s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
}

func (s *S) TestProvisionerGetAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web":    {"python", "myapp.py"},
		"worker": {"python worker.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	err = s.p.AddUnits(context.TODO(), a, 1, "worker", version, nil)
	require.NoError(s.t, err)
	wait()

	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
		Process:    "web",
	})
	require.NoError(s.t, err)

	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		MinUnits:   2,
		MaxUnits:   4,
		AverageCPU: "200m",
		Process:    "worker",
	})
	require.NoError(s.t, err)

	scales, err := s.p.GetAutoScale(context.TODO(), a)
	require.NoError(s.t, err)
	sort.Slice(scales, func(i, j int) bool {
		return scales[i].Process < scales[j].Process
	})
	require.EqualValues(s.t, []provTypes.AutoScaleSpec{
		{
			MinUnits:   1,
			MaxUnits:   2,
			AverageCPU: "500m",
			Version:    1,
			Process:    "web",
			Behavior: provTypes.BehaviorAutoScaleSpec{
				ScaleDown: &provTypes.ScaleDownPolicy{
					StabilizationWindow:   toInt32Ptr(300),
					PercentagePolicyValue: toInt32Ptr(10),
					UnitsPolicyValue:      toInt32Ptr(3),
				},
			},
		},
		{
			MinUnits:   2,
			MaxUnits:   4,
			AverageCPU: "200m",
			Version:    1,
			Process:    "worker",
			Behavior: provTypes.BehaviorAutoScaleSpec{
				ScaleDown: &provTypes.ScaleDownPolicy{
					StabilizationWindow:   toInt32Ptr(300),
					PercentagePolicyValue: toInt32Ptr(10),
					UnitsPolicyValue:      toInt32Ptr(3),
				},
			},
		},
	}, scales)
}

func (s *S) TestProvisionerGetScheduleKEDAAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web":    {"python", "myapp.py"},
		"worker": {"python worker.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	err = s.p.AddUnits(context.TODO(), a, 1, "worker", version, nil)
	require.NoError(s.t, err)
	wait()

	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
		Process:    "web",
		Schedules: []provTypes.AutoScaleSchedule{
			{
				MinReplicas: 2,
				Start:       "0 6 * * *",
				End:         "0 18 * * *",
				Timezone:    "UTC",
			},
		},
	})
	require.NoError(s.t, err)

	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		MinUnits:   2,
		MaxUnits:   4,
		AverageCPU: "200m",
		Process:    "worker",
		Schedules: []provTypes.AutoScaleSchedule{
			{
				MinReplicas: 4,
				Start:       "0 12 * * *",
				End:         "0 15 * * *",
				Timezone:    "UTC",
			},
		},
		Behavior: provTypes.BehaviorAutoScaleSpec{
			ScaleDown: &provTypes.ScaleDownPolicy{
				StabilizationWindow:   toInt32Ptr(60),
				PercentagePolicyValue: toInt32Ptr(20),
				UnitsPolicyValue:      toInt32Ptr(10),
			},
		},
	})
	require.NoError(s.t, err)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), testKEDAHPA("myapp-web"), metav1.CreateOptions{})
	require.NoError(s.t, err)

	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), testKEDAHPA("myapp-worker"), metav1.CreateOptions{})
	require.NoError(s.t, err)

	scales, err := s.p.GetAutoScale(context.TODO(), a)
	require.NoError(s.t, err)
	sort.Slice(scales, func(i, j int) bool {
		return scales[i].Process < scales[j].Process
	})
	require.EqualValues(s.t, []provTypes.AutoScaleSpec{
		{
			MinUnits:   1,
			MaxUnits:   2,
			AverageCPU: "500m",
			Version:    1,
			Process:    "web",
			Schedules: []provTypes.AutoScaleSchedule{
				{
					MinReplicas: 2,
					Start:       "0 6 * * *",
					End:         "0 18 * * *",
					Timezone:    "UTC",
				},
			},
			Behavior: provTypes.BehaviorAutoScaleSpec{
				ScaleDown: &provTypes.ScaleDownPolicy{
					StabilizationWindow:   toInt32Ptr(300),
					PercentagePolicyValue: toInt32Ptr(10),
					UnitsPolicyValue:      toInt32Ptr(3),
				},
			},
		},
		{
			MinUnits:   2,
			MaxUnits:   4,
			AverageCPU: "200m",
			Version:    1,
			Process:    "worker",
			Schedules: []provTypes.AutoScaleSchedule{
				{
					MinReplicas: 4,
					Start:       "0 12 * * *",
					End:         "0 15 * * *",
					Timezone:    "UTC",
				},
			},
			Behavior: provTypes.BehaviorAutoScaleSpec{
				ScaleDown: &provTypes.ScaleDownPolicy{
					StabilizationWindow:   toInt32Ptr(60),
					PercentagePolicyValue: toInt32Ptr(20),
					UnitsPolicyValue:      toInt32Ptr(10),
				},
			},
		},
	}, scales)
}

func (s *S) TestProvisionerGetPrometheusKEDAAutoScale(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	config.Set("kubernetes:keda:prometheus-address-template", "http://prometheus-address-test.{{.namespace}}")
	defer config.Unset("kubernetes:keda:prometheus-address-template")

	version := newSuccessfulVersion(c, a, map[string][]string{
		"web":    {"python", "myapp.py"},
		"worker": {"python worker.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()
	err = s.p.AddUnits(context.TODO(), a, 1, "worker", version, nil)
	require.NoError(s.t, err)
	wait()

	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		MinUnits:   1,
		MaxUnits:   2,
		AverageCPU: "500m",
		Process:    "web",
		Prometheus: []provTypes.AutoScalePrometheus{
			{
				Name:              "prometheus_metric_1",
				Query:             "some_query_1",
				PrometheusAddress: "test.prometheus.address.exemple",
				Threshold:         10.0,
			},
		},
		Behavior: provTypes.BehaviorAutoScaleSpec{
			ScaleDown: &provTypes.ScaleDownPolicy{
				PercentagePolicyValue: toInt32Ptr(21),
				UnitsPolicyValue:      toInt32Ptr(15),
			},
		},
	})
	require.NoError(s.t, err)

	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		MinUnits:   2,
		MaxUnits:   4,
		AverageCPU: "200m",
		Process:    "worker",
		Prometheus: []provTypes.AutoScalePrometheus{
			{
				Name:      "prometheus_metric_2",
				Query:     "some_query_2",
				Threshold: 20.0,
			},
			{
				Name:              "prometheus_metric_3",
				Query:             "some_query_3",
				PrometheusAddress: "test.prometheus.address3.exemple",
				Threshold:         30.0,
			},
		},
	})
	require.NoError(s.t, err)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), testKEDAHPA("myapp-web"), metav1.CreateOptions{})
	require.NoError(s.t, err)

	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), testKEDAHPA("myapp-worker"), metav1.CreateOptions{})
	require.NoError(s.t, err)

	scales, err := s.p.GetAutoScale(context.TODO(), a)
	require.NoError(s.t, err)
	sort.Slice(scales, func(i, j int) bool {
		return scales[i].Process < scales[j].Process
	})
	require.EqualValues(s.t, []provTypes.AutoScaleSpec{
		{
			MinUnits:   1,
			MaxUnits:   2,
			AverageCPU: "500m",
			Version:    1,
			Process:    "web",
			Prometheus: []provTypes.AutoScalePrometheus{
				{
					Name:              "prometheus_metric_1",
					Query:             "some_query_1",
					PrometheusAddress: "test.prometheus.address.exemple",
					Threshold:         10.0,
				},
			},
			Behavior: provTypes.BehaviorAutoScaleSpec{
				ScaleDown: &provTypes.ScaleDownPolicy{
					StabilizationWindow:   toInt32Ptr(300),
					PercentagePolicyValue: toInt32Ptr(21),
					UnitsPolicyValue:      toInt32Ptr(15),
				},
			},
		},
		{
			MinUnits:   2,
			MaxUnits:   4,
			AverageCPU: "200m",
			Version:    1,
			Process:    "worker",
			Prometheus: []provTypes.AutoScalePrometheus{
				{
					Name:              "prometheus_metric_2",
					Query:             "some_query_2",
					PrometheusAddress: "http://prometheus-address-test.default",
					Threshold:         20.0,
				},
				{
					Name:              "prometheus_metric_3",
					Query:             "some_query_3",
					PrometheusAddress: "test.prometheus.address3.exemple",
					Threshold:         30.0,
				},
			},
			Behavior: provTypes.BehaviorAutoScaleSpec{
				ScaleDown: &provTypes.ScaleDownPolicy{
					StabilizationWindow:   toInt32Ptr(300),
					PercentagePolicyValue: toInt32Ptr(10),
					UnitsPolicyValue:      toInt32Ptr(3),
				},
			},
		},
	}, scales)
}

func (s *S) TestProvisionerKEDAAutoScaleWhenAppStopAppStart(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	err = s.p.Stop(context.TODO(), a, "web", version, &bytes.Buffer{})
	require.NoError(s.t, err)

	autoScaleSpec := provTypes.AutoScaleSpec{
		MinUnits:   5,
		MaxUnits:   20,
		AverageCPU: "500m",
		Schedules: []provTypes.AutoScaleSchedule{
			{
				MinReplicas: 2,
				Start:       "0 6 * * *",
				End:         "0 18 * * *",
				Timezone:    "UTC",
			},
		},
		Prometheus: []provTypes.AutoScalePrometheus{
			{
				Name:              "prometheus_metric",
				Query:             "sum(some_metric{app='app_test'})",
				PrometheusAddress: "test.prometheus.address.exemple",
				Threshold:         10.0,
			},
		},
	}
	err = s.p.SetAutoScale(context.TODO(), a, autoScaleSpec)
	require.NoError(s.t, err)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	scaledObject, err := s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, map[string]string{AnnotationKEDAPausedReplicas: "0"}, scaledObject.GetAnnotations())

	err = s.p.Start(context.TODO(), a, "web", version, &bytes.Buffer{})
	require.NoError(s.t, err)

	err = s.p.SetAutoScale(context.TODO(), a, autoScaleSpec)
	require.NoError(s.t, err)

	scaledObject, err = s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, map[string]string(nil), scaledObject.GetAnnotations())
}

func (s *S) TestProvisionerKEDAAutoScaleWhenBevaher(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	err = s.p.Stop(context.TODO(), a, "web", version, &bytes.Buffer{})
	require.NoError(s.t, err)

	autoScaleSpec := provTypes.AutoScaleSpec{
		MinUnits:   5,
		MaxUnits:   20,
		AverageCPU: "500m",
		Schedules: []provTypes.AutoScaleSchedule{
			{
				MinReplicas: 2,
				Start:       "0 6 * * *",
				End:         "0 18 * * *",
				Timezone:    "UTC",
			},
		},
		Prometheus: []provTypes.AutoScalePrometheus{
			{
				Name:              "prometheus_metric",
				Query:             "sum(some_metric{app='app_test'})",
				PrometheusAddress: "test.prometheus.address.exemple",
				Threshold:         10.0,
			},
		},
		Behavior: provTypes.BehaviorAutoScaleSpec{
			ScaleDown: &provTypes.ScaleDownPolicy{
				StabilizationWindow:   toInt32Ptr(300),
				PercentagePolicyValue: toInt32Ptr(50),
				UnitsPolicyValue:      toInt32Ptr(2),
			},
		},
	}
	err = s.p.SetAutoScale(context.TODO(), a, autoScaleSpec)
	require.NoError(s.t, err)
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	scaledObject, err := s.client.KEDAClientForConfig.KedaV1alpha1().ScaledObjects(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	scaleDown := scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig.Behavior.ScaleDown
	require.Equal(s.t, int32(300), *scaleDown.StabilizationWindowSeconds)
	require.Equal(s.t, int32(50), scaleDown.Policies[0].Value)
	require.Equal(s.t, int32(2), scaleDown.Policies[1].Value)
}

func (s *S) TestEnsureVPAIfEnabled(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newCommittedVersion(c, a, map[string][]string{
		"web": {"cm1"},
	})
	err := s.p.AddUnits(context.Background(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
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
				require.NoError(s.t, err)
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
		require.NoError(s.t, err)
		ns, err := s.client.AppNamespace(context.TODO(), a)
		require.NoError(s.t, err)
		vpa, err := s.client.VPAClientset.AutoscalingV1().VerticalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
		if tt.expectedVPA == nil {
			require.True(s.t, k8sErrors.IsNotFound(err))
		} else {
			require.EqualValues(s.t, tt.expectedVPA, vpa)
		}
	}
}

func (s *S) TestGetVerticalAutoScaleRecommendations(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	rec, err := s.p.GetVerticalAutoScaleRecommendations(context.TODO(), a)
	require.NoError(s.t, err)
	require.Nil(s.t, rec)

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
	require.NoError(s.t, err)

	rec, err = s.p.GetVerticalAutoScaleRecommendations(context.TODO(), a)
	require.NoError(s.t, err)
	require.Nil(s.t, rec)

	vpaCRD := &extensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "verticalpodautoscalers.autoscaling.k8s.io"},
	}
	_, err = s.client.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), vpaCRD, metav1.CreateOptions{})
	require.NoError(s.t, err)

	rec, err = s.p.GetVerticalAutoScaleRecommendations(context.TODO(), a)
	require.NoError(s.t, err)
	require.EqualValues(s.t, []provTypes.RecommendedResources{
		{
			Process: "web",
			Recommendations: []provTypes.RecommendedProcessResources{
				{Type: "target", CPU: "100m", Memory: "99Mi"},
				{Type: "uncappedTarget", CPU: "101m", Memory: "98Mi"},
				{Type: "lowerBound", CPU: "102m", Memory: "97Mi"},
				{Type: "upperBound", CPU: "103m", Memory: "96Mi"},
			},
		},
	}, rec)
}

func (s *S) TestEnsureHPA(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	cpu := resource.MustParse("80000m")
	_ = cpu.String()
	initialHPA := testHPAWithTarget(autoscalingv2.MetricTarget{
		Type:         autoscalingv2.AverageValueMetricType,
		AverageValue: &cpu,
	})

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), initialHPA, metav1.CreateOptions{})
	require.NoError(s.t, err)

	err = ensureHPA(context.TODO(), s.clusterClient, a, "web")
	require.NoError(s.t, err)

	newHPA, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, initialHPA, newHPA, "diff", pretty.Diff(newHPA, initialHPA))
}

func (s *S) TestEnsureHPAWithCPUPlan(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	a.Plan.CPUMilli = 2000

	cpu := resource.MustParse("800m")
	_ = cpu.String()
	initialHPA := testHPAWithTarget(autoscalingv2.MetricTarget{
		Type:         autoscalingv2.AverageValueMetricType,
		AverageValue: &cpu,
	})

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), initialHPA, metav1.CreateOptions{})
	require.NoError(s.t, err)

	err = ensureHPA(context.TODO(), s.clusterClient, a, "web")
	require.NoError(s.t, err)

	expectedHPA := testHPAWithTarget(autoscalingv2.MetricTarget{
		Type:               autoscalingv2.UtilizationMetricType,
		AverageUtilization: toInt32Ptr(80),
	})

	newHPA, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedHPA, newHPA, "diff", pretty.Diff(newHPA, expectedHPA))

	err = ensureHPA(context.TODO(), s.clusterClient, a, "web")
	require.NoError(s.t, err)

	newHPA, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(context.TODO(), "myapp-web", metav1.GetOptions{})
	require.NoError(s.t, err)
	require.EqualValues(s.t, expectedHPA, newHPA, "diff", pretty.Diff(newHPA, expectedHPA))
}

func (s *S) TestEnsureHPAWithCPUPlanInvalid(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()
	version := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "myapp.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version, nil)
	require.NoError(s.t, err)
	wait()

	a.Plan.CPUMilli = 2000

	cpu := resource.MustParse("80000m")
	_ = cpu.String()
	initialHPA := testHPAWithTarget(autoscalingv2.MetricTarget{
		Type:         autoscalingv2.AverageValueMetricType,
		AverageValue: &cpu,
	})

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	_, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(context.TODO(), initialHPA, metav1.CreateOptions{})
	require.NoError(s.t, err)

	err = ensureHPA(context.TODO(), s.clusterClient, a, "web")
	require.ErrorContains(s.t, err, "autoscale cpu value cannot be greater than 95%")
}

func TestValidateBehaviorPercentageNoFail(t *testing.T) {
	t.Run("nil params returns default value", func(t *testing.T) {
		defaultValue := int32(50)
		got := getBehaviorPercentageNoFail(nil, defaultValue)
		require.Equal(t, defaultValue, got)
	})

	t.Run("empty ScaleDownPolicy returns default value", func(t *testing.T) {
		defaultValue := int32(10)
		got := getBehaviorPercentageNoFail(&provTypes.ScaleDownPolicy{}, defaultValue)
		require.Equal(t, defaultValue, got)
	})

	t.Run("PercentagePolicyValue set returns its value", func(t *testing.T) {
		val := int32(20)
		policy := &provTypes.ScaleDownPolicy{PercentagePolicyValue: &val}
		defaultValue := int32(10)
		got := getBehaviorPercentageNoFail(policy, defaultValue)
		require.Equal(t, val, got)
	})

	t.Run("StabilizationWindow set, PercentagePolicyValue nil returns default value", func(t *testing.T) {
		win := int32(300)
		policy := &provTypes.ScaleDownPolicy{StabilizationWindow: &win}
		defaultValue := int32(10)
		got := getBehaviorPercentageNoFail(policy, defaultValue)
		require.Equal(t, defaultValue, got)
	})
}

func TestValidateBehaviorUnitsNoFail(t *testing.T) {
	t.Run("nil params returns default value", func(t *testing.T) {
		defaultValue := int32(2)
		got := getBehaviorUnitsNoFail(nil, defaultValue)
		require.Equal(t, defaultValue, got)
	})

	t.Run("empty ScaleDownPolicy returns default value", func(t *testing.T) {
		defaultValue := int32(10)
		got := getBehaviorUnitsNoFail(&provTypes.ScaleDownPolicy{}, defaultValue)
		require.Equal(t, defaultValue, got)
	})

	t.Run("UnitsPolicyValue set returns its value", func(t *testing.T) {
		val := int32(20)
		policy := &provTypes.ScaleDownPolicy{UnitsPolicyValue: &val}
		defaultValue := int32(10)
		got := getBehaviorUnitsNoFail(policy, defaultValue)
		require.Equal(t, val, got)
	})

	t.Run("StabilizationWindow set, UnitsPolicyValue nil returns default value", func(t *testing.T) {
		win := int32(300)
		policy := &provTypes.ScaleDownPolicy{StabilizationWindow: &win}
		defaultValue := int32(10)
		got := getBehaviorUnitsNoFail(policy, defaultValue)
		require.Equal(t, defaultValue, got)
	})
}

func (s *S) TestProvisionerSetAutoScaleVersionUpdateOnProcessRemoval(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	// Deploy v1 with 2 processes: web and worker
	version1 := newSuccessfulVersion(c, a, map[string][]string{
		"web":    {"python", "web.py"},
		"worker": {"python", "worker.py"},
	})
	err := s.p.AddUnits(context.TODO(), a, 1, "web", version1, nil)
	require.NoError(s.t, err)
	wait()
	err = s.p.AddUnits(context.TODO(), a, 1, "worker", version1, nil)
	require.NoError(s.t, err)
	wait()

	// Set autoscale on both processes
	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		Process:    "web",
		MinUnits:   1,
		MaxUnits:   5,
		AverageCPU: "500m",
	})
	require.NoError(s.t, err)
	err = s.p.SetAutoScale(context.TODO(), a, provTypes.AutoScaleSpec{
		Process:    "worker",
		MinUnits:   1,
		MaxUnits:   3,
		AverageCPU: "500m",
	})
	require.NoError(s.t, err)

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)

	// Verify both HPAs exist with version 1
	hpas, err := s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, hpas.Items, 2)

	webHPA := findHPAByProcess(hpas.Items, "web")
	workerHPA := findHPAByProcess(hpas.Items, "worker")
	require.NotNil(s.t, webHPA, "web HPA should exist")
	require.NotNil(s.t, workerHPA, "worker HPA should exist")
	require.Equal(s.t, "1", webHPA.Labels["tsuru.io/app-version"])
	require.Equal(s.t, "1", workerHPA.Labels["tsuru.io/app-version"])

	// Deploy v2 with only web process (remove worker) - NO preserveVersions
	version2 := newSuccessfulVersion(c, a, map[string][]string{
		"web": {"python", "web.py"},
	})

	// Use servicecommon pipeline WITHOUT preserveVersions
	manager := &serviceManager{client: s.clusterClient, writer: &bytes.Buffer{}}
	err = servicecommon.RunServicePipeline(context.TODO(), manager, version1.Version(), provision.DeployArgs{
		App:              a,
		Version:          version2,
		PreserveVersions: false, // This is the key difference
	}, servicecommon.ProcessSpec{
		"web": {Start: true},
	})
	require.NoError(s.t, err)
	wait()

	// Verify worker HPA is removed
	hpas, err = s.client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, hpas.Items, 1, "only web HPA should remain")

	// Verify web HPA is updated to version 2 (not stuck at version 1)
	webHPA = findHPAByProcess(hpas.Items, "web")
	require.NotNil(s.t, webHPA, "web HPA should still exist")
	require.Equal(s.t, "2", webHPA.Labels["tsuru.io/app-version"], "web HPA should be updated to version 2")

	// Verify HPA targets the correct deployment
	require.Equal(s.t, "myapp-web", webHPA.Spec.ScaleTargetRef.Name)
}

func findHPAByProcess(hpas []autoscalingv2.HorizontalPodAutoscaler, processName string) *autoscalingv2.HorizontalPodAutoscaler {
	for i := range hpas {
		if hpas[i].Labels["tsuru.io/app-process"] == processName {
			return &hpas[i]
		}
	}
	return nil
}
