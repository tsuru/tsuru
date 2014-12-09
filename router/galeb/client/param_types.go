// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

type commonResponse struct {
	Links struct {
		Self string `json:"self"`
	} `json:"_links"`
	Id     int    `json:"id"`
	Status string `json:"status"`
}

func (c commonResponse) FullId() string {
	return c.Links.Self
}

type BackendPoolParams struct {
	Name              string `json:"name"`
	Environment       string `json:"environment"`
	FarmType          string `json:"farmtype"`
	Plan              string `json:"plan"`
	Project           string `json:"project"`
	LoadBalancePolicy string `json:"loadbalancepolicy"`
}

type BackendParams struct {
	Ip          string `json:"ip"`
	Port        int    `json:"port"`
	BackendPool string `json:"backendpool"`
}

type RuleParams struct {
	Name        string `json:"name"`
	Match       string `json:"match"`
	BackendPool string `json:"backendpool"`
	RuleType    string `json:"ruletype"`
	Project     string `json:"project"`
}

type VirtualHostParams struct {
	Name        string `json:"name"`
	FarmType    string `json:"farmtype"`
	Plan        string `json:"plan"`
	Environment string `json:"environment"`
	Project     string `json:"project"`
	RuleDefault string `json:"rule_default"`
}

type VirtualHostRuleParams struct {
	Order       int    `json:"order"`
	VirtualHost string `json:"virtualhost"`
	Rule        string `json:"rule"`
}
