// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/auth"
	"gopkg.in/check.v1"
)

func authorizedTsuruHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	fmt.Fprint(w, r.Method)
	return nil
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaGet(c *check.C) {
	RegisterHandler("/foo/bar", "GET", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("GET", check.Equals, rec.Body.String())
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaPost(c *check.C) {
	RegisterHandler("/foo/bar", "POST", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("POST", check.Equals, rec.Body.String())
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaPut(c *check.C) {
	RegisterHandler("/foo/bar", "PUT", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("PUT", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("PUT", check.Equals, rec.Body.String())
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaDelete(c *check.C) {
	RegisterHandler("/foo/bar", "DELETE", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("DELETE", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("DELETE", check.Equals, rec.Body.String())
}

func (s *S) TestIsNotAdmin(c *check.C) {
	RegisterHandler("/foo/bar", "POST", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("POST", check.Equals, rec.Body.String())
}
