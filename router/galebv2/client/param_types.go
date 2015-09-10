// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

type linkData struct {
	Self struct {
		Href string `json:"href"`
	} `json:"self"`
}

type commonPostResponse struct {
	Links linkData `json:"_links,omitempty"`
	ID    int      `json:"ID,omitempty"`
	Name  string   `json:"name,omitempty"`
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
	HcStatusCode int    `json:"hcStatusCode"`
}

type Target struct {
	commonPostResponse
	Project       string                `json:"project"`
	Environment   string                `json:"environment"`
	BalancePolicy string                `json:"balancePolicy"`
	TargetType    string                `json:"targetType"`
	BackendPool   string                `json:"parent,omitempty"`
	Properties    BackendPoolProperties `json:"properties,omitempty"`
}

type RuleProperties struct {
	Match string `json:"match"`
}

type Rule struct {
	commonPostResponse
	RuleType    string         `json:"ruleType,omitempty"`
	VirtualHost string         `json:"parent,omitempty"`
	BackendPool string         `json:"target,omitempty"`
	Default     bool           `json:"default,omitempty"`
	Order       int            `json:"order,omitempty"`
	Properties  RuleProperties `json:"properties,omitempty"`
}

type VirtualHost struct {
	commonPostResponse
	Environment string `json:"environment"`
	Project     string `json:"project"`
}
