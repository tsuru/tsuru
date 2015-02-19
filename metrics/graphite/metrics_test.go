// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package graphite

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuru/tsuru/metrics"
	"gopkg.in/check.v1"
)

type metricHandler struct {
	cpuMax string
}

func (h *metricHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	content := fmt.Sprintf(`[{"target": "sometarget", "datapoints": [[2.2, 1415129040], [2.2, 1415129050], [2.2, 1415129060], [2.2, 1415129070], [%s, 1415129080]]}]`, h.cpuMax)
	w.Write([]byte(content))
}

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestMetric(c *check.C) {
	h := metricHandler{cpuMax: "8.2"}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	g := graphite{host: ts.URL}
	series, err := g.Summarize("cpu", "1h", "max")
	c.Assert(err, check.IsNil)
	expected := metrics.Series([]metrics.Data{
		{Timestamp: 1.41512904e+09, Value: 2.2},
		{Timestamp: 1.41512905e+09, Value: 2.2},
		{Timestamp: 1.41512906e+09, Value: 2.2},
		{Timestamp: 1.41512907e+09, Value: 2.2},
		{Timestamp: 1.41512908e+09, Value: 8.2},
	})
	c.Assert(series, check.DeepEquals, expected)
}

func (s *S) TestMetricServerDown(c *check.C) {
	h := metricHandler{cpuMax: "8.2"}
	ts := httptest.NewServer(&h)
	ts.Close()
	g := graphite{host: ts.URL}
	_, err := g.Summarize("cpu", "1h", "max")
	c.Assert(err, check.Not(check.IsNil))
}

func (s *S) TestMetricEnvWithoutSchema(c *check.C) {
	h := metricHandler{cpuMax: "8.2"}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	g := graphite{host: ts.URL}
	series, err := g.Summarize("cpu", "1h", "max")
	c.Assert(err, check.IsNil)
	expected := metrics.Series([]metrics.Data{
		{Timestamp: 1.41512904e+09, Value: 2.2},
		{Timestamp: 1.41512905e+09, Value: 2.2},
		{Timestamp: 1.41512906e+09, Value: 2.2},
		{Timestamp: 1.41512907e+09, Value: 2.2},
		{Timestamp: 1.41512908e+09, Value: 8.2},
	})
	c.Assert(series, check.DeepEquals, expected)
}

func (s *S) TestDetect(c *check.C) {
	g := graphite{}
	c.Assert(g.Detect(nil), check.Equals, false)
	conf := map[string]string{
		"GRAPHITE_HOST": "http://ble",
	}
	c.Assert(g.Detect(conf), check.Equals, true)
}
