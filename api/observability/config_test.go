// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/check.v1"
)

func (s *S) TestTsuruSampler(c *check.C) {
	fallbackSamplerNever := sdktrace.NeverSample()
	fallbackSamplerAlways := sdktrace.AlwaysSample()

	tests := []struct {
		name            string
		operation       string
		fallbackSampler sdktrace.Sampler
		expectedDecision sdktrace.SamplingDecision
		expectedAttrs   []attribute.KeyValue
	}{
		{
			name:            "Read operation with NeverSample fallback",
			operation:       "GET /apps",
			fallbackSampler: fallbackSamplerNever,
			expectedDecision: sdktrace.Drop,
			expectedAttrs:   nil, // Fallback sampler (NeverSample) doesn't add attributes
		},
		{
			name:            "Write operation (POST) should be sampled",
			operation:       "POST /apps",
			fallbackSampler: fallbackSamplerNever, // Fallback shouldn't be used
			expectedDecision: sdktrace.RecordAndSample,
			expectedAttrs: []attribute.KeyValue{
				attribute.String("sampler.type", "tsuru"),
				attribute.String("sampling.reason", "write operation"),
			},
		},
		{
			name:            "Write operation (PUT) should be sampled",
			operation:       "PUT /apps",
			fallbackSampler: fallbackSamplerNever, // Fallback shouldn't be used
			expectedDecision: sdktrace.RecordAndSample,
			expectedAttrs: []attribute.KeyValue{
				attribute.String("sampler.type", "tsuru"),
				attribute.String("sampling.reason", "write operation"),
			},
		},
		{
			name:            "Write operation (DELETE) should be sampled",
			operation:       "DELETE /apps",
			fallbackSampler: fallbackSamplerNever, // Fallback shouldn't be used
			expectedDecision: sdktrace.RecordAndSample,
			expectedAttrs: []attribute.KeyValue{
				attribute.String("sampler.type", "tsuru"),
				attribute.String("sampling.reason", "write operation"),
			},
		},
		{
			name:            "Denied write operation with AlwaysSample fallback",
			operation:       "POST /node/status",
			fallbackSampler: fallbackSamplerAlways,
			expectedDecision: sdktrace.RecordAndSample, // Fallback sampler (AlwaysSample)
			expectedAttrs:   nil,                     // Fallback sampler (AlwaysSample) doesn't add attributes by default
		},
	}

	for _, test := range tests {
		c.Logf("Running test: %s", test.name)
		sampler := NewTsuruSampler(test.fallbackSampler)
		params := sdktrace.SamplingParameters{
			ParentContext: context.Background(),
			TraceID:       trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			Name:          test.operation,
			Kind:          trace.SpanKindServer, // Assuming server spans, adjust if needed
		}
		result := sampler.ShouldSample(params)

		c.Check(result.Decision, check.Equals, test.expectedDecision, check.Commentf("Test: %s", test.name))
		if test.expectedAttrs == nil {
			c.Check(result.Attributes, check.IsNil, check.Commentf("Test: %s", test.name))
		} else {
			c.Check(result.Attributes, check.DeepEquals, test.expectedAttrs, check.Commentf("Test: %s", test.name))
		}
		c.Check(sampler.Description(), check.Equals, "TsuruSampler")
	}
}
