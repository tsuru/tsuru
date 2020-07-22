// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/tsuru/servicemanager"
	provTypes "github.com/tsuru/tsuru/types/provision"

	check "gopkg.in/check.v1"
)

func (s *S) TestClusterMetrics(c *check.C) {
	servicemanager.Cluster = &provTypes.MockClusterService{
		OnList: func() ([]provTypes.Cluster, error) {
			return []provTypes.Cluster{
				{
					Name:        "my-cluster",
					Provisioner: "k9s",
				},
				{
					Name:        "my-k8s",
					Provisioner: "k8s",
				},
			}, nil
		},
	}

	prometheusRegistry := prometheus.NewRegistry()
	collector := &clustersMetricCollector{}
	prometheusRegistry.MustRegister(collector)

	metricGroups, err := prometheusRegistry.Gather()

	c.Assert(err, check.IsNil)
	c.Assert(metricGroups, check.HasLen, 2)
	c.Assert(metricGroups[0].GetName(), check.Equals, "tsuru_cluster_fetch_fail")
	metrics := metricGroups[0].Metric
	c.Assert(metrics, check.HasLen, 1)
	c.Assert(metrics[0].GetGauge().GetValue(), check.Equals, float64(0))

	c.Assert(metricGroups[1].GetName(), check.Equals, "tsuru_cluster_info")
	metrics = metricGroups[1].Metric
	c.Assert(metrics, check.HasLen, 2)
	c.Assert(metrics[0].GetGauge().GetValue(), check.Equals, float64(1))

	labels := metrics[0].GetLabel()
	c.Assert(labels, check.HasLen, 2)
	c.Assert(labels[0].GetName(), check.Equals, "name")
	c.Assert(labels[0].GetValue(), check.Equals, "my-cluster")
	c.Assert(labels[1].GetName(), check.Equals, "provisioner")
	c.Assert(labels[1].GetValue(), check.Equals, "k9s")

	labels = metrics[1].GetLabel()
	c.Assert(labels, check.HasLen, 2)
	c.Assert(labels[0].GetName(), check.Equals, "name")
	c.Assert(labels[0].GetValue(), check.Equals, "my-k8s")
	c.Assert(labels[1].GetName(), check.Equals, "provisioner")
	c.Assert(labels[1].GetValue(), check.Equals, "k8s")
}

func (s *S) TestClusterMetricsErrors(c *check.C) {
	servicemanager.Cluster = &provTypes.MockClusterService{
		OnList: func() ([]provTypes.Cluster, error) {
			return nil, errors.New("unknow error")
		},
	}

	prometheusRegistry := prometheus.NewRegistry()
	collector := &clustersMetricCollector{}
	prometheusRegistry.MustRegister(collector)

	metricGroups, err := prometheusRegistry.Gather()

	c.Assert(err, check.IsNil)
	c.Assert(metricGroups, check.HasLen, 1)
	c.Assert(metricGroups[0].GetName(), check.Equals, "tsuru_cluster_fetch_fail")

	metrics := metricGroups[0].Metric
	c.Assert(metrics, check.HasLen, 1)
	c.Assert(metrics[0].GetGauge().GetValue(), check.Equals, float64(1))
}
