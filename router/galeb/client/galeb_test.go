// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	ttesting "github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct {
	conn    *db.Storage
	server  *httptest.Server
	handler ttesting.MultiTestHandler
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("galeb:username", "myusername")
	config.Set("galeb:password", "mypassword")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) SetUpTest(c *gocheck.C) {
	s.handler = ttesting.MultiTestHandler{}
	s.server = httptest.NewServer(&s.handler)
	config.Set("galeb:api-url", s.server.URL+"/api")
}

func (s *S) TearDownTest(c *gocheck.C) {
	s.server.Close()
}

func (s *S) TearDownSuite(c *gocheck.C) {
	ttesting.ClearAllCollections(s.conn.Collection("router_galeb_client_tests").Database)
}

func (s *S) TestNewGalebClient(c *gocheck.C) {
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	c.Assert(client.apiUrl, gocheck.Equals, s.server.URL+"/api")
	c.Assert(client.username, gocheck.Equals, "myusername")
	c.Assert(client.password, gocheck.Equals, "mypassword")
}

func (s *S) TestGalebAddBackendPool(c *gocheck.C) {
	s.handler.Content = `{
      "_links": {
        "self": "http://galeb.somewhere/api/backendpool/3/"
      },
      "id": 3,
      "name": "pool2",
      "environment": "http://galeb.somewhere/api/environment/1/",
      "farmtype": "http://galeb.somewhere/api/farmtype/1/",
      "plan": "http://galeb.somewhere/api/plan/1/",
      "project": "http://galeb.somewhere/api/project/3/",
      "loadbalancepolicy": "http://galeb.somewhere/api/loadbalancepolicy/1/",
      "status": "201"
    }`
	s.handler.RspCode = http.StatusCreated
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	params := BackendPoolParams{
		Name:              "myname",
		Environment:       "myenv",
		Plan:              "myplan",
		Project:           "myproject",
		LoadBalancePolicy: "mypolicy",
		FarmType:          "mytype",
	}
	fullId, err := client.AddBackendPool(&params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Method, gocheck.DeepEquals, []string{"POST"})
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/backendpool/"})
	var parsedParams BackendPoolParams
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, gocheck.IsNil)
	c.Assert(parsedParams, gocheck.DeepEquals, params)
	c.Assert(s.handler.Header[0].Get("Content-Type"), gocheck.Equals, "application/json")
	c.Assert(fullId, gocheck.Equals, "http://galeb.somewhere/api/backendpool/3/")
}

func (s *S) TestGalebAddBackendPoolInvalidStatusCode(c *gocheck.C) {
	s.handler.RspCode = http.StatusOK
	s.handler.Content = "invalid content"
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	params := BackendPoolParams{}
	fullId, err := client.AddBackendPool(&params)
	c.Assert(err, gocheck.ErrorMatches, "POST /backendpool/: invalid response code: 200: invalid content")
	c.Assert(fullId, gocheck.Equals, "")
}

func (s *S) TestGalebAddBackendPoolInvalidResponse(c *gocheck.C) {
	s.handler.RspCode = http.StatusCreated
	s.handler.Content = "invalid content"
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	params := BackendPoolParams{}
	fullId, err := client.AddBackendPool(&params)
	c.Assert(err, gocheck.ErrorMatches, "POST /backendpool/: unable to parse response: invalid content: invalid character 'i' looking for beginning of value")
	c.Assert(fullId, gocheck.Equals, "")
}

func (s *S) TestGalebAddBackendPoolDefaultValues(c *gocheck.C) {
	config.Set("galeb:environment", "env1")
	config.Set("galeb:farm-type", "type1")
	config.Set("galeb:plan", "plan1")
	config.Set("galeb:project", "project1")
	config.Set("galeb:load-balance-policy", "policy1")
	defer config.Unset("galeb:environment")
	defer config.Unset("galeb:farm-type")
	defer config.Unset("galeb:plan")
	defer config.Unset("galeb:project")
	defer config.Unset("galeb:load-balance-policy")
	s.handler.RspCode = http.StatusCreated
	s.handler.Content = `{
      "_links": {
        "self": "http://galeb.somewhere/api/backendpool/999/"
      }
    }`
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	c.Assert(client.environment, gocheck.Equals, "env1")
	c.Assert(client.farmType, gocheck.Equals, "type1")
	c.Assert(client.plan, gocheck.Equals, "plan1")
	c.Assert(client.project, gocheck.Equals, "project1")
	c.Assert(client.loadBalancePolicy, gocheck.Equals, "policy1")
	params := BackendPoolParams{Name: "mypool"}
	fullId, err := client.AddBackendPool(&params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fullId, gocheck.Equals, "http://galeb.somewhere/api/backendpool/999/")
	var parsedParams BackendPoolParams
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, gocheck.IsNil)
	expected := BackendPoolParams{
		Name:              "mypool",
		Environment:       "env1",
		Plan:              "plan1",
		Project:           "project1",
		LoadBalancePolicy: "policy1",
		FarmType:          "type1",
	}
	c.Assert(parsedParams, gocheck.DeepEquals, expected)
}

