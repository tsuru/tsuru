// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package galeb

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	ttesting "github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct {
	conn    *db.Storage
	server  *httptest.Server
	handler ttesting.MultiTestHandler
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("galeb:username", "myusername")
	config.Set("galeb:password", "mypassword")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) SetUpTest(c *gocheck.C) {
	s.handler = ttesting.MultiTestHandler{}
	s.server = httptest.NewServer(&s.handler)
	config.Set("galeb:api-url", s.server.URL)
}

func (s *S) TearDownTest(c *gocheck.C) {
	s.server.Close()
}

func (s *S) TearDownSuite(c *gocheck.C) {
	ttesting.ClearAllCollections(s.conn.Collection("router_hipache_tests").Database)
}

func (s *S) TestNewGalebClient(c *gocheck.C) {
	client, err := newGalebClient()
	c.Assert(err, gocheck.IsNil)
	c.Assert(client.apiUrl, gocheck.Equals, s.server.URL)
	c.Assert(client.username, gocheck.Equals, "myusername")
	c.Assert(client.password, gocheck.Equals, "mypassword")
}

func (s *S) TestGalebAddBackendPool(c *gocheck.C) {
	s.handler.RspCode = http.StatusCreated
	client, err := newGalebClient()
	c.Assert(err, gocheck.IsNil)
	params := backendPoolParams{}
	err = client.addBackendPool(&params)
	c.Assert(err, gocheck.IsNil)
}
