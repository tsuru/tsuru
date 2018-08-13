// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

const (
	STATUS_SYNCHRONIZING = "SYNCHRONIZING"
	STATUS_PENDING       = "PENDING"
	STATUS_OK            = "OK"
)

type hrefData struct {
	Href string `json:"href"`
}

type linkData struct {
	Self hrefData `json:"self"`
}

type commonPostResponse struct {
	Links  linkData `json:"_links,omitempty"`
	ID     int      `json:"ID,omitempty"`
	Name   string   `json:"name,omitempty"`
	Status string   `json:"_status,omitempty"`
}

func (c commonPostResponse) FullId() string {
	return c.Links.Self.Href
}

func (c commonPostResponse) GetName() string {
	return c.Name
}

type BackendPoolProperties struct {
	HcPath       string `json:"hcPath"`
	HcBody       string `json:"hcBody"`
	HcStatusCode string `json:"hcStatusCode"`
}

type Target struct {
	commonPostResponse
	Project     string `json:"project"`
	Environment string `json:"environment"`
	BackendPool string `json:"parent,omitempty"`
}

type Pool struct {
	commonPostResponse
	Project       string                `json:"project"`
	Environment   string                `json:"environment"`
	BalancePolicy string                `json:"balancePolicy"`
	Properties    BackendPoolProperties `json:"properties,omitempty"`
}

type RuleProperties struct {
	Match string `json:"match"`
}

type Rule struct {
	commonPostResponse
	RuleType    string         `json:"ruleType,omitempty"`
	BackendPool string         `json:"pool,omitempty"`
	Default     bool           `json:"default,omitempty"`
	Order       int            `json:"order,omitempty"`
	Properties  RuleProperties `json:"properties,omitempty"`
}

type RuleOrdered struct {
	RuleId    int `json:"ruleId"`
	RuleOrder int `json:"ruleOrder"`
}

type VirtualHost struct {
	commonPostResponse
	Environment  string        `json:"environment,omitempty"`
	Project      string        `json:"project,omitempty"`
	RulesOrdered []RuleOrdered `json:"rulesOrdered,omitempty"`
}
