// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type SchemaSuite struct{}

var _ = gocheck.Suite(&SchemaSuite{})

func (s *SchemaSuite) TestSchemas(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/schema/app", nil)
	c.Assert(err, gocheck.IsNil)
	err = appSchema(recorder, request, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	l := []link{
		{"href": "/apps", "method": "POST", "rel": "create"},
	}
	expected := schema{
		Title:    "app schema",
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
