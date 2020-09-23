// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tsuru/tsuru/api/apitest"
	check "gopkg.in/check.v1"
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
	s.resetHandler()
	s.server = httptest.NewServer(&s.handler)
	s.client = &GalebClient{
		ApiURL:        s.server.URL + "/api",
		Username:      "myusername",
		Password:      "mypassword",
		Environment:   "env1",
		Project:       "proj1",
		BalancePolicy: "balance1",
		RuleType:      "ruletype1",
		WaitTimeout:   100 * time.Millisecond,
	}
}

func (s *S) resetHandler() {
	s.handler = apitest.MultiTestHandler{
		ConditionalContent: make(map[string]interface{}),
		RspHeader:          make(http.Header),
	}
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Close()
}

func (s *S) TestNewGalebClient(c *check.C) {
	c.Assert(s.client.ApiURL, check.Equals, s.server.URL+"/api")
	c.Assert(s.client.Username, check.Equals, "myusername")
	c.Assert(s.client.Password, check.Equals, "mypassword")
}

func (s *S) TestGalebAuthBasic(c *check.C) {
	rsp, err := s.client.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusOK)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/"})
	c.Assert(s.handler.Header[0].Get("Authorization"), check.Equals, "Basic bXl1c2VybmFtZTpteXBhc3N3b3Jk")
}

