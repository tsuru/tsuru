// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// withOpenTelemetry takes an existing http.Client and returns a new one
// that is instrumented with OpenTelemetry.
// Other client configurations like Timeout and CheckRedirect are preserved.
func withOpenTelemetry(cli *http.Client) *http.Client {
	transport := cli.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{
		Timeout:       cli.Timeout,
		CheckRedirect: cli.CheckRedirect,
		Transport:     otelhttp.NewTransport(transport),
	}
}

// OpenTelemetryTransport takes an http.RoundTripper and returns a new one
// that is instrumented with OpenTelemetry.
func OpenTelemetryTransport(rt http.RoundTripper) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return otelhttp.NewTransport(rt)
}
