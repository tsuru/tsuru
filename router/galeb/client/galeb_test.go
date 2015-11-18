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
		RspHeader:          make(http.Header),
	}
	s.server = httptest.NewServer(&s.handler)
	s.client = &GalebClient{
		ApiUrl:        s.server.URL + "/api",
		Username:      "myusername",
		Password:      "mypassword",
		Environment:   "env1",
		Project:       "proj1",
		BalancePolicy: "balance1",
		RuleType:      "ruletype1",
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
	s.handler.ConditionalContent["/api/target/3"] = []string{
		"200", `{"_status": "OK"}`,
	}
	s.handler.RspHeader.Set("Location", fmt.Sprintf("%s/target/3", s.client.ApiUrl))
	s.handler.RspCode = http.StatusCreated
	expected := Target{
		commonPostResponse: commonPostResponse{ID: 0, Name: "myname"},
		Project:            "proj1",
		Environment:        "env1",
	}
	fullId, err := s.client.AddBackendPool("myname")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"POST", "GET"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{"/api/pool", "/api/target/3"})
	var parsedParams Target
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, check.IsNil)
	c.Assert(parsedParams, check.DeepEquals, expected)
	c.Assert(s.handler.Header[0].Get("Content-Type"), check.Equals, "application/json")
	c.Assert(fullId, check.Equals, fmt.Sprintf("%s/target/3", s.client.ApiUrl))
}

func (s *S) TestGalebAddBackendPoolInvalidStatusCode(c *check.C) {
	s.handler.RspCode = http.StatusOK
	s.handler.Content = "invalid content"
	fullId, err := s.client.AddBackendPool("")
	c.Assert(err, check.ErrorMatches,
		"POST /pool: invalid response code: 200: invalid content - PARAMS: .+")
	c.Assert(fullId, check.Equals, "")
}

func (s *S) TestGalebAddBackendPoolInvalidResponse(c *check.C) {
	s.handler.RspCode = http.StatusCreated
	s.handler.Content = "invalid content"
	fullId, err := s.client.AddBackendPool("")
	c.Assert(err, check.ErrorMatches,
		"POST /pool: empty location header. PARAMS: .+")
	c.Assert(fullId, check.Equals, "")
}

