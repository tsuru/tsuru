// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
)

// schema represents a json schema.
type schema struct {
	Title      string              `json:"title"`
	Properties map[string]property `json:"properties"`
	Required   []string            `json:"required"`
	Links      []link              `json:"links"`
	Type       string              `json:"type"`
	Items      *schema             `json:"items"`
	Ref        string              `json:"$ref"`
}

// link represents a json schema link.
type link map[string]string

// property represents a json schema property.
type property map[string]interface{}

// appSchema returns a json schema for app.
func appSchema(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	host, err := config.GetString("host")
	if err != nil {
		return err
	}
	l := []link{
		{"href": host + "/apps/{name}/log", "method": "GET", "rel": "log"},
		{"href": host + "/apps/{name}/env", "method": "GET", "rel": "get_env"},
		{"href": host + "/apps/{name}/env", "method": "POST", "rel": "set_env"},
		{"href": host + "/apps/{name}/env", "method": "DELETE", "rel": "unset_env"},
		{"href": host + "/apps/{name}/restart", "method": "GET", "rel": "restart"},
		{"href": host + "/apps/{name}", "method": "POST", "rel": "update"},
		{"href": host + "/apps/{name}", "method": "DELETE", "rel": "delete"},
		{"href": host + "/apps/{name}/run", "method": "POST", "rel": "run"},
	}
	s := schema{
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
	return json.NewEncoder(w).Encode(s)
}

// serviceSchema returns a json schema for service.
func serviceSchema(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	s := schema{
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
	return json.NewEncoder(w).Encode(s)
}

// servicesSchema returns a json schema for services.
func servicesSchema(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	s := schema{
		Title: "service collection",
		Type:  "array",
		Items: &schema{
			Ref: "http://myhost.com/schema/service",
		},
	}
	return json.NewEncoder(w).Encode(s)
}
