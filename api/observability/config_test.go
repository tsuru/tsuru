// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package observability

import (
	"fmt"

	"github.com/uber/jaeger-client-go"
	"gopkg.in/check.v1"
)

func (s *S) TestTsuruJaegerSampler(c *check.C) {
	fallbackSampler := jaeger.NewConstSampler(false)
	sampler := tsuruJaegerSampler{fallbackSampler: fallbackSampler}

	tests := []struct {
		operation string
		expected  bool
		tags      []jaeger.Tag
	}{
		{
			operation: "GET /apps",
			expected:  false,
			tags: []jaeger.Tag{
				jaeger.NewTag("sampler.type", "const"),
				jaeger.NewTag("sampler.param", false),
			},
		},
		{
			operation: "POST /apps",
			expected:  true,
			tags: []jaeger.Tag{
				jaeger.NewTag("sampler.type", "tsuru"),
				jaeger.NewTag("sampling.reason", "write operation"),
			},
		},
		{
			operation: "PUT /apps",
			expected:  true,
			tags: []jaeger.Tag{
				jaeger.NewTag("sampler.type", "tsuru"),
				jaeger.NewTag("sampling.reason", "write operation"),
			},
		},
		{
			operation: "DELETE /apps",
			expected:  true,
			tags: []jaeger.Tag{
				jaeger.NewTag("sampler.type", "tsuru"),
				jaeger.NewTag("sampling.reason", "write operation"),
			},
		},
		{
			operation: "POST /node/status",
			expected:  false,
			tags: []jaeger.Tag{
				jaeger.NewTag("sampler.type", "const"),
				jaeger.NewTag("sampler.param", false),
			},
		},
	}

	for _, test := range tests {
		fmt.Println(test.operation)
		sampled, tags := sampler.IsSampled(jaeger.TraceID{}, test.operation)

		c.Check(sampled, check.Equals, test.expected)
		c.Check(tags, check.DeepEquals, test.tags)
	}

}