func (s *S) TestGalebAuthToken(c *check.C) {
	s.handler.Content = `{"token":"aqwsed"}`
	s.client.UseToken = true
	rsp, err := s.client.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusOK)
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/token", "/api/"})
	c.Assert(s.handler.Header, check.HasLen, 2)
	c.Assert(s.handler.Header[0].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:mypassword")))
	c.Assert(s.handler.Header[1].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:aqwsed")))
	s.resetHandler()
	rsp, err = s.client.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusOK)
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/"})
	c.Assert(s.handler.Header, check.HasLen, 1)
	c.Assert(s.handler.Header[0].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:aqwsed")))
}

func (s *S) TestGalebAuthTokenExpired(c *check.C) {
	s.handler.Content = `{"token":"xyz"}`
	s.client.UseToken = true
	rsp, err := s.client.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusOK)
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/token", "/api/"})
	c.Assert(s.handler.Header, check.HasLen, 2)
	c.Assert(s.handler.Header[0].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:mypassword")))
	c.Assert(s.handler.Header[1].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:xyz")))
	s.resetHandler()
	s.handler.Content = `{"token":"abc"}`
	unauthorizedCount := 0
	s.handler.Hook = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/api/") && unauthorizedCount < 2 {
			unauthorizedCount++
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		return false
	}
	rsp, err = s.client.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusOK)
	c.Assert(unauthorizedCount, check.Equals, 2)
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/", "/api/token", "/api/", "/api/token", "/api/"})
	c.Assert(s.handler.Header, check.HasLen, 5)
	c.Assert(s.handler.Header[0].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:xyz")))
	c.Assert(s.handler.Header[1].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:mypassword")))
	c.Assert(s.handler.Header[2].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:abc")))
	c.Assert(s.handler.Header[3].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:mypassword")))
	c.Assert(s.handler.Header[4].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:abc")))
}

func (s *S) TestGalebAuthTokenExpiredMaxRetries(c *check.C) {
	s.handler.Content = `{"token":"abc"}`
	s.client.UseToken = true
	s.client.token = "xyz"
	s.handler.Hook = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		return false
	}
	rsp, err := s.client.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusUnauthorized)
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/", "/api/token", "/api/", "/api/token", "/api/", "/api/token", "/api/"})
	c.Assert(s.handler.Header, check.HasLen, 7)
	c.Assert(s.handler.Header[0].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:xyz")))
	c.Assert(s.handler.Header[1].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:mypassword")))
	c.Assert(s.handler.Header[2].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:abc")))
	c.Assert(s.handler.Header[3].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:mypassword")))
	c.Assert(s.handler.Header[4].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:abc")))
	c.Assert(s.handler.Header[5].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:mypassword")))
	c.Assert(s.handler.Header[6].Get("Authorization"), check.Equals, "Basic "+
		base64.StdEncoding.EncodeToString([]byte("myusername:abc")))
}

func (s *S) TestGalebGetMaxRetries(c *check.C) {
	s.handler.Hook = func(w http.ResponseWriter, r *http.Request) bool {
		hj, ok := w.(http.Hijacker)
		c.Assert(ok, check.Equals, true)
		conn, _, _ := hj.Hijack()
		conn.Close()
		return true
	}
	_, err := s.client.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.NotNil)
	s.handler.WithLock(func() {
		c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/", "/api/", "/api/", "/api/"})
	})
}

func (s *S) TestGalebPostNoRetry(c *check.C) {
	s.handler.Hook = func(w http.ResponseWriter, r *http.Request) bool {
		hj, ok := w.(http.Hijacker)
		c.Assert(ok, check.Equals, true)
		conn, _, _ := hj.Hijack()
		conn.Close()
		return true
	}
	_, err := s.client.doRequest(context.TODO(), "POST", "/", nil)
	c.Assert(err, check.NotNil)
	s.handler.WithLock(func() {
		c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/"})
	})
}

func (s *S) TestGalebAuthTokenConcurrentRequests(c *check.C) {
	s.handler.Content = `{"token":"xyz"}`
	s.client.UseToken = true
	nConcurrent := 50
	wg := sync.WaitGroup{}
	for i := 0; i < nConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.client.doRequest(context.TODO(), "GET", "/", nil)
			c.Assert(err, check.IsNil)
		}()
	}
	wg.Wait()
	c.Assert(len(s.handler.URL) >= nConcurrent+1, check.Equals, true, check.Commentf("%d: %#v", len(s.handler.URL), s.handler.URL))
	for i, url := range s.handler.URL {
		if url == "/api/" {
			c.Assert(s.handler.Header[i].Get("Authorization"), check.Equals, "Basic "+
				base64.StdEncoding.EncodeToString([]byte("myusername:xyz")))
		}
	}
}

func (s *S) TestGalebAddBackendPool(c *check.C) {
	s.handler.ConditionalContent["/api/target/3"] = []string{
		"200", `{"status": {"1": "OK"}}`,
	}
	s.handler.RspHeader.Set("Location", fmt.Sprintf("%s/target/3", s.client.ApiURL))
	s.handler.RspCode = http.StatusCreated
	expected := Target{
		commonPostResponse: commonPostResponse{ID: 0, Name: "myname"},
	}
	fullId, err := s.client.AddBackendPool(context.TODO(), "myname", true)
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"POST", "GET"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/pool", "/api/target/3"})
	var parsedParams Target
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, check.IsNil)
	c.Assert(parsedParams, check.DeepEquals, expected)
	c.Assert(s.handler.Header[0].Get("Content-Type"), check.Equals, "application/json")
	c.Assert(fullId, check.Equals, fmt.Sprintf("%s/target/3", s.client.ApiURL))
}

func (s *S) TestGalebAddBackendPoolInvalidStatusCode(c *check.C) {
	s.handler.RspCode = http.StatusOK
	s.handler.Content = "invalid content"
	fullId, err := s.client.AddBackendPool(context.TODO(), "", true)
	c.Assert(err, check.ErrorMatches,
		"POST /pool: invalid response code: 200: invalid content - PARAMS: .+")
	c.Assert(fullId, check.Equals, "")
}

func (s *S) TestGalebAddBackendPoolInvalidResponse(c *check.C) {
	s.handler.RspCode = http.StatusCreated
	s.handler.Content = "invalid content"
	fullId, err := s.client.AddBackendPool(context.TODO(), "", true)
	c.Assert(err, check.ErrorMatches,
		"POST /pool: empty location header. PARAMS: .+")
	c.Assert(fullId, check.Equals, "")
}

func (s *S) TestUpdatePoolPropertiesNoChanges(c *check.C) {
	s.handler.ConditionalContent["/api/pool/search/findByName?name=mypool"] = fmt.Sprintf(`{
		"_embedded": {
			"pool": [
				{
					"_links": {
						"self": {
							"href": "%s/pool/22"
						}
					}
				}
			]
		}
	}`, s.client.ApiURL)
	s.handler.ConditionalContent["/api/pool/22"] = `{
		"id": 22,
		"_lastmodified_by": "system",
		"hc_path": "/",
		"hc_http_status_code": "200",
		"hc_body": ""
	}`
	props := BackendPoolHealthCheck{
		HcPath:           "/",
		HcBody:           "",
		HcHTTPStatusCode: "200",
	}
	err := s.client.UpdatePoolProperties(context.TODO(), "mypool", props)
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "GET"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{
		"/api/pool/search/findByName?name=mypool",
		"/api/pool/22",
	})
}

func (s *S) TestUpdatePoolProperties(c *check.C) {
	s.handler.Hook = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.String(), "/api/pool/search/findByName?name=mypool") {
			data := fmt.Sprintf(`{
				"_embedded": {
					"pool": [
						{
							"_links": {
								"self": {
									"href": "%s/pool/22"
								}
							}
						}
					]
				}
			}`, s.client.ApiURL)
			w.Write([]byte(data))
			return true
		}
		if strings.HasSuffix(r.URL.Path, "/api/pool/22") {
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(&commonPostResponse{Status: map[string]string{"": STATUS_OK}})
			}
			if r.Method == http.MethodPatch {
				w.WriteHeader(http.StatusNoContent)
			}
			return true
		}
		return false
	}
	props := BackendPoolHealthCheck{
		HcPath:           "/",
		HcBody:           "",
		HcHTTPStatusCode: "200",
	}
	err := s.client.UpdatePoolProperties(context.TODO(), "mypool", props)
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "GET", "PATCH", "GET"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{
		"/api/pool/search/findByName?name=mypool",
		"/api/pool/22",
		"/api/pool/22",
		"/api/pool/22",
	})
}

