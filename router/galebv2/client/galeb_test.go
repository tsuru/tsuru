// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/tsuru/tsuru/api/apitest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct {
	server  *httptest.Server
	handler apitest.MultiTestHandler
	client  *GalebClient
}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	s.handler = apitest.MultiTestHandler{
		ConditionalContent: make(map[string]interface{}),
	}
	s.server = httptest.NewServer(&s.handler)
	s.client = &GalebClient{
		ApiUrl:            s.server.URL + "/api",
		Username:          "myusername",
		Password:          "mypassword",
		Environment:       "env1",
		Project:           "proj1",
		BalancePolicy:     "balance1",
		RuleType:          "ruletype1",
		TargetTypeBackend: "targetbackend1",
		TargetTypePool:    "targetpool1",
	}
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Close()
}

func (s *S) TestNewGalebClient(c *check.C) {
	c.Assert(s.client.ApiUrl, check.Equals, s.server.URL+"/api")
	c.Assert(s.client.Username, check.Equals, "myusername")
	c.Assert(s.client.Password, check.Equals, "mypassword")
}

func (s *S) TestGalebAddBackendPool(c *check.C) {
	s.handler.Content = `{
      "_links": {
        "self": {
          "href": "http://galeb.somewhere/api/target/3"
        }
      }
    }`
	s.handler.RspCode = http.StatusCreated
	expected := Target{
		ID:            0,
		Name:          "myname",
		Project:       "proj1",
		Environment:   "env1",
		BalancePolicy: "balance1",
		TargetType:    "targetpool1",
		BackendPool:   "",
		Properties:    BackendPoolProperties{},
	}
	fullId, err := s.client.AddBackendPool("myname")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"POST"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{"/api/target"})
	var parsedParams Target
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, check.IsNil)
	c.Assert(parsedParams, check.DeepEquals, expected)
	c.Assert(s.handler.Header[0].Get("Content-Type"), check.Equals, "application/json")
	c.Assert(fullId, check.Equals, "http://galeb.somewhere/api/target/3")
}

func (s *S) TestGalebAddBackendPoolInvalidStatusCode(c *check.C) {
	s.handler.RspCode = http.StatusOK
	s.handler.Content = "invalid content"
	fullId, err := s.client.AddBackendPool("")
	c.Assert(err, check.ErrorMatches,
		"POST /target: invalid response code: 200: invalid content - PARAMS: .+")
	c.Assert(fullId, check.Equals, "")
}

func (s *S) TestGalebAddBackendPoolInvalidResponse(c *check.C) {
	s.handler.RspCode = http.StatusCreated
	s.handler.Content = "invalid content"
	fullId, err := s.client.AddBackendPool("")
	c.Assert(err, check.ErrorMatches,
		"POST /target: unable to parse response: invalid content: invalid character 'i' looking for beginning of value - PARAMS: .+")
	c.Assert(fullId, check.Equals, "")
}

func (s *S) TestGalebAddBackend(c *check.C) {
	s.handler.ConditionalContent["/api/target/search/findByName?name=mypool"] = `{
		"_embedded": {
			"target": [
				{
					"_links": {
						"self": {
							"href": "http://galeb.somewhere/api/target/9"
						}
					}
				}
			]
		}
	}`
	s.handler.ConditionalContent["/api/target"] = `{
      "_links": {
        "self": {
          "href": "http://galeb.somewhere/api/target/10"
        }
      }
    }`
	s.handler.RspCode = http.StatusCreated
	expected := Target{
		ID:            0,
		Name:          "http://10.0.0.1:8080",
		Project:       "proj1",
		Environment:   "env1",
		BalancePolicy: "balance1",
		TargetType:    "targetbackend1",
		BackendPool:   "http://galeb.somewhere/api/target/9",
		Properties:    BackendPoolProperties{},
	}
	url1, _ := url.Parse("http://10.0.0.1:8080")
	fullId, err := s.client.AddBackend(url1, "mypool")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "POST"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{"/api/target/search/findByName?name=mypool", "/api/target"})
	var parsedParams Target
	err = json.Unmarshal(s.handler.Body[1], &parsedParams)
	c.Assert(err, check.IsNil)
	c.Assert(parsedParams, check.DeepEquals, expected)
	c.Assert(s.handler.Header[1].Get("Content-Type"), check.Equals, "application/json")
	c.Assert(fullId, check.Equals, "http://galeb.somewhere/api/target/10")
}

func (s *S) TestGalebAddVirtualHost(c *check.C) {
	s.handler.RspCode = http.StatusCreated
	s.handler.Content = `{
      "_links": {
        "self": {
          "href": "http://galeb.somewhere/api/virtualhost/999"
        }
      }
    }`
	fullId, err := s.client.AddVirtualHost("myvirtualhost.com")
	c.Assert(err, check.IsNil)
	c.Assert(fullId, check.Equals, "http://galeb.somewhere/api/virtualhost/999")
	var parsedParams VirtualHost
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, check.IsNil)
	expected := VirtualHost{
		ID:          0,
		Name:        "myvirtualhost.com",
		Environment: "env1",
		Project:     "proj1",
	}
	c.Assert(parsedParams, check.DeepEquals, expected)
}

