// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	check "gopkg.in/check.v1"
)

func (s *S) TestClient(c *check.C) {
	testCases := []struct {
		name            string
		cli             *http.Client
		timeout         time.Duration
		maxIddle        int
		followRedirects bool
		insecure        bool
	}{
		{
			name:            "Dial15Full300Client",
			cli:             Dial15Full300Client,
			timeout:         5 * time.Minute,
			maxIddle:        5,
			followRedirects: true,
		},
		{
			name:            "Dial15FullUnlimitedClient",
			cli:             Dial15FullUnlimitedClient,
			timeout:         0 * time.Second,
			maxIddle:        5,
			followRedirects: true,
		},
		{
			name:            "Dial15Full300ClientNoKeepAlive",
			cli:             Dial15Full300ClientNoKeepAlive,
			timeout:         5 * time.Minute,
			maxIddle:        -1,
			followRedirects: true,
		},
		{
			name:            "Dial15Full60ClientNoKeepAlive",
			cli:             Dial15Full60ClientNoKeepAlive,
			timeout:         1 * time.Minute,
			maxIddle:        -1,
			followRedirects: true,
		},
		{
			name:            "Dial15Full60ClientNoKeepAliveNoRedirect",
			cli:             Dial15Full60ClientNoKeepAliveNoRedirect,
			timeout:         1 * time.Minute,
			maxIddle:        -1,
			followRedirects: false,
		},
		{
			name:            "Dial15Full60ClientNoKeepAliveNoRedirectInsecure",
			cli:             Dial15Full60ClientNoKeepAliveNoRedirectInsecure,
			timeout:         1 * time.Minute,
			maxIddle:        -1,
			followRedirects: false,
			insecure:        true,
		},
		{
			name:            "Dial15Full60ClientNoKeepAliveInsecure",
			cli:             Dial15Full60ClientNoKeepAliveInsecure,
			timeout:         1 * time.Minute,
			maxIddle:        -1,
			followRedirects: true,
			insecure:        true,
		},
		{
			name:            "Dial15Full60ClientWithPool",
			cli:             Dial15Full60ClientWithPool,
			timeout:         1 * time.Minute,
			maxIddle:        10,
			followRedirects: true,
		},
		{
			name:            "Dial15Full300ClientWithPool",
			cli:             Dial15Full300ClientWithPool,
			timeout:         5 * time.Minute,
			maxIddle:        10,
			followRedirects: true,
		},
	}

	for _, testCase := range testCases {
		fmt.Println(testCase.name)
		c.Assert(testCase.cli.Timeout, check.Equals, testCase.timeout)
		opentracingTransport := testCase.cli.Transport.(*AutoOpentracingTransport)
		transport := opentracingTransport.RoundTripper.(*http.Transport)
		c.Assert(transport.TLSHandshakeTimeout, check.Equals, 15*time.Second)
		c.Assert(transport.IdleConnTimeout, check.Equals, 15*time.Second)
		c.Assert(transport.MaxIdleConnsPerHost, check.Equals, testCase.maxIddle)
		if testCase.followRedirects {
			c.Assert(testCase.cli.CheckRedirect, check.IsNil)
		} else {
			c.Assert(testCase.cli.CheckRedirect, check.Not(check.IsNil))
		}

		tlsConfig := transport.TLSClientConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		}
		c.Assert(tlsConfig.InsecureSkipVerify, check.Equals, testCase.insecure)

		fmt.Println(testCase.name, "OK")
	}
}
