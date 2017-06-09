// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	requestLatencies = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tsuru_router_request_duration_seconds",
		Help:    "The router requests latency distributions.",
		Buckets: append(prometheus.DefBuckets, []float64{15, 30, 45, 60, 90, 120}...),
	}, []string{"router"})

	requestErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_router_request_errors_total",
		Help: "The total number of router request errors.",
	}, []string{"router"})
)

func init() {
	prometheus.MustRegister(requestLatencies)
	prometheus.MustRegister(requestErrors)
}

func InstrumentRequest(routerName string) func(error) {
	begin := time.Now()
	return func(err error) {
		requestLatencies.WithLabelValues(routerName).Observe(time.Since(begin).Seconds())
		if err != nil {
			requestErrors.WithLabelValues(routerName).Inc()
		}
	}
}