func (s *S) TestGalebAddBackend(c *check.C) {
	s.handler.ConditionalContent["/api/target/10"] = []string{
		"200", `{"_status": "OK"}`,
	}
	s.handler.ConditionalContent["/api/pool/search/findByName?name=mypool"] = `{
		"_embedded": {
			"pool": [
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
	s.handler.RspHeader.Set("Location", fmt.Sprintf("%s/target/10", s.client.ApiUrl))
	s.handler.RspCode = http.StatusCreated
	expected := Target{
		commonPostResponse: commonPostResponse{ID: 0, Name: "http://10.0.0.1:8080"},
		Project:            "proj1",
		Environment:        "env1",
		BackendPool:        "http://galeb.somewhere/api/target/9",
	}
	url1, _ := url.Parse("http://10.0.0.1:8080")
	fullId, err := s.client.AddBackend(url1, "mypool")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "POST", "GET"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{
		"/api/pool/search/findByName?name=mypool",
		"/api/target",
		"/api/target/10",
	})
	var parsedParams Target
	err = json.Unmarshal(s.handler.Body[1], &parsedParams)
	c.Assert(err, check.IsNil)
	c.Assert(parsedParams, check.DeepEquals, expected)
	c.Assert(s.handler.Header[1].Get("Content-Type"), check.Equals, "application/json")
	c.Assert(fullId, check.Equals, fmt.Sprintf("%s/target/10", s.client.ApiUrl))
}

func (s *S) TestGalebAddVirtualHost(c *check.C) {
	s.handler.ConditionalContent["/api/virtualhost/999"] = []string{
		"200", `{"_status": "OK"}`,
	}
	s.handler.RspHeader.Set("Location", fmt.Sprintf("%s/virtualhost/999", s.client.ApiUrl))
	s.handler.RspCode = http.StatusCreated
	fullId, err := s.client.AddVirtualHost("myvirtualhost.com")
	c.Assert(err, check.IsNil)
	c.Assert(fullId, check.Equals, fmt.Sprintf("%s/virtualhost/999", s.client.ApiUrl))
	c.Assert(s.handler.Method, check.DeepEquals, []string{"POST", "GET"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{
		"/api/virtualhost",
		"/api/virtualhost/999",
	})
	var parsedParams VirtualHost
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, check.IsNil)
	expected := VirtualHost{
		commonPostResponse: commonPostResponse{ID: 0, Name: "myvirtualhost.com"},
		Environment:        "env1",
		Project:            "proj1",
	}
	c.Assert(parsedParams, check.DeepEquals, expected)
}

func (s *S) TestGalebAddRuleToID(c *check.C) {
	s.handler.RspHeader.Set("Location", "http://galeb.somewhere/api/rule/8")
	s.handler.RspCode = http.StatusCreated
	expected := Rule{
		commonPostResponse: commonPostResponse{ID: 0, Name: "myrule"},
		RuleType:           "ruletype1",
		BackendPool:        "http://galeb.somewhere/api/target/9",
		Default:            true,
		Order:              0,
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
	s.handler.ConditionalContent["/api/pool/search/findByName?name=mypool"] = []string{
		"200", fmt.Sprintf(`{
		"_embedded": {
			"pool": [
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
	c.Assert(s.handler.Url, check.DeepEquals, []string{"/api/pool/search/findByName?name=mypool", "/api/target/10"})
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

func (s *S) TestGalebRemoveRule(c *check.C) {
	s.handler.ConditionalContent["/api/rule/search/findByName?name=myrule"] = []string{
		"200", fmt.Sprintf(`{
		"_embedded": {
			"rule": [
				{
					"_links": {
						"self": {
							"href": "%s/rule/10"
						}
					}
				}
			]
		}
	}`, s.client.ApiUrl)}
	s.handler.RspCode = http.StatusNoContent
	err := s.client.RemoveRule("myrule")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "DELETE"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{"/api/rule/search/findByName?name=myrule", "/api/rule/10"})
}

func (s *S) TestGalebRemoveBackendByIDInvalidResponse(c *check.C) {
	s.handler.RspCode = http.StatusOK
	s.handler.Content = "invalid content"
	err := s.client.RemoveBackendByID("/target/11")
	c.Assert(err, check.ErrorMatches, "DELETE /target/11: invalid response code: 200: invalid content")
}

func (s *S) TestRemoveRuleVirtualHost(c *check.C) {
	s.handler.ConditionalContent["/api/rule/search/findByName?name=myrule"] = []string{
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
	err := s.client.RemoveRuleVirtualHost("myrule", "myvh")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "GET", "DELETE"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{
		"/api/rule/search/findByName?name=myrule",
		"/api/virtualhost/search/findByName?name=myvh",
		"/api/rule/1/parents/2",
	})
}

func (s *S) TestGalebSetRuleVirtualHost(c *check.C) {
	s.handler.ConditionalContent["/api/rule/1"] = []string{
		"200", `{"_status": "OK"}`,
	}
	s.handler.ConditionalContent["/api/rule/search/findByName?name=myrule"] = []string{
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
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "GET", "PATCH", "GET"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{
		"/api/rule/search/findByName?name=myrule",
		"/api/virtualhost/search/findByName?name=myvh",
		"/api/rule/1/parents",
		"/api/rule/1",
	})
	c.Assert(s.handler.Header[2].Get("Content-Type"), check.Equals, "text/uri-list")
	c.Assert(string(s.handler.Body[2]), check.Equals, fmt.Sprintf("%s/virtualhost/2", s.client.ApiUrl))
}

func (s *S) TestFindTargetsByParent(c *check.C) {
	s.handler.ConditionalContent["/api/target/search/findByParentName?name=mypool&size=999999"] = []string{
		"200", `{
		"_embedded": {
			"target": [
				{
					"name": "http://10.0.0.1:1234",
					"_links": {
						"self": {
							"href": "http://galeb.somewhere/api/target/9"
						}
					}
				},
				{
					"name": "http://10.0.0.2:5678",
					"_links": {
						"self": {
							"href": "http://galeb.somewhere/api/target/10"
						}
					}
				}
			]
		}
	}`}
	s.handler.RspCode = http.StatusOK
	targets, err := s.client.FindTargetsByParent("mypool")
	c.Assert(err, check.IsNil)
	c.Assert(targets, check.DeepEquals, []Target{
		{
			commonPostResponse: commonPostResponse{
				Name: "http://10.0.0.1:1234",
				Links: linkData{
					Self: hrefData{Href: "http://galeb.somewhere/api/target/9"},
				},
			},
		},
		{
			commonPostResponse: commonPostResponse{
				Name: "http://10.0.0.2:5678",
				Links: linkData{
					Self: hrefData{Href: "http://galeb.somewhere/api/target/10"},
				},
			},
		},
	})
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{
		"/api/target/search/findByParentName?name=mypool&size=999999",
	})
	c.Assert(s.handler.Header[0].Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestFindVirtualHostsByRule(c *check.C) {
	s.handler.ConditionalContent["/api/rule/search/findByName?name=myrule"] = []string{
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
	s.handler.ConditionalContent["/api/rule/1/parents?size=999999"] = []string{
		"200", `{
		"_embedded": {
			"virtualhost": [
				{
					"name": "myvirtualhost",
					"_links": {
						"self": {
							"href": "http://galeb.somewhere/api/virtualhost/1"
						}
					}
				}
			]
		}
	}`}
	s.handler.RspCode = http.StatusOK
	virtualhosts, err := s.client.FindVirtualHostsByRule("myrule")
	c.Assert(err, check.IsNil)
	c.Assert(virtualhosts, check.DeepEquals, []VirtualHost{
		{
			commonPostResponse: commonPostResponse{
				Name: "myvirtualhost",
				Links: linkData{
					Self: hrefData{Href: "http://galeb.somewhere/api/virtualhost/1"},
				},
			},
		},
	})
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "GET"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{
		"/api/rule/search/findByName?name=myrule",
		"/api/rule/1/parents?size=999999",
	})
}

func (s *S) TestHealthcheck(c *check.C) {
	s.handler.ConditionalContent["/api/healthcheck"] = "WORKING"
	s.handler.RspCode = 200
	err := s.client.Healthcheck()
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET"})
	c.Assert(s.handler.Url, check.DeepEquals, []string{
		"/api/healthcheck",
	})
	c.Assert(s.handler.Header[0].Get("Content-Type"), check.Equals, "application/json")
}
