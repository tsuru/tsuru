// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/types/service"
	check "gopkg.in/check.v1"
)

func (s *S) TestServiceBrokerList(c *check.C) {
	brokers := []service.Broker{
		{Name: "broker-1", URL: "http://localhost:8080", Config: service.BrokerConfig{Context: map[string]interface{}{}}},
		{Name: "broker-2", URL: "http://localhost:8080", Config: service.BrokerConfig{Context: map[string]interface{}{}}},
	}
	s.mockService.ServiceBroker.OnList = func() ([]service.Broker, error) {
		return brokers, nil
	}
	request, err := http.NewRequest("GET", "/1.7/brokers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var response map[string][]service.Broker
	err = json.NewDecoder(recorder.Body).Decode(&response)
	c.Assert(err, check.IsNil)
	c.Assert(response["brokers"], check.DeepEquals, brokers)
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
		Config: service.BrokerConfig{
			AuthConfig: &service.AuthConfig{
				BasicAuthConfig: &service.BasicAuthConfig{
					Username: "username",
					Password: "password",
				},
				BearerConfig: &service.BearerConfig{},
			},
			Context: nil,
		},
	}
	s.mockService.ServiceBroker.OnCreate = func(b service.Broker) error {
		c.Assert(b, check.DeepEquals, expectedBroker)
		return nil
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
}

func (s *S) TestServiceBrokerAddWithCache(c *check.C) {
	duration := 5 * time.Minute
	expectedBroker := service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:8080",
		Config: service.BrokerConfig{
			AuthConfig: &service.AuthConfig{
				BasicAuthConfig: &service.BasicAuthConfig{
					Username: "username",
					Password: "password",
				},
				BearerConfig: &service.BearerConfig{},
			},
			Context: nil,
		},
	}
	s.mockService.ServiceBroker.OnCreate = func(b service.Broker) error {
		expectedBroker.Config.CacheExpiration = &duration
		c.Assert(b, check.DeepEquals, expectedBroker)
		return nil
	}
	bodyData, err := form.EncodeToString(expectedBroker)
	c.Assert(err, check.IsNil)
	bodyData = "Config.CacheExpiration=5m&" + bodyData
	request, err := http.NewRequest("POST", "/1.7/brokers", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
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
	s.mockService.ServiceBroker.OnCreate = func(b service.Broker) error {
		c.Assert(b.Name, check.Equals, broker.Name)
		return service.ErrServiceBrokerAlreadyExists
	}
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
		URL:  "https://localhost:9090",
		Config: service.BrokerConfig{
			AuthConfig: &service.AuthConfig{
				BasicAuthConfig: &service.BasicAuthConfig{
					Username: "new-user",
					Password: "password",
				},
				BearerConfig: &service.BearerConfig{},
			},
			Context: nil,
		},
	}
	s.mockService.ServiceBroker.OnUpdate = func(name string, b service.Broker) error {
		c.Assert(name, check.Equals, "broker-name")
		c.Assert(b, check.DeepEquals, broker)
		return nil
	}
	bodyData, err := form.EncodeToString(broker)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("PUT", "/1.7/brokers/broker-name", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestServiceBrokerUpdateWithCache(c *check.C) {
	duration := 2 * time.Hour
	broker := service.Broker{
		Name: "broker-name",
		URL:  "https://localhost:9090",
		Config: service.BrokerConfig{
			AuthConfig: &service.AuthConfig{
				BasicAuthConfig: &service.BasicAuthConfig{
					Username: "new-user",
					Password: "password",
				},
				BearerConfig: &service.BearerConfig{},
			},
			Context: nil,
		},
	}
	s.mockService.ServiceBroker.OnUpdate = func(name string, b service.Broker) error {
		broker.Config.CacheExpiration = &duration
		c.Assert(name, check.Equals, "broker-name")
		c.Assert(b, check.DeepEquals, broker)
		return nil
	}
	bodyData, err := form.EncodeToString(broker)
	c.Assert(err, check.IsNil)
	bodyData = "Config.CacheExpiration=2h&" + bodyData
	request, err := http.NewRequest("PUT", "/1.7/brokers/broker-name", strings.NewReader(bodyData))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestServiceBrokerUpdateNotFound(c *check.C) {
	broker := service.Broker{Name: "not-found"}
	s.mockService.ServiceBroker.OnUpdate = func(name string, b service.Broker) error {
		c.Assert(name, check.Equals, "broker-name")
		c.Assert(b.Name, check.Equals, broker.Name)
		return service.ErrServiceBrokerNotFound
	}
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
	s.mockService.ServiceBroker.OnDelete = func(name string) error {
		c.Assert(name, check.Equals, "broker-name")
		return nil
	}
	request, err := http.NewRequest("DELETE", "/1.7/brokers/broker-name", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestServiceBrokerDeleteNotFound(c *check.C) {
	s.mockService.ServiceBroker.OnDelete = func(name string) error {
		c.Assert(name, check.Equals, "broker-name")
		return service.ErrServiceBrokerNotFound
	}
	request, err := http.NewRequest("DELETE", "/1.7/brokers/broker-name", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestServiceBrokerDeleteUnauthorized(c *check.C) {
	request, err := http.NewRequest("DELETE", "/1.7/brokers/broker-name", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer 12345")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
}
