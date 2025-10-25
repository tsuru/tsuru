// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package observability

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/tsuru/tsuru/log"
)

var (
	tracerProvider          *sdktrace.TracerProvider
	writeOperations         = []string{"POST", "PUT", "DELETE"}
	writeOperationsDenyList = []string{"POST /node/status"}
)

func init() {
	if err := initTracer(); err != nil {
		log.Debugf("Could not initialize OpenTelemetry tracer: %s", err.Error())
	}
}

func initTracer() error {
	ctx := context.Background()

	// Resource with service name
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("tsurud"),
		),
	)
	if err != nil {
		return err
	}

	// Configure OTLP gRPC exporter
	var exporter *otlptrace.Exporter

	// Check if OTLP endpoint is configured
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpoint == "" {
		// Fallback to Jaeger env or default
		otlpEndpoint = os.Getenv("JAEGER_ENDPOINT")
		if otlpEndpoint == "" {
			otlpEndpoint = "localhost:4317" // Default OTLP gRPC port
		}
	}

	exporter, err = otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(otlpEndpoint),
		otlptracegrpc.WithInsecure(), // Use TLS in production
	)
	if err != nil {
		return err
	}

	// Sampler
	sampler := &tsuruSampler{
		defaultSampler: sdktrace.TraceIDRatioBased(getSamplingRatio()),
	}

	// Tracer provider
	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tracerProvider)

	// Set propagator (TraceContext + B3)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, // W3C Trace Context (recommended)
			propagation.Baggage{},
			b3Propagator{}, // B3 for backward compatibility
		),
	)

	return nil
}

func getSamplingRatio() float64 {
	ratio := 0.001
	if param := os.Getenv("JAEGER_SAMPLER_PARAM"); param != "" {
		var samplerParam float64
		if _, err := fmt.Sscanf(param, "%f", &samplerParam); err == nil {
			ratio = samplerParam
		}
	}
	if param := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); param != "" {
		var samplerParam float64
		if _, err := fmt.Sscanf(param, "%f", &samplerParam); err == nil {
			ratio = samplerParam
		}
	}
	return ratio
}

func Shutdown(ctx context.Context) error {
	if tracerProvider != nil {
		return tracerProvider.Shutdown(ctx)
	}
	return nil
}

// tsuruSampler implements custom sampling logic
type tsuruSampler struct {
	defaultSampler sdktrace.Sampler
}

func (s *tsuruSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	operation := p.Name
	if isWriteOperationDenied(operation) {
		return s.defaultSampler.ShouldSample(p)
	}
	for _, writeOp := range writeOperations {
		if strings.HasPrefix(operation, writeOp) {
			return sdktrace.SamplingResult{
				Decision:   sdktrace.RecordAndSample,
				Tracestate: trace.SpanContextFromContext(p.ParentContext).TraceState(),
			}
		}
	}
	return s.defaultSampler.ShouldSample(p)
}

func (s *tsuruSampler) Description() string {
	return "TsuruSampler{default=" + s.defaultSampler.Description() + "}"
}

func isWriteOperationDenied(operation string) bool {
	for _, deniedOp := range writeOperationsDenyList {
		if deniedOp == operation {
			return true
		}
	}
	return false
}

// b3Propagator provides B3 propagation
type b3Propagator struct{}

func (b3Propagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return
	}
	carrier.Set("b3", fmt.Sprintf("%s-%s-%s", sc.TraceID().String(), sc.SpanID().String(), sampledFlagToString(sc.IsSampled())))
	carrier.Set("X-B3-TraceId", sc.TraceID().String())
	carrier.Set("X-B3-SpanId", sc.SpanID().String())
	carrier.Set("X-B3-Sampled", sampledFlagToString(sc.IsSampled()))
}

func (b3Propagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if b3Header := carrier.Get("b3"); b3Header != "" {
		return extractB3Single(ctx, b3Header)
	}
	return extractB3Multi(ctx, carrier)
}

func (b3Propagator) Fields() []string {
	return []string{"b3", "X-B3-TraceId", "X-B3-SpanId", "X-B3-Sampled"}
}

func sampledFlagToString(sampled bool) string {
	if sampled {
		return "1"
	}
	return "0"
}

func extractB3Single(ctx context.Context, b3 string) context.Context {
	parts := strings.Split(b3, "-")
	if len(parts) < 2 {
		return ctx
	}
	traceID, err := trace.TraceIDFromHex(parts[0])
	if err != nil {
		return ctx
	}
	spanID, err := trace.SpanIDFromHex(parts[1])
	if err != nil {
		return ctx
	}
	var flags trace.TraceFlags
	if len(parts) >= 3 && parts[2] == "1" {
		flags = trace.FlagsSampled
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: flags, Remote: true})
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

func extractB3Multi(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	traceIDStr := carrier.Get("X-B3-TraceId")
	spanIDStr := carrier.Get("X-B3-SpanId")
	if traceIDStr == "" || spanIDStr == "" {
		return ctx
	}
	traceID, err := trace.TraceIDFromHex(traceIDStr)
	if err != nil {
		return ctx
	}
	spanID, err := trace.SpanIDFromHex(spanIDStr)
	if err != nil {
		return ctx
	}
	var flags trace.TraceFlags
	if carrier.Get("X-B3-Sampled") == "1" {
		flags = trace.FlagsSampled
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: flags, Remote: true})
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}
