// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package observability

import (
	"encoding/json"
	"fmt"
	stdLog "log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
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
	json   bool
}

func (l *middleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	next(rw, r)

	if r.URL.Path == "/healthcheck" || r.URL.Path == "/metrics" {
		return
	}

	duration := time.Since(start)
	statusCode := rw.(negroni.ResponseWriter).Status()
	if statusCode == 0 {
		statusCode = 200
	}
	nowFormatted := time.Now().Format(time.RFC3339Nano)

	var requestID string
	if header := requestIDHeader(); header != "" {
		requestID = context.GetRequestID(r, header)
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	// finish tracing
	span := trace.SpanFromContext(r.Context())
	if span.IsRecording() {
		span.SetAttributes(semconv.HTTPStatusCodeKey.Int(statusCode))
		if statusCode >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, "HTTP server error")
		}
		span.End()
	}

	// finish metrics
	path := r.URL.Query().Get(":mux-path-template")
	status := normalizeHTTPStatus(statusCode)
	httpRequests.WithLabelValues(status, r.Method, path).Inc()
	httpDuration.WithLabelValues(r.Method, path).Observe(duration.Seconds())

	durationMS := float64(duration) / float64(time.Millisecond)

	if !l.json {
		if requestID != "" {
			requestID = fmt.Sprintf(" [Request-ID: %s]", requestID)
		}

		// finish logs
		l.logger.Printf("%s %s %s %s %d %q in %0.6fms%s", nowFormatted, scheme, r.Method, r.URL.Path, statusCode, r.UserAgent(), durationMS, requestID)
		return
	}

	line := &logLine{
		Time: nowFormatted,
		Request: logLineRequest{
			Scheme:    scheme,
			Method:    r.Method,
			Path:      r.URL.Path,
			RequestID: requestID,
			UserAgent: r.UserAgent(),
		},
		Response: logLineResponse{
			StatusCode: statusCode,
			DurationMS: fmt.Sprintf("%0.6f", durationMS),
		},
	}

	if r.RemoteAddr != "" {
		line.Request.SourceIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	if token := context.GetAuthToken(r); token != nil {
		line.Auth = &logLineAuth{
			Username: token.GetUserName(),
			Engine:   token.Engine(),
		}
	}

	data, err := json.Marshal(line)
	if err == nil {
		l.logger.Print(string(data))
	} else {
		l.logger.Printf("could not marshal json: %s", err.Error())
	}

}

type logLine struct {
	Time     string          `json:"time"`
	Request  logLineRequest  `json:"request"`
	Response logLineResponse `json:"response"`
	Auth     *logLineAuth    `json:"auth,omitempty"`
}

type logLineRequest struct {
	Scheme    string `json:"scheme"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	UserAgent string `json:"userAgent,omitempty"`
	RequestID string `json:"requestID,omitempty"`
	SourceIP  string `json:"sourceIP,omitempty"`
}

type logLineResponse struct {
	StatusCode int    `json:"statusCode"`
	DurationMS string `json:"durationMS"`
}

type logLineAuth struct {
	Username string `json:"username"`
	Engine   string `json:"engine"`
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
	logFormat, _ := config.GetString("log:format")

	return &middleware{
		logger: stdLog.New(os.Stdout, "", 0),
		json:   logFormat == "json",
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
	tracer := otel.Tracer("tsuru/api")
	pathTemplate := r.URL.Query().Get(":mux-path-template")

	opName := r.Method
	if pathTemplate != "" {
		opName = r.Method + " " + pathTemplate
	}

	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	newCtx, span := tracer.Start(ctx, opName, trace.WithSpanKind(trace.SpanKindServer))

	requestID := ""
	if requestIDHeader := requestIDHeader(); requestIDHeader != "" {
		requestID = r.Header.Get(requestIDHeader)
	}

	span.SetAttributes(
		semconv.HTTPMethodKey.String(r.Method),
		semconv.HTTPURLKey.String(sanitizeURL(r.URL).RequestURI()),
		semconv.NetHostNameKey.String(r.URL.Host),
		// TODO: consider adding a more specific request ID attribute if available, e.g. X-Request-ID
		// For now, using a generic attribute if a requestID is present.
		// semconv.HTTPRequestIDKey is not available in v1.21.0, consider updating semconv or using a custom attribute.
	)
	if requestID != "" {
		span.SetAttributes(attribute.String("http.request_id", requestID))
	}

	*r = *r.WithContext(newCtx)
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

func requestIDHeader() string {
	requestIDHeader, _ := config.GetString("request-id-header")
	return requestIDHeader
}
