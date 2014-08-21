// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/auth"
	"launchpad.net/gocheck"
)

func authorizedTsuruHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	fmt.Fprint(w, r.Method)
	return nil
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaGet(c *gocheck.C) {
	RegisterHandler("/foo/bar", "GET", authorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, gocheck.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("GET", gocheck.Equals, string(b))
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaPost(c *gocheck.C) {
	RegisterHandler("/foo/bar", "POST", authorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, gocheck.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("POST", gocheck.Equals, string(b))
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaPut(c *gocheck.C) {
	RegisterHandler("/foo/bar", "PUT", authorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("PUT", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, gocheck.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("PUT", gocheck.Equals, string(b))
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaDelete(c *gocheck.C) {
	RegisterHandler("/foo/bar", "DELETE", authorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("DELETE", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, gocheck.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert("DELETE", gocheck.Equals, string(b))
}

func (s *S) TestIsNotAdmin(c *gocheck.C) {
	RegisterHandler("/foo/bar", "POST", authorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, gocheck.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	b, err := ioutil.ReadAll(rec.Body)
	c.Assert("POST", gocheck.Equals, string(b))
}
