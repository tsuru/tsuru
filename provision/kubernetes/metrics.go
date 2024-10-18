// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/provision"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

func (p *kubernetesProvisioner) UnitsMetrics(ctx context.Context, a *appTypes.App) ([]provTypes.UnitMetric, error) {
	clusterClient, err := clusterForPool(ctx, a.Pool)
	if err != nil {
		return nil, err
	}
	ns, err := clusterClient.AppNamespace(ctx, a)
	if err != nil {
		return nil, err
	}

	metricsClient, err := MetricsClientForConfig(clusterClient.restConfig)
	if err != nil {
		return nil, err
	}
	l, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: a,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix: tsuruLabelPrefix,
		},
	})
	if err != nil {
		return nil, err
	}
	labelSelector := labels.SelectorFromSet(l.ToAppSelector())
	metricList, err := metricsClient.MetricsV1beta1().PodMetricses(ns).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})
	if err != nil {
		return nil, err
	}

	unitMetrics := []provTypes.UnitMetric{}
	for _, metric := range metricList.Items {
		totalCPUUsage := resource.NewQuantity(0, resource.DecimalSI)
		totalMemoryUsage := resource.NewQuantity(0, resource.BinarySI)

		for _, container := range metric.Containers {
			cpuUsage, ok := container.Usage["cpu"]
			if !ok {
				continue
			}
			totalCPUUsage.Add(cpuUsage)
		}

		for _, container := range metric.Containers {
			memoryUsage, ok := container.Usage["memory"]
			if !ok {
				continue
			}
			totalMemoryUsage.Add(memoryUsage)
		}

		unitMetrics = append(unitMetrics, provTypes.UnitMetric{
			ID:     metric.ObjectMeta.Name,
			CPU:    totalCPUUsage.String(),
			Memory: totalMemoryUsage.String(),
		})
	}

	return unitMetrics, nil
}