func (s *S) TestGalebAddVirtualHost(c *check.C) {
	s.handler.ConditionalContent["/api/virtualhost/999"] = []string{
		"200", `{"_status": "OK"}`,
	}
	s.handler.RspHeader.Set("Location", fmt.Sprintf("%s/virtualhost/999", s.client.ApiURL))
	s.handler.RspCode = http.StatusCreated
	fullId, err := s.client.AddVirtualHost(context.TODO(), "myvirtualhost.com", true)
	c.Assert(err, check.IsNil)
	c.Assert(fullId, check.Equals, fmt.Sprintf("%s/virtualhost/999", s.client.ApiURL))
	c.Assert(s.handler.Method, check.DeepEquals, []string{"POST", "GET"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{
		"/api/virtualhost",
		"/api/virtualhost/999",
	})
	var parsedParams VirtualHost
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, check.IsNil)
	expected := VirtualHost{
		commonPostResponse: commonPostResponse{ID: 0, Name: "myvirtualhost.com"},
		Environment:        []string{"env1"},
		Project:            "proj1",
	}
	c.Assert(parsedParams, check.DeepEquals, expected)
}

func (s *S) TestGalebAddRuleToID(c *check.C) {
	s.handler.RspHeader.Set("Location", "http://galeb.somewhere/api/rule/8")
	s.handler.RspCode = http.StatusCreated
	expected := Rule{
		commonPostResponse: commonPostResponse{ID: 0, Name: "myrule"},
		BackendPool:        []string{"http://galeb.somewhere/api/target/9"},
		Matching:           "/",
		Project:            "proj1",
	}
	fullId, err := s.client.addRuleToID(context.TODO(), "myrule", "http://galeb.somewhere/api/target/9")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"POST"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/rule"})
	var parsedParams Rule
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, check.IsNil)
	c.Assert(parsedParams, check.DeepEquals, expected)
	c.Assert(s.handler.Header[0].Get("Content-Type"), check.Equals, "application/json")
	c.Assert(fullId, check.Equals, "http://galeb.somewhere/api/rule/8")
}

