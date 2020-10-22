// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"io"
	"net/http"

	opentracingHTTP "github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
)

func withOpenTracing(cli *http.Client) *http.Client {
	return &http.Client{
		Timeout:       cli.Timeout,
		CheckRedirect: cli.CheckRedirect,
		Transport: &AutoOpentracingTransport{
			RoundTripper: cli.Transport,
		},
	}
}

func OpentracingTransport(rt http.RoundTripper) http.RoundTripper {
	return &AutoOpentracingTransport{RoundTripper: rt}
}

type AutoOpentracingTransport struct {
	http.RoundTripper
}

func (t *AutoOpentracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := t.RoundTripper
	tracer := opentracing.GlobalTracer()
	if rt == nil {
		rt = http.DefaultTransport
	}

	req, ht := opentracingHTTP.TraceRequest(tracer, req)

	transport := &opentracingHTTP.Transport{RoundTripper: rt}
	response, err := transport.RoundTrip(req)

	if err != nil {
		ht.Finish()
		return nil, err
	}
	response.Body = &autoCloseTracer{ht: ht, ReadCloser: response.Body}
	return response, nil
}

type autoCloseTracer struct {
	io.ReadCloser
	ht *opentracingHTTP.Tracer
}

func (a *autoCloseTracer) Close() error {
	err := a.ReadCloser.Close()
	a.ht.Finish()
	return err
}
