// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package observability

import (
	"fmt"
	stdLog "log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/opentracing/opentracing-go"
	opentracingExt "github.com/opentracing/opentracing-go/ext"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
)

const (
	metricsNamespace = "tsuru"
	metricsSubsystem = "http"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "requests_total",
		Help:      "Number of HTTP operations",
	}, []string{"status", "method", "path"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "request_duration_seconds",
		Help:      "Spend time by processing a route",
		Buckets: []float64{
			0.001, // 1ms
			0.01,  // 10ms
			0.1,   // 100 ms
			0.5,
			1.0, // 1s
			5.0,
			10.0, // 10s
			20.0,
			30.0,
		},
	}, []string{"method", "path"})
)

type middleware struct {
	logger *stdLog.Logger
}

func (l *middleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	next(rw, r)
	duration := time.Since(start)
	statusCode := rw.(negroni.ResponseWriter).Status()
	if statusCode == 0 {
		statusCode = 200
	}
	nowFormatted := time.Now().Format(time.RFC3339Nano)
	requestIDHeader, _ := config.GetString("request-id-header")
	var requestID string
	if requestIDHeader != "" {
		requestID = context.GetRequestID(r, requestIDHeader)
		if requestID != "" {
			requestID = fmt.Sprintf(" [%s: %s]", requestIDHeader, requestID)
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	// finish tracing
	span := opentracing.SpanFromContext(r.Context())
	if span != nil {
		span.SetTag("http.status_code", statusCode)
		if statusCode >= http.StatusInternalServerError {
			opentracingExt.Error.Set(span, true)
		}
		span.Finish()
	}

	// finish metrics
	path := r.URL.Query().Get(":mux-path-template")
	status := normalizeHTTPStatus(statusCode)
	httpRequests.WithLabelValues(status, r.Method, path).Inc()
	httpDuration.WithLabelValues(r.Method, path).Observe(duration.Seconds())

	// finish logs
	l.logger.Printf("%s %s %s %s %d %q in %0.6fms%s", nowFormatted, scheme, r.Method, r.URL.Path, statusCode, r.UserAgent(), float64(duration)/float64(time.Millisecond), requestID)
}

func normalizeHTTPStatus(status int) string {
	if status < 200 {
		return "1xx"
	} else if status < 300 {
		return "2xx"
	} else if status < 400 {
		return "3xx"
	} else if status < 500 {
		return "4xx"
	}
	return "5xx"
}

func NewMiddleware() *middleware {
	return &middleware{
		logger: stdLog.New(os.Stdout, "", 0),
	}
}

func PrePopulateMetrics(method, path string) {
	httpRequests.WithLabelValues("1xx", method, path)
	httpRequests.WithLabelValues("2xx", method, path)
	httpRequests.WithLabelValues("3xx", method, path)
	httpRequests.WithLabelValues("4xx", method, path)
	httpRequests.WithLabelValues("5xx", method, path)
	httpDuration.WithLabelValues(method, path)
}

func StartSpan(r *http.Request) {
	tracer := opentracing.GlobalTracer()
	pathTemplate := r.URL.Query().Get(":mux-path-template")

	opName := r.Method
	if pathTemplate != "" {
		opName = r.Method + " " + pathTemplate
	}

	tags := []opentracing.StartSpanOption{
		opentracingExt.SpanKindRPCServer,
		opentracing.Tag{Key: "component", Value: "api/router"},
		opentracing.Tag{Key: "request_id", Value: r.Header.Get("X-Request-ID")},
		opentracing.Tag{Key: "http.method", Value: r.Method},
		opentracing.Tag{Key: "http.url", Value: sanitizeURL(r.URL).RequestURI()},
	}

	wireContext, err := tracer.Extract(
		opentracing.HTTPHeaders,
		opentracing.HTTPHeadersCarrier(r.Header))

	if err == nil {
		tags = append(tags, opentracing.ChildOf(wireContext))
	}

	span := tracer.StartSpan(opName, tags...)

	ctx := opentracing.ContextWithSpan(r.Context(), span)
	newR := r.WithContext(ctx)

	*r = *newR
}

func sanitizeURL(u *url.URL) *url.URL {
	destURL := *u

	values := u.Query()
	for k := range values {
		if len(k) > 0 && k[0] == ':' {
			delete(values, k)
		}
	}
	destURL.RawQuery = values.Encode()
	return &destURL
}
