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
)

func (p *kubernetesProvisioner) UnitsMetrics(ctx context.Context, a provision.App) ([]provision.UnitMetric, error) {
	clusterClient, err := clusterForPool(ctx, a.GetPool())
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
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
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

	unitMetrics := []provision.UnitMetric{}
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

		unitMetrics = append(unitMetrics, provision.UnitMetric{
			ID:     metric.ObjectMeta.Name,
			CPU:    totalCPUUsage.String(),
			Memory: totalMemoryUsage.String(),
		})
	}

	return unitMetrics, nil
}
