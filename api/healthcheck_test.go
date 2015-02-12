// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/httptest"

	"gopkg.in/check.v1"
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
