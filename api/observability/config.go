// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package observability

import (
	"strings"

	"context"
	"strings"

	"github.com/tsuru/tsuru/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func init() {
	ctx := context.Background()

	// Create a new OTLP HTTP trace exporter
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		log.Errorf("failed to create OTLP HTTP trace exporter: %v", err)
		return
	}

	// Create a new resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("tsurud"),
		),
	)
	if err != nil {
		log.Errorf("failed to create resource: %v", err)
		return
	}

	// Instantiate a new TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(NewTsuruSampler(sdktrace.ParentBased(sdktrace.AlwaysSample()))),
	)

	// Set this provider as the global tracer provider
	otel.SetTracerProvider(tp)

	// Set up a global text map propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	log.Debug("OpenTelemetry initialized successfully")
}

var (
	_                       sdktrace.Sampler = &tsuruSampler{}
	writeOperations         []string         = []string{"POST", "PUT", "DELETE"}
	writeOperationsDenyList []string         = []string{"POST /node/status"}
)

// NewTsuruSampler creates a new tsuruSampler.
// fallbackSampler is used if the operation is not a write operation or is in the deny list.
func NewTsuruSampler(fallbackSampler sdktrace.Sampler) sdktrace.Sampler {
	return &tsuruSampler{fallbackSampler: fallbackSampler}
}

type tsuruSampler struct {
	fallbackSampler sdktrace.Sampler
}

// ShouldSample implements the OpenTelemetry Sampler interface.
func (t *tsuruSampler) ShouldSample(parameters sdktrace.SamplingParameters) sdktrace.SamplingResult {
	// parameters.Name is the span name, which corresponds to `operation` in the old sampler.
	if isWriteOperationDenied(parameters.Name) {
		return t.fallbackSampler.ShouldSample(parameters)
	}

	for _, writeOperation := range writeOperations {
		if strings.HasPrefix(parameters.Name, writeOperation) {
			return sdktrace.SamplingResult{
				Decision:   sdktrace.RecordAndSample,
				Attributes: []attribute.KeyValue{attribute.String("sampler.type", "tsuru"), attribute.String("sampling.reason", "write operation")},
				Tracestate: parameters.ParentContext.TraceState(),
			}
		}
	}
	return t.fallbackSampler.ShouldSample(parameters)
}

// Description returns a human-readable description of the Sampler.
func (t *tsuruSampler) Description() string {
	return "TsuruSampler"
}

func isWriteOperationDenied(operation string) bool {
	for _, writeOperation := range writeOperationsDenyList {
		if writeOperation == operation {
			return true
		}
	}
	return false
}
