// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"launchpad.net/gocheck"
	"github.com/tsuru/config"
	tsuruTesting "github.com/tsuru/tsuru/testing"
)

type HealthCheckSuite struct{
	ts 			*httptest.Server
	h  			*tsuruTesting.TestHandler
}

var _ = gocheck.Suite(&HealthCheckSuite{})

func (s *HealthCheckSuite) SetUpSuite(c *gocheck.C) {
	s.h = &tsuruTesting.TestHandler{}
	s.ts = tsuruTesting.StartGandalfTestServer(s.h)
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
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(body, gocheck.DeepEquals, []byte("WORKING"))
}

func (s *HealthCheckSuite) TestHealthCheckMongoAccess(c *gocheck.C) {
	config.Set("database:url", "localhost:34456")
	defer config.Unset("database:url")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck", nil)
	c.Assert(err, gocheck.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusInternalServerError)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(body), gocheck.Equals, "Failed to connect to MongoDB: no reachable servers")
}

func (s *HealthCheckSuite) TestHealthCheckGandalfAccess(c *gocheck.C) {
	config.Set("git:api-server", "localhost:0")
	defer config.Unset("git:api-server")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/healthcheck", nil)
	c.Assert(err, gocheck.IsNil)
	healthcheck(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusInternalServerError)
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(body), gocheck.Equals, "Failed to connect to Gandalf server, it's probably down.")
}
