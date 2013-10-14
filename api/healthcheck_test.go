// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type HealthCheckSuite struct{}

var _ = gocheck.Suite(&HealthCheckSuite{})

func (s *HealthCheckSuite) TestHealthCheck(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck", nil)
	c.Assert(err, gocheck.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(body, gocheck.DeepEquals, []byte("WORKING"))
}