func (s *S) TestGalebAddRuleToPool(c *check.C) {
	s.handler.RspHeader.Set("Location", "http://galeb.somewhere/api/rule/8")
	s.handler.RspCode = http.StatusCreated
	s.handler.ConditionalContent["/api/pool/search/findByName?name=mypool"] = []string{
		"200", `{
		"_embedded": {
			"pool": [
				{
					"_links": {
						"self": {
							"href": "http://galeb.somewhere/api/pool/9"
						}
					}
				}
			]
		}
	}`}
	expected := Rule{
		commonPostResponse: commonPostResponse{ID: 0, Name: "myrule"},
		BackendPool:        []string{"http://galeb.somewhere/api/pool/9"},
		Matching:           "/",
		Project:            "proj1",
	}
	fullId, err := s.client.AddRuleToPool(context.TODO(), "myrule", "mypool")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "POST"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/pool/search/findByName?name=mypool", "/api/rule"})
	var parsedParams Rule
	err = json.Unmarshal(s.handler.Body[1], &parsedParams)
	c.Assert(err, check.IsNil)
	c.Assert(parsedParams, check.DeepEquals, expected)
	c.Assert(s.handler.Header[1].Get("Content-Type"), check.Equals, "application/json")
	c.Assert(fullId, check.Equals, "http://galeb.somewhere/api/rule/8")
}

func (s *S) TestGalebRemoveBackendByID(c *check.C) {
	s.handler.ConditionalContent["GET /api/target/mybackendID"] = []string{
		"200", `{"_status": "OK"}`,
	}
	s.handler.ConditionalContent["DELETE /api/target/mybackendID"] = []string{
		"204", "",
	}
	s.handler.RspCode = http.StatusNoContent
	err := s.client.RemoveResourceByID(context.TODO(), "/target/mybackendID")
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"DELETE", "GET"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/target/mybackendID", "/api/target/mybackendID"})
}

