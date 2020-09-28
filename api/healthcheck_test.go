// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/hc"
	check "gopkg.in/check.v1"
)

type HealthCheckSuite struct{}

var _ = check.Suite(&HealthCheckSuite{})

func (s *HealthCheckSuite) TestHealthCheck(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck", nil)
	c.Assert(err, check.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "WORKING")
}

func (s *HealthCheckSuite) TestHealthCheckWithChecks(c *check.C) {
	hc.AddChecker("mychecker", func(ctx context.Context) error {
		return nil
	})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck?check=all", nil)
	c.Assert(err, check.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, "(?s).*MongoDB: WORKING.*mychecker: WORKING.*")
}

func (s *HealthCheckSuite) TestHealthCheckWithChecksSingleChecker(c *check.C) {
	hc.AddChecker("mychecker", func(context.Context) error {
		return nil
	})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck?check=MongoDB", nil)
	c.Assert(err, check.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, `(?s).*MongoDB: WORKING \([^\s]*\)`+"\n")
}

func (s *HealthCheckSuite) TestHealthCheckWithChecksMultipleChecker(c *check.C) {
	hc.AddChecker("mychecker", func(context.Context) error {
		return nil
	})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck?check=MongoDB&check=mychecker", nil)
	c.Assert(err, check.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Matches, "(?s).*MongoDB: WORKING.*mychecker: WORKING.*")
}

func (s *HealthCheckSuite) TestHealthCheckWithChecksInvalidChecker(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck?check=xxx", nil)
	c.Assert(err, check.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "WORKING")
}