func (s *S) TestGalebAddRuleToID(c *check.C) {
	s.handler.ConditionalContent["/api/rule"] = `{
      "_links": {
        "self": {
          "href": "http://galeb.somewhere/api/rule/8"
        }
      }
    }`
	s.handler.RspCode = http.StatusCreated
	expected := Rule{
		ID:          0,
		Name:        "myrule",
		RuleType:    "ruletype1",
		VirtualHost: "",
		BackendPool: "http://galeb.somewhere/api/target/9",
		Default:     true,
		Order:       0,
		Properties: RuleProperties{
			Match: "/",
		},
	}
	fullId, err := s.client.AddRuleToID("myrule", "http://galeb.somewhere/api/target/9")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"POST"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{"/api/rule"})
	var parsedParams Rule
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, check.IsNil)
	c.Assert(parsedParams, check.DeepEquals, expected)
	c.Assert(s.handler.Header[0].Get("Content-Type"), check.Equals, "application/json")
	c.Assert(fullId, check.Equals, "http://galeb.somewhere/api/rule/8")
}

func (s *S) TestGalebRemoveBackendByID(c *check.C) {
	s.handler.RspCode = http.StatusNoContent
	err := s.client.RemoveBackendByID("/target/mybackendID")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"DELETE"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{"/api/target/mybackendID"})
}

func (s *S) TestGalebRemoveBackendPool(c *check.C) {
	s.handler.ConditionalContent["/api/target/search/findByName?name=mypool"] = []string{
		"200", fmt.Sprintf(`{
		"_embedded": {
			"target": [
				{
					"_links": {
						"self": {
							"href": "%s/target/10"
						}
					}
				}
			]
		}
	}`, s.client.ApiUrl)}
	s.handler.RspCode = http.StatusNoContent
	err := s.client.RemoveBackendPool("mypool")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "DELETE"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{"/api/target/search/findByName?name=mypool", "/api/target/10"})
}

func (s *S) TestGalebRemoveVirtualHost(c *check.C) {
	s.handler.ConditionalContent["/api/virtualhost/search/findByName?name=myvh.com"] = []string{
		"200", fmt.Sprintf(`{
		"_embedded": {
			"virtualhost": [
				{
					"_links": {
						"self": {
							"href": "%s/virtualhost/10"
						}
					}
				}
			]
		}
	}`, s.client.ApiUrl)}
	s.handler.RspCode = http.StatusNoContent
	err := s.client.RemoveVirtualHost("myvh.com")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "DELETE"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{"/api/virtualhost/search/findByName?name=myvh.com", "/api/virtualhost/10"})
}

func (s *S) TestGalebRemoveRuleByID(c *check.C) {
	s.handler.RspCode = http.StatusNoContent
	err := s.client.RemoveRuleByID("/rule/myrule1")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"DELETE"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{"/api/rule/myrule1"})
}

func (s *S) TestGalebRemoveBackendByIDInvalidResponse(c *check.C) {
	s.handler.RspCode = http.StatusOK
	s.handler.Content = "invalid content"
	err := s.client.RemoveBackendByID("/target/11")
	c.Assert(err, check.ErrorMatches, "DELETE /target/11: invalid response code: 200: invalid content")
}

func (s *S) TestGalebSetRuleVirtualHost(c *check.C) {
	s.handler.ConditionalContent["/api/rule/search/findByNameAndParent?name=myrule&parent="] = []string{
		"200", fmt.Sprintf(`{
		"_embedded": {
			"rule": [
				{
					"_links": {
						"self": {
							"href": "%s/rule/1"
						}
					}
				}
			]
		}
	}`, s.client.ApiUrl)}
	s.handler.ConditionalContent["/api/virtualhost/search/findByName?name=myvh"] = []string{
		"200", fmt.Sprintf(`{
		"_embedded": {
			"virtualhost": [
				{
					"_links": {
						"self": {
							"href": "%s/virtualhost/2"
						}
					}
				}
			]
		}
	}`, s.client.ApiUrl)}
	s.handler.RspCode = http.StatusNoContent
	err := s.client.SetRuleVirtualHost("myrule", "myvh")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "GET", "PATCH"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{
		"/api/rule/search/findByNameAndParent?name=myrule&parent=",
		"/api/virtualhost/search/findByName?name=myvh",
		"/api/rule/1",
	})
	expected := Rule{
		VirtualHost: fmt.Sprintf("%s/virtualhost/2", s.client.ApiUrl),
	}
	var parsedParams Rule
	err = json.Unmarshal(s.handler.Body[2], &parsedParams)
	c.Assert(err, check.IsNil)
	c.Assert(parsedParams, check.DeepEquals, expected)
}
