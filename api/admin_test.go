// Copyright 2014 tsuru authors. All rights reserved.
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

func (s *AdminApiSuite) TestRegisterAdminHandlerMakesHandlerAvailableViaGet(c *gocheck.C) {
	h := testH{}
	RegisterAdminHandler("/foo/bar", "GET", h)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("GET", gocheck.Equals, string(b))
}

func (s *AdminApiSuite) TestRegisterAdminHandlerMakesHandlerAvailableViaPost(c *gocheck.C) {
	h := testH{}
	RegisterAdminHandler("/foo/bar", "POST", h)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("POST", gocheck.Equals, string(b))
}

func (s *AdminApiSuite) TestRegisterAdminHandlerMakesHandlerAvailableViaPut(c *gocheck.C) {
	h := testH{}
	RegisterAdminHandler("/foo/bar", "PUT", h)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("PUT", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("PUT", gocheck.Equals, string(b))
}

func (s *AdminApiSuite) TestRegisterAdminHandlerMakesHandlerAvailableViaDelete(c *gocheck.C) {
	h := testH{}
	RegisterAdminHandler("/foo/bar", "DELETE", h)
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("DELETE", "http://example.com/foo/bar", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("DELETE", gocheck.Equals, string(b))
}
