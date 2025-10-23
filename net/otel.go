// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func withOpenTelemetry(cli *http.Client) *http.Client {
	return &http.Client{
		Timeout:       cli.Timeout,
		CheckRedirect: cli.CheckRedirect,
		Transport: otelhttp.NewTransport(
			cli.Transport,
			otelhttp.WithSpanNameFormatter(func(operation string, req *http.Request) string {
				return req.Method + " " + req.URL.Path
			}),
		),
	}
}

// OtelTransport wraps an http.RoundTripper with OpenTelemetry instrumentation
func OtelTransport(rt http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(
		rt,
		otelhttp.WithSpanNameFormatter(func(operation string, req *http.Request) string {
			return req.Method + " " + req.URL.Path
		}),
	)
}
