// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/servicemanager"
)

var (
	desc        = prometheus.NewDesc("tsuru_cluster_info", "Basic information about existing clusters", []string{"provisioner", "name"}, nil)
	poolsDesc   = prometheus.NewDesc("tsuru_cluster_pool", "information about related pool that are inside the cluster", []string{"name", "pool"}, nil)
	failureDesc = prometheus.NewDesc("tsuru_cluster_fetch_fail", "indicates whether failed to get clusters", []string{}, nil)
)

func init() {
	prometheus.MustRegister(&clustersMetricCollector{})
}

type clustersMetricCollector struct{}

func (c *clustersMetricCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- desc
}

func (c *clustersMetricCollector) Collect(ch chan<- prometheus.Metric) {
	clusters, err := servicemanager.Cluster.List()
	failureValue := float64(0)
	if err != nil {
		failureValue = float64(1)
		log.Errorf("Could not get clusters: %s", err.Error())
	}

	ch <- prometheus.MustNewConstMetric(failureDesc, prometheus.GaugeValue, failureValue)

	if failureValue > 0 {
		return
	}

	for _, cluster := range clusters {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(1), cluster.Provisioner, cluster.Name)

		for _, pool := range cluster.Pools {
			ch <- prometheus.MustNewConstMetric(poolsDesc, prometheus.GaugeValue, float64(1), cluster.Name, pool)
		}
	}
}
