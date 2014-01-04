// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type AdminApiSuite struct{}

var _ = gocheck.Suite(&AdminApiSuite{})

type testH struct{}

func (h testH) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(r.Method))
}

func (s *AdminApiSuite) TestRegisterHandlerMakesHandlerAvailableViaGet(c *gocheck.C) {
	h := testH{}
	RegisterHandler("/foo/bar", "GET", h)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("GET", gocheck.Equals, string(b))
}

func (s *AdminApiSuite) TestRegisterHandlerMakesHandlerAvailableViaPost(c *gocheck.C) {
	h := testH{}
	RegisterHandler("/foo/bar", "POST", h)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("POST", gocheck.Equals, string(b))
}

func (s *AdminApiSuite) TestRegisterHandlerMakesHandlerAvailableViaPut(c *gocheck.C) {
	h := testH{}
	RegisterHandler("/foo/bar", "PUT", h)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("PUT", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("PUT", gocheck.Equals, string(b))
}

func (s *AdminApiSuite) TestRegisterHandlerMakesHandlerAvailableViaDelete(c *gocheck.C) {
	h := testH{}
	RegisterHandler("/foo/bar", "DELETE", h)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("DELETE", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("DELETE", gocheck.Equals, string(b))
}
