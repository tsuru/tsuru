// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"fmt"
	"net/http"
	"time"

	"github.com/tsuru/config"
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
		c.Assert(testCase.cli.Transport, check.NotNil)
		if testCase.followRedirects {
			c.Assert(testCase.cli.CheckRedirect, check.IsNil)
		} else {
			c.Assert(testCase.cli.CheckRedirect, check.Not(check.IsNil))
		}

		fmt.Println(testCase.name, "OK")
	}
}

func (s *S) TestProxyFromConfig(c *check.C) {
	config.Set("proxy:gcr.io", "my.proxy:8123")
	defer config.Unset("proxy")

	proxy, ok := proxyFromConfig("gcr.io")
	c.Assert(proxy, check.Equals, "my.proxy:8123")
	c.Assert(ok, check.Equals, true)

	proxy, ok = proxyFromConfig("other.io")
	c.Assert(proxy, check.Equals, "")
	c.Assert(ok, check.Equals, false)

	proxy, ok = proxyFromConfig("http://gcr.io/xyz")
	c.Assert(proxy, check.Equals, "my.proxy:8123")
	c.Assert(ok, check.Equals, true)
}
