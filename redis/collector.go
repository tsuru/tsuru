// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redis

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	labels = []string{"pool"}

	requestsDesc  = prometheus.NewDesc("tsuru_redis_connections_requests_total", "The total number of connections requests to redis pool.", labels, nil)
	hitsDesc      = prometheus.NewDesc("tsuru_redis_connections_hits_total", "The total number of times a free connection was found in redis pool.", labels, nil)
	waitsDesc     = prometheus.NewDesc("tsuru_redis_connections_waits_total", "The total number of times the redis pool had to wait for a connection.", labels, nil)
	timeoutsDesc  = prometheus.NewDesc("tsuru_redis_connections_timeouts_total", "The total number of wait timeouts in redis pool.", labels, nil)
	connsDesc     = prometheus.NewDesc("tsuru_redis_connections_current", "The current number of connections in redis pool.", labels, nil)
	freeConnsDesc = prometheus.NewDesc("tsuru_redis_connections_free_current", "The current number of free connections in redis pool.", labels, nil)

	collector = &poolStatsCollector{clients: map[string]poolStatsClient{}}
)

type poolStatsCollector struct {
	sync.RWMutex
	clients map[string]poolStatsClient
}

func init() {
	prometheus.MustRegister(collector)
}

func (p *poolStatsCollector) Add(name string, client poolStatsClient) {
	p.Lock()
	p.clients[name] = client
	p.Unlock()
}

func (p *poolStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- requestsDesc
	ch <- hitsDesc
	ch <- waitsDesc
	ch <- timeoutsDesc
	ch <- connsDesc
	ch <- freeConnsDesc
}

func (p *poolStatsCollector) Collect(ch chan<- prometheus.Metric) {
	p.RLock()
	for name, client := range p.clients {
		stats := client.PoolStats()
		ch <- prometheus.MustNewConstMetric(requestsDesc, prometheus.CounterValue, float64(stats.Requests), name)
		ch <- prometheus.MustNewConstMetric(hitsDesc, prometheus.CounterValue, float64(stats.Hits), name)
		ch <- prometheus.MustNewConstMetric(waitsDesc, prometheus.CounterValue, float64(stats.Waits), name)
		ch <- prometheus.MustNewConstMetric(timeoutsDesc, prometheus.CounterValue, float64(stats.Timeouts), name)
		ch <- prometheus.MustNewConstMetric(connsDesc, prometheus.GaugeValue, float64(stats.TotalConns), name)
		ch <- prometheus.MustNewConstMetric(freeConnsDesc, prometheus.GaugeValue, float64(stats.FreeConns), name)
	}
	p.RUnlock()
}
