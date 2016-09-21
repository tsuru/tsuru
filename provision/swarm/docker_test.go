// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"crypto/tls"
	"net/http"

	"github.com/fsouza/go-dockerclient/testing"
	"gopkg.in/check.v1"
)

func (s *S) TestNewClient(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	cli, err := newClient(srv.URL())
	c.Assert(err, check.IsNil)
	err = cli.Ping()
	c.Assert(err, check.IsNil)
	httpTrans, ok := cli.HTTPClient.Transport.(*http.Transport)
	c.Assert(ok, check.Equals, true)
	c.Assert(httpTrans.DisableKeepAlives, check.Equals, true)
	c.Assert(httpTrans.MaxIdleConnsPerHost, check.Equals, -1)
}

func (s *S) TestNewClientTLSConfig(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	swarmConfig.tlsConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	cli, err := newClient(srv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(cli.TLSConfig, check.DeepEquals, swarmConfig.tlsConfig)
	httpTrans, ok := cli.HTTPClient.Transport.(*http.Transport)
	c.Assert(ok, check.Equals, true)
	c.Assert(httpTrans.TLSClientConfig, check.DeepEquals, swarmConfig.tlsConfig)
}
