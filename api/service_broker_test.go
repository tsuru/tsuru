// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/types/service"
	check "gopkg.in/check.v1"
)

func (s *S) TestServiceBrokerList(c *check.C) {
	err := servicemanager.ServiceBroker.Create(service.Broker{
		Name: "broker-1",
		URL:  "http://localhost:8080",
	})
	c.Assert(err, check.IsNil)
	err = servicemanager.ServiceBroker.Create(service.Broker{
		Name: "broker-2",
		URL:  "http://localhost:8080",
	})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.7/brokers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var response map[string][]service.Broker
	err = json.NewDecoder(recorder.Body).Decode(&response)
	c.Assert(err, check.IsNil)
	c.Assert(response["brokers"], check.DeepEquals, []service.Broker{
		{Name: "broker-1", URL: "http://localhost:8080"},
		{Name: "broker-2", URL: "http://localhost:8080"},
	})
}

func (s *S) TestServiceBrokerListEmpty(c *check.C) {
	request, err := http.NewRequest("GET", "/1.7/brokers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestServiceBrokerListUnauthorized(c *check.C) {
	request, err := http.NewRequest("GET", "/1.7/brokers", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
}
