// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package observability

import (
	"strings"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/tsuru/tsuru/log"
	"github.com/uber/jaeger-client-go"
	jaegerConfig "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-client-go/zipkin"
	jaegerMetrics "github.com/uber/jaeger-lib/metrics"
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

	sampler, err := NewTsuruJaegerSamplerFromConfig(cfg)
	if err != nil {
		log.Fatal(err.Error())
	}

	tracer, _, err := cfg.NewTracer(
		jaegerConfig.Injector(opentracing.HTTPHeaders, zipkinPropagator),
		jaegerConfig.Extractor(opentracing.HTTPHeaders, zipkinPropagator),
		jaegerConfig.Injector(opentracing.TextMap, zipkinPropagator),
		jaegerConfig.Extractor(opentracing.TextMap, zipkinPropagator),
		jaegerConfig.Sampler(sampler),
	)
	if err == nil {
		opentracing.SetGlobalTracer(tracer)
	} else {
		// FIXME: we need to mark that traces are disabled
		log.Debugf("Could not initialize jaeger tracer: %s", err.Error())
	}

}

var (
	_                        jaeger.Sampler = &tsuruJaegerSampler{}
	writeOperations          []string       = []string{"POST", "PUT", "DELETE"}
	writeOperationsBlackList []string       = []string{"POST /node/status"}
)

func NewTsuruJaegerSamplerFromConfig(cfg *jaegerConfig.Configuration) (*tsuruJaegerSampler, error) {
	cfgSampler := cfg.Sampler
	if cfgSampler == nil {
		cfgSampler = &jaegerConfig.SamplerConfig{
			Type:  jaeger.SamplerTypeRemote,
			Param: 0.001,
		}
	}

	fallbackSampler, err := cfgSampler.NewSampler(cfg.ServiceName, jaeger.NewMetrics(jaegerMetrics.NullFactory, nil))
	if err != nil {
		return nil, err
	}

	return &tsuruJaegerSampler{fallbackSampler: fallbackSampler}, nil
}

type tsuruJaegerSampler struct {
	fallbackSampler jaeger.Sampler
}

func (t *tsuruJaegerSampler) Close() {
	t.fallbackSampler.Close()
}

func (*tsuruJaegerSampler) Equal(other jaeger.Sampler) bool {
	_, ok := other.(*tsuruJaegerSampler)
	return ok
}

func (t *tsuruJaegerSampler) IsSampled(id jaeger.TraceID, operation string) (sampled bool, tags []jaeger.Tag) {
	if isWriteOperationBlackList(operation) {
		return t.fallbackSampler.IsSampled(id, operation)
	}

	for _, writeOperation := range writeOperations {
		if strings.HasPrefix(operation, writeOperation) {
			return true, []jaeger.Tag{
				jaeger.NewTag("sampler.type", "tsuru"),
				jaeger.NewTag("sampling.reason", "write operation"),
			}
		}
	}
	return t.fallbackSampler.IsSampled(id, operation)
}

func isWriteOperationBlackList(operation string) bool {
	for _, writeOperation := range writeOperationsBlackList {
		if writeOperation == operation {
			return true
		}
	}
	return false
}
