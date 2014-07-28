// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestDelayedRouter(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/dream/tel'aran'rhiod", nil)
	c.Assert(err, gocheck.IsNil)
	router := &delayedRouter{}
	called := false
	router.Add("GET", "/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	router.ServeHTTP(recorder, request)
	c.Assert(called, gocheck.Equals, false)
	c.Assert(request.URL.Query().Get(":world"), gocheck.Equals, "tel'aran'rhiod")
	runDelayedHandler(recorder, request)
	c.Assert(called, gocheck.Equals, true)
}

func (s *S) TestDelayedRouterAddAll(c *gocheck.C) {
	router := &delayedRouter{}
	called := false
	router.AddAll("/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		recorder := httptest.NewRecorder()
		request, err := http.NewRequest(method, "/dream/tel'aran'rhiod", nil)
		c.Assert(err, gocheck.IsNil)
		router.ServeHTTP(recorder, request)
		c.Assert(called, gocheck.Equals, false)
		c.Assert(request.URL.Query().Get(":world"), gocheck.Equals, "tel'aran'rhiod")
		runDelayedHandler(recorder, request)
		c.Assert(called, gocheck.Equals, true)
		called = false
	}
}
