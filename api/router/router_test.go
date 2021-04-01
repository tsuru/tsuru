// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuru/tsuru/api/context"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func runDelayedHandler(w http.ResponseWriter, r *http.Request) {
	h := context.GetDelayedHandler(r)
	if h != nil {
		h.ServeHTTP(w, r)
	}
}

func (s *S) TestVersion(c *check.C) {
	router := NewRouter()
	var version string
	router.Add("1.0", "GET", "/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version = "1.0"
	}))
	router.Add("1.1", "GET", "/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version = "1.1"
	}))
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/1.1/dream/tel'aran'rhiod", nil)
	c.Assert(err, check.IsNil)
	router.ServeHTTP(recorder, request)
	runDelayedHandler(recorder, request)
	c.Assert(request.URL.Query().Get(":world"), check.Equals, "tel'aran'rhiod")
	c.Assert(version, check.Equals, "1.1")
	request, err = http.NewRequest("GET", "/1.0/dream/tel'aran'rhiod", nil)
	c.Assert(err, check.IsNil)
	router.ServeHTTP(recorder, request)
	runDelayedHandler(recorder, request)
	c.Assert(request.URL.Query().Get(":world"), check.Equals, "tel'aran'rhiod")
	c.Assert(version, check.Equals, "1.0")
}

func (s *S) TestVersionAddAll(c *check.C) {
	router := NewRouter()
	var version string
	router.AddAll("1.0", "/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version = "1.0"
	}))
	router.AddAll("1.1", "/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version = "1.1"
	}))
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/1.1/dream/tel'aran'rhiod", nil)
	c.Assert(err, check.IsNil)
	router.ServeHTTP(recorder, request)
	runDelayedHandler(recorder, request)
	c.Assert(request.URL.Query().Get(":world"), check.Equals, "tel'aran'rhiod")
	c.Assert(version, check.Equals, "1.1")
	request, err = http.NewRequest("DELETE", "/1.0/dream/tel'aran'rhiod", nil)
	c.Assert(err, check.IsNil)
	router.ServeHTTP(recorder, request)
	runDelayedHandler(recorder, request)
	c.Assert(request.URL.Query().Get(":world"), check.Equals, "tel'aran'rhiod")
	c.Assert(version, check.Equals, "1.0")
}

func (s *S) TestDelayedRouter(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/dream/tel'aran'rhiod", nil)
	c.Assert(err, check.IsNil)
	router := NewRouter()
	called := false
	router.Add("1.0", "GET", "/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	router.ServeHTTP(recorder, request)
	c.Assert(called, check.Equals, false)
	c.Assert(request.URL.Query().Get(":world"), check.Equals, "tel'aran'rhiod")
	runDelayedHandler(recorder, request)
	c.Assert(called, check.Equals, true)
}

func (s *S) TestDelayedRouterAddAll(c *check.C) {
	router := NewRouter()
	called := false
	router.AddAll("1.0", "/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
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

func (s *S) TestDelayedRouterAddNamed(c *check.C) {
	router := NewRouter()
	called := false
	router.AddNamed("my-route", "1.0", "GET", "/dream/{world}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	for _, prefix := range []string{"/", "/1.0/"} {
		recorder := httptest.NewRecorder()
		request, err := http.NewRequest("GET", fmt.Sprintf("%vdream/tel'aran'rhiod", prefix), nil)
		c.Assert(err, check.IsNil)
		router.ServeHTTP(recorder, request)
		c.Assert(called, check.Equals, false)
		c.Assert(request.URL.Query().Get(":world"), check.Equals, "tel'aran'rhiod")
		c.Assert(request.URL.Query().Get(":mux-route-name"), check.Equals, "my-route")
		runDelayedHandler(recorder, request)
		c.Assert(called, check.Equals, true)
		called = false
	}
	namedRoute := router.mux.Get("my-route")
	c.Assert(namedRoute, check.NotNil)
	tpl, err := namedRoute.GetPathTemplate()
	c.Assert(err, check.IsNil)
	c.Assert(tpl, check.Equals, "/{version:[0-9.]+}/dream/{world}")
}
