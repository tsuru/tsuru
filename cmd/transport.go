// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
)

var (
	_                   http.RoundTripper = &VerboseRoundTripper{}
	zero                                  = 0
	defaultRoundTripper                   = http.DefaultTransport
)

// VerboseRoundTripper is a RoundTripper that dumps request and response
// based on the Verbosity.
// Verbosity >= 1 --> Dumps request
// Verbosity >= 2 --> Dumps response
type VerboseRoundTripper struct {
	http.RoundTripper
	Verbosity *int
	Writer    io.Writer
}

func (v *VerboseRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	roundTripper := v.RoundTripper
	if roundTripper == nil {
		roundTripper = defaultRoundTripper
	}
	verbosity := v.Verbosity
	if verbosity == nil {
		verbosity = &zero
	}
	req.Header.Add(VerbosityHeader, strconv.Itoa(*verbosity))
	req.Close = true
	if *verbosity >= 1 {
		fmt.Fprintf(v.Writer, "*************************** <Request uri=%q> **********************************\n", req.URL.RequestURI())
		requestDump, err := httputil.DumpRequest(req, true)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(v.Writer, string(requestDump))
		if requestDump[len(requestDump)-1] != '\n' {
			fmt.Fprintln(v.Writer)
		}
		fmt.Fprintf(v.Writer, "*************************** </Request uri=%q> **********************************\n", req.URL.RequestURI())
	}
	response, err := roundTripper.RoundTrip(req)
	if *verbosity >= 2 && response != nil {
		fmt.Fprintf(v.Writer, "*************************** <Response uri=%q> **********************************\n", req.URL.RequestURI())
		responseDump, errDump := httputil.DumpResponse(response, true)
		if errDump != nil {
			return nil, errDump
		}
		fmt.Fprintf(v.Writer, string(responseDump))
		if responseDump[len(responseDump)-1] != '\n' {
			fmt.Fprintln(v.Writer)
		}
		fmt.Fprintf(v.Writer, "*************************** </Response uri=%q> **********************************\n", req.URL.RequestURI())
	}
	return response, err
}
