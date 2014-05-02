// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"github.com/tsuru/tsuru/auth"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type ServerSuite struct{}

var _ = gocheck.Suite(&ServerSuite{})

func authorizedTsuruHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	fmt.Fprint(w, "success")
	return nil
}

func (s *ServerSuite) TestRegisterHandlerMakesHandlerAvailableViaGet(c *gocheck.C) {
	RegisterHandler("/foo/bar", "GET", authorizedTsuruHandler)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("GET", gocheck.Equals, string(b))
}

func (s *ServerSuite) TestRegisterHandlerMakesHandlerAvailableViaPost(c *gocheck.C) {
	RegisterHandler("/foo/bar", "POST", authorizedTsuruHandler)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("POST", gocheck.Equals, string(b))
}

func (s *ServerSuite) TestRegisterHandlerMakesHandlerAvailableViaPut(c *gocheck.C) {
	RegisterHandler("/foo/bar", "PUT", authorizedTsuruHandler)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("PUT", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("PUT", gocheck.Equals, string(b))
}

func (s *ServerSuite) TestRegisterHandlerMakesHandlerAvailableViaDelete(c *gocheck.C) {
	RegisterHandler("/foo/bar", "DELETE", authorizedTsuruHandler)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("DELETE", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("DELETE", gocheck.Equals, string(b))
}

func (s *ServerSuite) TestIsNotAdmin(c *gocheck.C) {
	RegisterHandler("/foo/bar", "POST", authorizedTsuruHandler)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert("POST", gocheck.Equals, string(b))
}
