// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/api/apitest"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
)

type HealthCheckSuite struct {
	ts *httptest.Server
	h  *apitest.TestHandler
}

var _ = check.Suite(&HealthCheckSuite{})

func (s *HealthCheckSuite) SetUpSuite(c *check.C) {
	s.h = &apitest.TestHandler{}
	s.ts = repositorytest.StartGandalfTestServer(s.h)
}

func (s *HealthCheckSuite) TearDownSuite(c *check.C) {
	s.ts.Close()
}

func (s *HealthCheckSuite) TestHealthCheck(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck", nil)
	c.Assert(err, check.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "WORKING")
}
