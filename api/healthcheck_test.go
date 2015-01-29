// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

type HealthCheckSuite struct {
	ts *httptest.Server
	h  *testing.TestHandler
}

var _ = gocheck.Suite(&HealthCheckSuite{})

func (s *HealthCheckSuite) SetUpSuite(c *gocheck.C) {
	s.h = &testing.TestHandler{}
	s.ts = testing.StartGandalfTestServer(s.h)
}

func (s *HealthCheckSuite) TearDownSuite(c *gocheck.C) {
	s.ts.Close()
}

func (s *HealthCheckSuite) TestHealthCheck(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck", nil)
	c.Assert(err, gocheck.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), gocheck.Equals, "WORKING")
}

func (s *HealthCheckSuite) TestFullHealthCheck(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck?check=all", nil)
	c.Assert(err, gocheck.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	expected := `MongoDB: WORKING
Gandalf: WORKING
`
	c.Assert(recorder.Body.String(), gocheck.Equals, expected)
}

func (s *HealthCheckSuite) TestFullHealthCheckMongoAccess(c *gocheck.C) {
	config.Set("database:url", "localhost:34456")
	defer config.Unset("database:url")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck?check=all", nil)
	c.Assert(err, gocheck.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusInternalServerError)
	expected := "MongoDB: failed to connect - no reachable servers\n"
	c.Assert(recorder.Body.String(), gocheck.Equals, expected)
}

func (s *HealthCheckSuite) TestFullHealthCheckGandalfAccess(c *gocheck.C) {
	config.Set("git:api-server", "localhost:0")
	defer config.Unset("git:api-server")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck?check=all", nil)
	c.Assert(err, gocheck.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusInternalServerError)
	expected := `MongoDB: WORKING
Gandalf: Failed to connect to Gandalf server, it's probably down.
`
	c.Assert(recorder.Body.String(), gocheck.Equals, expected)
}
