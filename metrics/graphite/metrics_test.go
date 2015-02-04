// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package graphite

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/metrics"
	"launchpad.net/gocheck"
)

type metricHandler struct {
	cpuMax string
}

func (h *metricHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	content := fmt.Sprintf(`[{"target": "sometarget", "datapoints": [[2.2, 1415129040], [2.2, 1415129050], [2.2, 1415129060], [2.2, 1415129070], [%s, 1415129080]]}]`, h.cpuMax)
	w.Write([]byte(content))
}

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestMetric(c *gocheck.C) {
	h := metricHandler{cpuMax: "8.2"}
	ts := httptest.NewServer(&h)
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	g := graphite{}
	series, err := g.Summarize("cpu", "1h", "max")
	c.Assert(err, gocheck.IsNil)
	expected := metrics.Series([]metrics.Data{
		{Timestamp: 1.41512904e+09, Value: 2.2},
		{Timestamp: 1.41512905e+09, Value: 2.2},
		{Timestamp: 1.41512906e+09, Value: 2.2},
		{Timestamp: 1.41512907e+09, Value: 2.2},
		{Timestamp: 1.41512908e+09, Value: 8.2},
	})
	c.Assert(series, gocheck.DeepEquals, expected)
}

func (s *S) TestMetricServerDown(c *gocheck.C) {
	h := metricHandler{cpuMax: "8.2"}
	ts := httptest.NewServer(&h)
	config.Set("graphite:host", ts.URL)
	ts.Close()
	g := graphite{}
	_, err := g.Summarize("cpu", "1h", "max")
	c.Assert(err, gocheck.Not(gocheck.IsNil))
}

func (s *S) TestMetricEnvWithoutSchema(c *gocheck.C) {
	h := metricHandler{cpuMax: "8.2"}
	ts := httptest.NewServer(&h)
	config.Set("graphite:host", ts.URL)
	defer ts.Close()
	g := graphite{}
	series, err := g.Summarize("cpu", "1h", "max")
	c.Assert(err, gocheck.IsNil)
	expected := metrics.Series([]metrics.Data{
		{Timestamp: 1.41512904e+09, Value: 2.2},
		{Timestamp: 1.41512905e+09, Value: 2.2},
		{Timestamp: 1.41512906e+09, Value: 2.2},
		{Timestamp: 1.41512907e+09, Value: 2.2},
		{Timestamp: 1.41512908e+09, Value: 8.2},
	})
	c.Assert(series, gocheck.DeepEquals, expected)
}
