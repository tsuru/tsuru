// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package observability

import (
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/tsuru/tsuru/log"
	jaegerConfig "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-client-go/zipkin"
)

func init() {
	// We decided to use B3 Format, in the future plan to move to W3C context propagation
	// https://github.com/w3c/trace-context
	zipkinPropagator := zipkin.NewZipkinB3HTTPHeaderPropagator()

	// setup opentracing
	cfg, err := jaegerConfig.FromEnv()
	if err != nil {
		log.Fatal(err.Error())
	}
	cfg.ServiceName = "tsurud"

	tracer, _, err := cfg.NewTracer(
		jaegerConfig.Injector(opentracing.HTTPHeaders, zipkinPropagator),
		jaegerConfig.Extractor(opentracing.HTTPHeaders, zipkinPropagator),
		jaegerConfig.Injector(opentracing.TextMap, zipkinPropagator),
		jaegerConfig.Extractor(opentracing.TextMap, zipkinPropagator),
	)
	if err == nil {
		opentracing.SetGlobalTracer(tracer)
	} else {
		// FIXME: we need to mark that traces are disabled
		log.Debugf("Could not initialize jaeger tracer: %s", err.Error())
	}

}
