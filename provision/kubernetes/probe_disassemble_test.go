// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDisassembleHealthProbeHTTP(t *testing.T) {
	probe := &apiv1.Probe{
		ProbeHandler: apiv1.ProbeHandler{
			HTTPGet: &apiv1.HTTPGetAction{
				Path:   "/health",
				Port:   intstr.FromInt(8888),
				Scheme: apiv1.URISchemeHTTPS,
				HTTPHeaders: []apiv1.HTTPHeader{
					{Name: "Host", Value: "app.example.com"},
					{Name: "X-Check", Value: "enabled"},
				},
			},
		},
		FailureThreshold: 5,
		PeriodSeconds:    15,
		TimeoutSeconds:   4,
	}

	result, err := disassembleHealthProbe(probe)

	require.NoError(t, err)
	require.Equal(t, &provTypes.TsuruYamlHealthcheck{
		Headers: map[string]string{
			"Host":    "app.example.com",
			"X-Check": "enabled",
		},
		Path:            "/health",
		Scheme:          "https",
		AllowedFailures: 5,
		IntervalSeconds: 15,
		TimeoutSeconds:  4,
	}, result)
}

func TestDisassembleHealthProbeExec(t *testing.T) {
	probe := &apiv1.Probe{
		ProbeHandler: apiv1.ProbeHandler{
			Exec: &apiv1.ExecAction{Command: []string{"sh", "-c", "check-health"}},
		},
		FailureThreshold: 3,
		PeriodSeconds:    10,
		TimeoutSeconds:   2,
	}

	result, err := disassembleHealthProbe(probe)

	require.NoError(t, err)
	require.Equal(t, &provTypes.TsuruYamlHealthcheck{
		Scheme:          "http",
		Command:         []string{"sh", "-c", "check-health"},
		AllowedFailures: 3,
		IntervalSeconds: 10,
		TimeoutSeconds:  2,
	}, result)
}

func TestDisassembleStartupProbe(t *testing.T) {
	probe := &apiv1.Probe{
		ProbeHandler: apiv1.ProbeHandler{
			HTTPGet: &apiv1.HTTPGetAction{
				Path:   "/startup",
				Port:   intstr.FromInt(8888),
				Scheme: apiv1.URISchemeHTTP,
				HTTPHeaders: []apiv1.HTTPHeader{
					{Name: "X-Startup", Value: "enabled"},
				},
			},
		},
		FailureThreshold: 12,
		PeriodSeconds:    5,
		TimeoutSeconds:   1,
	}

	result, err := disassembleStartupProbe(probe)

	require.NoError(t, err)
	require.Equal(t, &provTypes.TsuruYamlStartupcheck{
		Headers:         map[string]string{"X-Startup": "enabled"},
		Path:            "/startup",
		Scheme:          "http",
		AllowedFailures: 12,
		IntervalSeconds: 5,
		TimeoutSeconds:  1,
	}, result)
}

func TestProcessesFromDeployments(t *testing.T) {
	oldWebProbe := &apiv1.Probe{
		ProbeHandler:     apiv1.ProbeHandler{HTTPGet: &apiv1.HTTPGetAction{Path: "/old-health"}},
		FailureThreshold: 3,
		PeriodSeconds:    10,
		TimeoutSeconds:   60,
	}
	webProbe := &apiv1.Probe{
		ProbeHandler:     apiv1.ProbeHandler{HTTPGet: &apiv1.HTTPGetAction{Path: "/health", Scheme: apiv1.URISchemeHTTPS}},
		FailureThreshold: 5,
		PeriodSeconds:    15,
		TimeoutSeconds:   4,
	}
	startupProbe := &apiv1.Probe{
		ProbeHandler:     apiv1.ProbeHandler{Exec: &apiv1.ExecAction{Command: []string{"check-startup"}}},
		FailureThreshold: 12,
		PeriodSeconds:    5,
		TimeoutSeconds:   1,
	}
	workerProbe := &apiv1.Probe{
		ProbeHandler:     apiv1.ProbeHandler{Exec: &apiv1.ExecAction{Command: []string{"check-worker"}}},
		FailureThreshold: 3,
		PeriodSeconds:    10,
		TimeoutSeconds:   2,
	}
	deployment := func(name string, container apiv1.Container) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: appsv1.DeploymentSpec{Template: apiv1.PodTemplateSpec{
				Spec: apiv1.PodSpec{Containers: []apiv1.Container{container}},
			}},
		}
	}
	grouped := groupedDeploymentsAll{versioned: map[int][]deploymentInfo{
		1: {
			{dep: deployment("web-v1", apiv1.Container{ReadinessProbe: oldWebProbe}), process: "web", version: 1, isBase: true},
		},
		2: {
			{dep: deployment("web-v2", apiv1.Container{ReadinessProbe: webProbe, LivenessProbe: webProbe, StartupProbe: startupProbe}), process: "web", version: 2, isBase: true},
			{dep: deployment("worker-v2", apiv1.Container{ReadinessProbe: workerProbe, LivenessProbe: workerProbe}), process: "worker", version: 2, isBase: true},
			{dep: deployment("ignored-v2", apiv1.Container{ReadinessProbe: webProbe}), process: "ignored", version: 2, isBase: false},
			{dep: deployment("scheduler-v2", apiv1.Container{}), process: "scheduler", version: 2, isBase: true},
		},
	}}

	result, err := processesFromDeployments(grouped)

	require.NoError(t, err)
	require.Equal(t, []appTypes.Process{
		{Name: "scheduler"},
		{
			Name: "web",
			Healthcheck: &provTypes.TsuruYamlHealthcheck{
				Path:            "/health",
				Scheme:          "https",
				AllowedFailures: 5,
				IntervalSeconds: 15,
				TimeoutSeconds:  4,
				ForceRestart:    true,
			},
			Startupcheck: &provTypes.TsuruYamlStartupcheck{
				Scheme:          "http",
				Command:         []string{"check-startup"},
				AllowedFailures: 12,
				IntervalSeconds: 5,
				TimeoutSeconds:  1,
			},
		},
		{
			Name: "worker",
			Healthcheck: &provTypes.TsuruYamlHealthcheck{
				Scheme:          "http",
				Command:         []string{"check-worker"},
				AllowedFailures: 3,
				IntervalSeconds: 10,
				TimeoutSeconds:  2,
				ForceRestart:    true,
			},
		},
	}, result)
}

func TestProcessesFromDeploymentsIgnoresLivenessProbeWithoutReadinessProbe(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web-v1"},
		Spec: appsv1.DeploymentSpec{Template: apiv1.PodTemplateSpec{
			Spec: apiv1.PodSpec{Containers: []apiv1.Container{{
				LivenessProbe: &apiv1.Probe{
					ProbeHandler: apiv1.ProbeHandler{HTTPGet: &apiv1.HTTPGetAction{Path: "/health"}},
				},
			}},
			}}},
	}
	grouped := groupedDeploymentsAll{versioned: map[int][]deploymentInfo{
		1: {{dep: deployment, process: "web", version: 1, isBase: true}},
	}}

	result, err := processesFromDeployments(grouped)

	require.NoError(t, err)
	require.Equal(t, []appTypes.Process{{Name: "web"}}, result)
}

func TestProcessesFromDeploymentsWithoutContainers(t *testing.T) {
	grouped := groupedDeploymentsAll{versioned: map[int][]deploymentInfo{
		1: {
			{
				dep:     &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "invalid"}},
				process: "web",
				version: 1,
				isBase:  true,
			},
		},
	}}

	_, err := processesFromDeployments(grouped)

	require.EqualError(t, err, `deployment "invalid" has no containers`)
}
