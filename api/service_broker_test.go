// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/ajg/form"
	osb "github.com/pmorie/go-open-service-broker-client/v2"
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

func (s *S) TestServiceBrokerAdd(c *check.C) {
	expectedBroker := service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
		AuthConfig: &osb.AuthConfig{
			BasicAuthConfig: &osb.BasicAuthConfig{
				Username: "username",
				Password: "password",
			},
			BearerConfig: &osb.BearerConfig{},
		},
	}
	bodyData, err := form.EncodeToString(expectedBroker)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/1.7/brokers", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	broker, err := servicemanager.ServiceBroker.Find("broker-name")
	c.Assert(err, check.IsNil)
	c.Assert(broker, check.DeepEquals, expectedBroker)
}

func (s *S) TestServiceBrokerAddUnauthorized(c *check.C) {
	request, err := http.NewRequest("POST", "/1.7/brokers", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
}

func (s *S) TestServiceBrokerAddAlreadyExists(c *check.C) {
	broker := service.Broker{
		Name: "broker-name",
		URL:  "http://localhost:8080",
	}
	err := servicemanager.ServiceBroker.Create(broker)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`name=broker-name&url=https://localhost:8080`)
	request, err := http.NewRequest("POST", "/1.7/brokers", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
}

func (s *S) TestServiceBrokerUpdate(c *check.C) {
	broker := service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
		AuthConfig: &osb.AuthConfig{
			BasicAuthConfig: &osb.BasicAuthConfig{
				Username: "username",
				Password: "password",
			},
			BearerConfig: &osb.BearerConfig{},
		},
	}
	err := servicemanager.ServiceBroker.Create(broker)
	c.Assert(err, check.IsNil)
	broker.URL = "https://localhost:9090"
	broker.AuthConfig.BasicAuthConfig.Username = "new-user"
	bodyData, err := form.EncodeToString(broker)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/1.7/brokers/broker-name", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	newBroker, err := servicemanager.ServiceBroker.Find("broker-name")
	c.Assert(err, check.IsNil)
	c.Assert(newBroker, check.DeepEquals, broker)
}

func (s *S) TestServiceBrokerUpdateNotFound(c *check.C) {
	broker := service.Broker{Name: "not-found"}
	bodyData, err := form.EncodeToString(broker)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/1.7/brokers/broker-name", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestServiceBrokerUpdateUnauthorized(c *check.C) {
	broker := service.Broker{Name: "broker"}
	bodyData, err := form.EncodeToString(broker)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/1.7/brokers/broker-name", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer 12345")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
}

func (s *S) TestServiceBrokerDelete(c *check.C) {
	broker := service.Broker{
		Name: "broker-name",
		URL:  "http://localhost:8080",
	}
	err := servicemanager.ServiceBroker.Create(broker)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.7/brokers/broker-name", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	_, err = servicemanager.ServiceBroker.Find("broker-name")
	c.Assert(err, check.DeepEquals, service.ErrServiceBrokerNotFound)
}

func (s *S) TestServiceBrokerDeleteNotFound(c *check.C) {
	request, err := http.NewRequest("DELETE", "/1.7/brokers/broker-name", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestServiceBrokerDeleteUnauthorized(c *check.C) {
	broker := service.Broker{
		Name: "broker-name",
		URL:  "http://localhost:8080",
	}
	err := servicemanager.ServiceBroker.Create(broker)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.7/brokers/broker-name", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer 12345")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
	foundBroker, err := servicemanager.ServiceBroker.Find("broker-name")
	c.Assert(err, check.IsNil)
	c.Assert(foundBroker.Name, check.DeepEquals, "broker-name")
}
