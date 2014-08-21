// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/config"
	"launchpad.net/gocheck"
)

type SchemaSuite struct{}

var _ = gocheck.Suite(&SchemaSuite{})

func (s *SchemaSuite) TestAppSchema(c *gocheck.C) {
	config.Set("host", "http://myhost.com")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/schema/app", nil)
	c.Assert(err, gocheck.IsNil)
	err = appSchema(recorder, request, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	l := []link{
		{"href": "http://myhost.com/apps/{name}/log", "method": "GET", "rel": "log"},
		{"href": "http://myhost.com/apps/{name}/env", "method": "GET", "rel": "get_env"},
		{"href": "http://myhost.com/apps/{name}/env", "method": "POST", "rel": "set_env"},
		{"href": "http://myhost.com/apps/{name}/env", "method": "DELETE", "rel": "unset_env"},
		{"href": "http://myhost.com/apps/{name}/restart", "method": "GET", "rel": "restart"},
		{"href": "http://myhost.com/apps/{name}", "method": "POST", "rel": "update"},
		{"href": "http://myhost.com/apps/{name}", "method": "DELETE", "rel": "delete"},
		{"href": "http://myhost.com/apps/{name}/run", "method": "POST", "rel": "run"},
	}
	expected := schema{
		Title:    "app schema",
		Type:     "object",
		Links:    l,
		Required: []string{"platform", "name"},
		Properties: map[string]property{
			"name": {
				"type": "string",
			},
			"platform": {
				"type": "string",
			},
			"ip": {
				"type": "string",
			},
			"cname": {
				"type": "string",
			},
		},
	}
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	result := schema{}
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, expected)
}

func (s *SchemaSuite) TestServiceSchema(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/schema/service", nil)
	c.Assert(err, gocheck.IsNil)
	err = serviceSchema(recorder, request, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	expected := schema{
		Title:    "service",
		Type:     "object",
		Required: []string{"name"},
		Properties: map[string]property{
			"name": {
				"type": "string",
			},
			"endpoint": {
				"type": "string",
			},
			"status": {
				"type": "string",
			},
			"doc": {
				"type": "string",
			},
		},
	}
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	result := schema{}
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, expected)
}

func (s *SchemaSuite) TestServicesSchema(c *gocheck.C) {
	config.Set("host", "http://myhost.com")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/schema/services", nil)
	c.Assert(err, gocheck.IsNil)
	err = servicesSchema(recorder, request, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	expected := schema{
		Title: "service collection",
		Type:  "array",
		Items: &schema{
			Ref: "http://myhost.com/schema/service",
		},
	}
	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	result := schema{}
	err = json.Unmarshal(body, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, expected)
}