func (s *S) TestGalebRemoveBackendByIDError(c *check.C) {
	s.handler.RspCode = http.StatusBadRequest
	err := s.client.RemoveResourceByID(context.TODO(), "/target/mybackendID")
	c.Assert(err, check.ErrorMatches, `.*invalid response code: 400.*`)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"DELETE"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/target/mybackendID"})
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
	}`, s.client.ApiURL)}
	_ = s.client.RemoveBackendPool(context.TODO(), "mypool")
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "DELETE"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/pool/search/findByName?name=mypool", "/api/target/10"})
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
	}`, s.client.ApiURL)}
	_ = s.client.RemoveVirtualHost(context.TODO(), "myvh.com")
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "DELETE"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/virtualhost/search/findByName?name=myvh.com", "/api/virtualhost/10"})
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
	}`, s.client.ApiURL)}
	_ = s.client.RemoveRule(context.TODO(), "myrule")
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "DELETE"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/rule/search/findByName?name=myrule", "/api/rule/10"})
}

func (s *S) TestGalebRemoveBackendByIDInvalidResponse(c *check.C) {
	s.handler.RspCode = http.StatusOK
	_ = s.client.RemoveResourceByID(context.TODO(), "/target/11")
	c.Assert(s.handler.Method, check.DeepEquals, []string{"DELETE"})
}

func (s *S) TestGalebSetRuleVirtualHost(c *check.C) {
	s.handler.ConditionalContent["/api/rule/1"] = []string{
		"200", `{"status": {"1": "OK"}}`,
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
	}`, s.client.ApiURL)}
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
	}`, s.client.ApiURL)}
	_ = s.client.SetRuleVirtualHost(context.TODO(), "myrule", "myvh", true)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "GET", "GET"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{
		"/api/rule/search/findByName?name=myrule",
		"/api/virtualhost/search/findByName?name=myvh",
		"/api/virtualhost/2/virtualhostgroup",
	})
}

func (s *S) TestFindTargetsByPool(c *check.C) {
	s.handler.ConditionalContent["/api/pool/search/findByName?name=mypool"] = []string{
		"200", fmt.Sprintf(`{
		"_embedded": {
			"pool": [
				{
					"id": 7,
					"_links": {
						"self": {
							"href": "%s/pool/7"
						}
					}
				}
			]
		}
	}`, s.client.ApiURL)}
	s.handler.ConditionalContent["/api/pool/7/targets"] = []string{
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
	targets, err := s.client.FindTargetsByPool(context.TODO(), "mypool")
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
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "GET"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{
		"/api/pool/search/findByName?name=mypool",
		"/api/pool/7/targets",
	})
}

func (s *S) TestHealthcheck(c *check.C) {
	s.handler.ConditionalContent["/api/healthcheck"] = "WORKING"
	s.handler.RspCode = 200
	err := s.client.Healthcheck(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{
		"/api/healthcheck",
	})
	c.Assert(s.handler.Header[0].Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestGalebAddBackendPoolPendingTimeout(c *check.C) {
	s.handler.ConditionalContent["/api/target/3"] = []string{
		"200", `{"status": {"1": "PENDING"}}`,
	}
	s.handler.RspHeader.Set("Location", fmt.Sprintf("%s/target/3", s.client.ApiURL))
	s.handler.RspCode = http.StatusCreated
	expected := Target{
		commonPostResponse: commonPostResponse{ID: 0, Name: "myname"},
	}
	_, err := s.client.AddBackendPool(context.TODO(), "myname", true)
	c.Assert(err, check.ErrorMatches, `GET /target/3: timeout after [0-9]+ms waiting for status change from {"1":"PENDING"}`)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"POST", "GET", "GET"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{"/api/pool", "/api/target/3", "/api/target/3"})
	var parsedParams Target
	err = json.Unmarshal(s.handler.Body[0], &parsedParams)
	c.Assert(err, check.IsNil)
	c.Assert(parsedParams, check.DeepEquals, expected)
	c.Assert(s.handler.Header[0].Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestGalebAddBackends(c *check.C) {
	s.handler.ConditionalContent["/api/target/10"] = []string{
		"200", `{"_status": "OK"}`,
	}
	s.handler.ConditionalContent["/api/pool/search/findByName?name=mypool"] = []string{"200", `{
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
	}`}
	s.handler.RspHeader.Set("Location", fmt.Sprintf("%s/target/10", s.client.ApiURL))
	s.handler.RspCode = http.StatusCreated
	url1, _ := url.Parse("http://10.0.0.1:8080")
	url2, _ := url.Parse("http://10.0.0.2:8080")
	err := s.client.AddBackends(context.TODO(), []*url.URL{
		url1,
		url2,
	}, "mypool", true)
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.HasLen, 5)
	c.Assert(s.handler.Method[0], check.Equals, "GET")
	c.Assert(s.handler.URL, check.HasLen, 5)
	c.Assert(s.handler.URL[0], check.Equals, "/api/pool/search/findByName?name=mypool")
}

func (s *S) TestGalebAddBackendsWithMaxRequests(c *check.C) {
	s.client.MaxRequests = 1
	s.handler.ConditionalContent["/api/target/10"] = []string{
		"200", `{"_status": "OK"}`,
	}
	s.handler.ConditionalContent["/api/pool/search/findByName?name=mypool"] = []string{"200", `{
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
	}`}
	s.handler.RspHeader.Set("Location", fmt.Sprintf("%s/target/10", s.client.ApiURL))
	s.handler.RspCode = http.StatusCreated
	url1, _ := url.Parse("http://10.0.0.1:8080")
	url2, _ := url.Parse("http://10.0.0.2:8080")
	err := s.client.AddBackends(context.TODO(), []*url.URL{
		url1,
		url2,
	}, "mypool", true)
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.HasLen, 5)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "POST", "GET", "POST", "GET"})
	c.Assert(s.handler.URL, check.HasLen, 5)
	c.Assert(s.handler.URL[0], check.Equals, "/api/pool/search/findByName?name=mypool")
}

func (s *S) TestGalebAddBackendsNoWait(c *check.C) {
	s.handler.ConditionalContent["/api/pool/search/findByName?name=mypool"] = []string{"200", `{
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
	}`}
	s.handler.RspHeader.Set("Location", fmt.Sprintf("%s/target/10", s.client.ApiURL))
	s.handler.RspCode = http.StatusCreated
	url1, _ := url.Parse("http://10.0.0.1:8080")
	err := s.client.AddBackends(context.TODO(), []*url.URL{
		url1,
	}, "mypool", false)
	c.Assert(err, check.IsNil)
	c.Assert(s.handler.Method, check.DeepEquals, []string{"GET", "POST"})
	c.Assert(s.handler.URL, check.DeepEquals, []string{
		"/api/pool/search/findByName?name=mypool",
		"/api/target",
	})
}
