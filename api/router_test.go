// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/httptest"

	"gopkg.in/check.v1"
)

func (s *S) TestDelayedRouter(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/dream/tel'aran'rhiod", nil)
	c.Assert(err, check.IsNil)
	router := &delayedRouter{}
	called := false
	router.Add("GET", "/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	router.ServeHTTP(recorder, request)
	c.Assert(called, check.Equals, false)
	c.Assert(request.URL.Query().Get(":world"), check.Equals, "tel'aran'rhiod")
	runDelayedHandler(recorder, request)
	c.Assert(called, check.Equals, true)
}

func (s *S) TestDelayedRouterAddAll(c *check.C) {
	router := &delayedRouter{}
	called := false
	router.AddAll("/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		recorder := httptest.NewRecorder()
		request, err := http.NewRequest(method, "/dream/tel'aran'rhiod", nil)
		c.Assert(err, check.IsNil)
		router.ServeHTTP(recorder, request)
		c.Assert(called, check.Equals, false)
		c.Assert(request.URL.Query().Get(":world"), check.Equals, "tel'aran'rhiod")
		runDelayedHandler(recorder, request)
		c.Assert(called, check.Equals, true)
		called = false
	}
}
