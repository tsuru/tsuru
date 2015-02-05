// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/repository/repositorytest"
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
	s.ts = repositorytest.StartGandalfTestServer(s.h)
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
