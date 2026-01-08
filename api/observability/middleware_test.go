// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package observability

import (
	"bytes"
	stdc "context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codegangsta/negroni"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/tsuru/config"
	apiContext "github.com/tsuru/tsuru/api/context"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestMiddleware(c *check.C) {
	httpRequests.Reset()
	httpDuration.Reset()

	promReg := prometheus.NewRegistry()
	promReg.Register(httpRequests)
	promReg.Register(httpDuration)

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/my/path", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("User-Agent", "ardata 1.1")
	h, handlerLog := doHandler()
	handlerLog.sleep = 100 * time.Millisecond
	handlerLog.response = http.StatusOK
	var out bytes.Buffer
	middle := middleware{
		logger: log.New(&out, "", 0),
	}
	middle.ServeHTTP(negroni.NewResponseWriter(recorder), request, h)
	c.Assert(handlerLog.called, check.Equals, true)
	timePart := time.Now().Format(time.RFC3339Nano)[:19]
	c.Assert(out.String(), check.Matches, fmt.Sprintf(`%s\..+? http PUT /my/path 200 "ardata 1.1" in 1\d{2}\.\d+ms`+"\n", timePart))

	metricsFamilies, err := promReg.Gather()
	c.Assert(err, check.IsNil)
	c.Assert(metricsFamilies, check.HasLen, 2)

	var buf bytes.Buffer
	for _, metricFamily := range metricsFamilies {
		expfmt.MetricFamilyToText(&buf, metricFamily)
	}

	if !c.Check(strings.Contains(buf.String(), `tsuru_http_requests_total{method="PUT",path="",status="2xx"} 1`), check.Equals, true) {
		fmt.Println("Found prometheus metrics:", buf.String())
	}
	if !c.Check(strings.Contains(buf.String(), `tsuru_http_request_duration_seconds_bucket{method="PUT",path="",le="+Inf"} 1`), check.Equals, true) {
		fmt.Println("Found prometheus metrics:", buf.String())
	}
}

func (s *S) TestMiddlewareJSON(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/my/path", nil)
	request.RemoteAddr = "10.1.1.1:60000"
	c.Assert(err, check.IsNil)
	request.Header.Set("User-Agent", "ardata 1.1")

	h, handlerLog := doHandler()
	handlerLog.sleep = 100 * time.Millisecond
	handlerLog.response = http.StatusOK
	var out bytes.Buffer
	middle := middleware{
		logger: log.New(&out, "", 0),
		json:   true,
	}
	middle.ServeHTTP(negroni.NewResponseWriter(recorder), request, h)
	c.Assert(handlerLog.called, check.Equals, true)

	m := map[string]interface{}{}
	err = json.NewDecoder(&out).Decode(&m)
	c.Assert(err, check.IsNil)

	timePart := time.Now().Format(time.RFC3339Nano)[:16]
	c.Assert(m["time"], check.Matches, timePart+".*")
	c.Assert(m["request"], check.DeepEquals, map[string]interface{}{"method": "PUT", "path": "/my/path", "scheme": "http", "userAgent": "ardata 1.1", "sourceIP": "10.1.1.1"})

	response := m["response"].(map[string]interface{})
	c.Assert(response["statusCode"], check.Equals, float64(200))
	c.Assert(response["durationMS"], check.Matches, `1\d{2}\.\d+`)
}

func (s *S) TestMiddlewareWithoutStatusCode(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/my/path", nil)
	c.Assert(err, check.IsNil)
	h, handlerLog := doHandler()
	handlerLog.sleep = 100 * time.Millisecond
	handlerLog.response = 0
	var out bytes.Buffer
	middle := middleware{
		logger: log.New(&out, "", 0),
	}
	middle.ServeHTTP(negroni.NewResponseWriter(recorder), request, h)
	c.Assert(handlerLog.called, check.Equals, true)
	timePart := time.Now().Format(time.RFC3339Nano)[:19]
	c.Assert(out.String(), check.Matches, fmt.Sprintf(`%s\..+? http PUT /my/path 200 "" in 1\d{2}\.\d+ms`+"\n", timePart))
}

func (s *S) TestMiddlewareWithRequestID(c *check.C) {
	config.Set("request-id-header", "Request-ID")
	defer config.Unset("request-id-header")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/my/path", nil)
	c.Assert(err, check.IsNil)
	apiContext.SetRequestID(request, "Request-ID", "my-rid")
	h, handlerLog := doHandler()
	handlerLog.sleep = 100 * time.Millisecond
	handlerLog.response = http.StatusOK
	var out bytes.Buffer
	middle := middleware{
		logger: log.New(&out, "", 0),
	}
	middle.ServeHTTP(negroni.NewResponseWriter(recorder), request, h)
	c.Assert(handlerLog.called, check.Equals, true)
	timePart := time.Now().Format(time.RFC3339Nano)[:19]
	c.Assert(out.String(), check.Matches, fmt.Sprintf(`%s\..+? http PUT /my/path 200 "" in 1\d{2}\.\d+ms \[Request-ID: my-rid\]`+"\n", timePart))
}

func (s *S) TestMiddlewareHTTPS(c *check.C) {
	h, handlerLog := doHandler()
	handlerLog.response = http.StatusOK
	var out bytes.Buffer
	middle := middleware{
		logger: log.New(&out, "", 0),
	}
	n := negroni.New()
	n.Use(&middle)
	n.UseHandler(h)
	srv := httptest.NewTLSServer(n)
	defer srv.Close()
	cli := srv.Client()
	request, err := http.NewRequest("PUT", srv.URL+"/my/path", nil)
	c.Assert(err, check.IsNil)
	rsp, err := cli.Do(request)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusOK)
	c.Assert(handlerLog.called, check.Equals, true)
	timePart := time.Now().Format(time.RFC3339Nano)[:19]
	c.Assert(out.String(), check.Matches, fmt.Sprintf(`%s\..+? https PUT /my/path 200 "Go-http-client/1.1" in \d{1}\.\d+ms`+"\n", timePart))
}

func (s *S) TestStartSpan(c *check.C) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(stdc.TODO())

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "my-request-id")
	StartSpan(req)

	span := trace.SpanFromContext(req.Context())
	c.Assert(span, check.Not(check.IsNil))
	c.Assert(span.IsRecording(), check.Equals, true)

	// Verify span exists and is recording (attributes can't be directly inspected)
	c.Check(span, check.Not(check.IsNil))
}

type handlerLog struct {
	called   bool
	sleep    time.Duration
	response int
}

func doHandler() (http.HandlerFunc, *handlerLog) {
	h := &handlerLog{}
	return func(w http.ResponseWriter, r *http.Request) {
		if h.sleep != 0 {
			time.Sleep(h.sleep)
		}
		h.called = true
		if h.response != 0 {
			w.WriteHeader(h.response)
		}
	}, h
}
