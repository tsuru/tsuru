// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

const (
	STATUS_SYNCHRONIZING = "SYNCHRONIZING"
	STATUS_PENDING       = "PENDING"
	STATUS_OK            = "OK"
	STATUS_DELETED       = "DELETED"
)

type hrefData struct {
	Href string `json:"href"`
}

type linkData struct {
	Self hrefData `json:"self"`
}

type commonPostResponse struct {
	Links  linkData          `json:"_links,omitempty"`
	ID     int               `json:"ID,omitempty"`
	Name   string            `json:"name,omitempty"`
	Status map[string]string `json:"status,omitempty"`
}

func (c commonPostResponse) FullId() string {
	return c.Links.Self.Href
}

func (c commonPostResponse) GetName() string {
	return c.Name
}

type Target struct {
	commonPostResponse
	BackendPool string `json:"pool,omitempty"`
}

type BackendPoolHealthCheck struct {
	HcPath           string `json:"hcPath,omitempty"`
	HcBody           string `json:"hcBody,omitempty"`
	HcHttpStatusCode string `json:"hcHttpStatusCode,omitempty"`
}

type Pool struct {
	commonPostResponse
	Project       string `json:"project"`
	Environment   string `json:"environment"`
	BalancePolicy string `json:"balancepolicy"`
	BackendPoolHealthCheck
}

type RuleProperties struct {
	Match string `json:"match"`
}

type Rule struct {
	commonPostResponse
	BackendPool []string `json:"pools,omitempty"`
	Matching    string   `json:"matching,omitempty"`
	Project     string   `json:"project"`
}

type RuleOrdered struct {
	commonPostResponse
	VirtualHostGroup string `json:"virtualhostgroup"`
	Environment      string `json:"environment"`
	Rule             string `json:"rule"`
	Order            int    `json:"order"`
}

type VirtualHostGroup struct {
	commonPostResponse
}

type VirtualHost struct {
	commonPostResponse
	Environment      []string `json:"environments,omitempty"`
	Project          string   `json:"project,omitempty"`
	VirtualHostGroup string   `json:"virtualhostgroup,omitempty"`
}