func (s *S) TestGalebAddBackend(c *gocheck.C) {
	s.handler.Content = `{
      "_links": {
        "self": "http://galeb.somewhere/api/backend/9/"
      },
      "id": 9,
      "ip": "10.0.0.1",
      "port": 8080,
      "backendpool": "http://galeb.somewhere/api/backendpool/1/",
      "status": "201"
    }`
	s.handler.RspCode = http.StatusCreated
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	params := BackendParams{
		Ip:          "10.0.0.1",
		Port:        8080,
		BackendPool: "http://galeb.somewhere/api/backendpool/1/",
	}
	fullId, err := client.AddBackend(&params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Method, gocheck.DeepEquals, []string{"POST"})
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/backend/"})
	var parsedParams BackendParams
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, gocheck.IsNil)
	c.Assert(parsedParams, gocheck.DeepEquals, params)
	c.Assert(s.handler.Header[0].Get("Content-Type"), gocheck.Equals, "application/json")
	c.Assert(fullId, gocheck.Equals, "http://galeb.somewhere/api/backend/9/")
}

func (s *S) TestGalebAddRuleDefaultValues(c *gocheck.C) {
	config.Set("galeb:rule-type", "rule1")
	config.Set("galeb:project", "project1")
	defer config.Unset("galeb:rule-type")
	defer config.Unset("galeb:project")
	s.handler.RspCode = http.StatusCreated
	s.handler.Content = `{
      "_links": {
        "self": "http://galeb.somewhere/api/rule/999/"
      }
    }`
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	c.Assert(client.ruleType, gocheck.Equals, "rule1")
	c.Assert(client.project, gocheck.Equals, "project1")
	params := RuleParams{
		Name:        "myrule",
		Match:       "/",
		BackendPool: "pool1",
	}
	fullId, err := client.AddRule(&params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fullId, gocheck.Equals, "http://galeb.somewhere/api/rule/999/")
	var parsedParams RuleParams
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, gocheck.IsNil)
	expected := RuleParams{
		Name:        "myrule",
		Match:       "/",
		BackendPool: "pool1",
		RuleType:    "rule1",
		Project:     "project1",
	}
	c.Assert(parsedParams, gocheck.DeepEquals, expected)
}

func (s *S) TestGalebAddVirtualHostDefaultValues(c *gocheck.C) {
	config.Set("galeb:farm-type", "farm1")
	config.Set("galeb:plan", "plan1")
	config.Set("galeb:environment", "env1")
	config.Set("galeb:project", "project1")
	defer config.Unset("galeb:farm-type")
	defer config.Unset("galeb:plan")
	defer config.Unset("galeb:environment")
	defer config.Unset("galeb:project")
	s.handler.RspCode = http.StatusCreated
	s.handler.Content = `{
      "_links": {
        "self": "http://galeb.somewhere/api/virtualhost/999/"
      }
    }`
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	c.Assert(client.farmType, gocheck.Equals, "farm1")
	c.Assert(client.project, gocheck.Equals, "project1")
	c.Assert(client.plan, gocheck.Equals, "plan1")
	c.Assert(client.environment, gocheck.Equals, "env1")
	params := VirtualHostParams{
		Name:        "myvirtualhost.com",
		RuleDefault: "myrule",
	}
	fullId, err := client.AddVirtualHost(&params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fullId, gocheck.Equals, "http://galeb.somewhere/api/virtualhost/999/")
	var parsedParams VirtualHostParams
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, gocheck.IsNil)
	expected := VirtualHostParams{
		Name:        "myvirtualhost.com",
		RuleDefault: "myrule",
		FarmType:    "farm1",
		Plan:        "plan1",
		Environment: "env1",
		Project:     "project1",
	}
	c.Assert(parsedParams, gocheck.DeepEquals, expected)
}

func (s *S) TestGalebAddVirtualHostRule(c *gocheck.C) {
	s.handler.Content = `{
      "_links": {
        "self": "http://galeb.somewhere/api/virtualhostrule/9/"
      },
      "status": "201"
    }`
	s.handler.RspCode = http.StatusCreated
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	params := VirtualHostRuleParams{
		Order:       1,
		Rule:        "rule1",
		VirtualHost: "virtualhost1",
	}
	fullId, err := client.AddVirtualHostRule(&params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Method, gocheck.DeepEquals, []string{"POST"})
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/virtualhostrule/"})
	var parsedParams VirtualHostRuleParams
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, gocheck.IsNil)
	c.Assert(parsedParams, gocheck.DeepEquals, params)
	c.Assert(s.handler.Header[0].Get("Content-Type"), gocheck.Equals, "application/json")
	c.Assert(fullId, gocheck.Equals, "http://galeb.somewhere/api/virtualhostrule/9/")
}

func (s *S) TestGalebRemoveResource(c *gocheck.C) {
	s.handler.RspCode = http.StatusNoContent
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	err = client.RemoveResource(client.apiUrl + "/backendpool/10/")
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Method, gocheck.DeepEquals, []string{"DELETE"})
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/backendpool/10/"})
}

func (s *S) TestGalebRemoveResourceInvalidResponse(c *gocheck.C) {
	s.handler.RspCode = http.StatusOK
	s.handler.Content = "invalid content"
	client, err := NewGalebClient()
	c.Assert(err, gocheck.IsNil)
	err = client.RemoveResource(client.apiUrl + "/backendpool/10/")
	c.Assert(err, gocheck.ErrorMatches, "DELETE /backendpool/10/: invalid response code: 200: invalid content")
}
