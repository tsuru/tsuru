// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/tsuru/config"
)

var timeoutHttpClient = clientWithTimeout(10 * time.Second)

type GalebClient struct {
	apiUrl            string
	username          string
	password          string
	environment       string
	farmType          string
	plan              string
	project           string
	loadBalancePolicy string
	ruleType          string
}

func NewGalebClient() (*GalebClient, error) {
	apiUrl, err := config.GetString("galeb:api-url")
	if err != nil {
		return nil, err
	}
	username, err := config.GetString("galeb:username")
	if err != nil {
		return nil, err
	}
	password, err := config.GetString("galeb:password")
	if err != nil {
		return nil, err
	}
	environment, _ := config.GetString("galeb:environment")
	farmType, _ := config.GetString("galeb:farm-type")
	plan, _ := config.GetString("galeb:plan")
	project, _ := config.GetString("galeb:project")
	loadBalancePolicy, _ := config.GetString("galeb:load-balance-policy")
	ruleType, _ := config.GetString("galeb:rule-type")
	return &GalebClient{
		apiUrl:            apiUrl,
		username:          username,
		password:          password,
		environment:       environment,
		farmType:          farmType,
		plan:              plan,
		project:           project,
		loadBalancePolicy: loadBalancePolicy,
		ruleType:          ruleType,
	}, nil
}

func clientWithTimeout(timeout time.Duration) *http.Client {
	dialTimeout := func(network, addr string) (net.Conn, error) {
		return net.DialTimeout(network, addr, timeout)
	}
	transport := http.Transport{
		Dial: dialTimeout,
	}
	return &http.Client{
		Transport: &transport,
	}
}

func (c *GalebClient) doRequest(method, path string, params interface{}) (*http.Response, error) {
	buf := bytes.Buffer{}
	if params != nil {
		err := json.NewEncoder(&buf).Encode(params)
		if err != nil {
			return nil, err
		}
	}
	url := fmt.Sprintf("%s/%s", strings.TrimRight(c.apiUrl, "/"), strings.TrimLeft(path, "/"))
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	rsp, err := timeoutHttpClient.Do(req)
	return rsp, err
}

func (c *GalebClient) doCreateResource(path string, params interface{}) (string, error) {
	rsp, err := c.doRequest("POST", path, params)
	if err != nil {
		return "", err
	}
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("POST %s: invalid response code: %d: %s - PARAMS: %#v", path, rsp.StatusCode, string(responseData), params)
	}
	var commonRsp commonResponse
	err = json.Unmarshal(responseData, &commonRsp)
	if err != nil {
		return "", fmt.Errorf("POST %s: unable to parse response: %s: %s - PARAMS: %#v", path, string(responseData), err.Error(), params)
	}
	return commonRsp.FullId(), nil
}

func (c *GalebClient) fillDefaultBackendPoolValues(params *BackendPoolParams) {
	if params.Environment == "" {
		params.Environment = c.environment
	}
	if params.LoadBalancePolicy == "" {
		params.LoadBalancePolicy = c.loadBalancePolicy
	}
	if params.Plan == "" {
		params.Plan = c.plan
	}
	if params.Project == "" {
		params.Project = c.project
	}
	if params.FarmType == "" {
		params.FarmType = c.farmType
	}
}

func (c *GalebClient) fillDefaultRuleValues(params *RuleParams) {
	if params.RuleType == "" {
		params.RuleType = c.ruleType
	}
	if params.Project == "" {
		params.Project = c.project
	}
}

func (c *GalebClient) fillDefaultVirtualHostValues(params *VirtualHostParams) {
	if params.Environment == "" {
		params.Environment = c.environment
	}
	if params.FarmType == "" {
		params.FarmType = c.farmType
	}
	if params.Plan == "" {
		params.Plan = c.plan
	}
	if params.Project == "" {
		params.Project = c.project
	}
}

func (c *GalebClient) AddBackendPool(params *BackendPoolParams) (string, error) {
	c.fillDefaultBackendPoolValues(params)
	return c.doCreateResource("/backendpool/", params)
}

func (c *GalebClient) AddBackend(params *BackendParams) (string, error) {
	return c.doCreateResource("/backend/", params)
}

func (c *GalebClient) AddRule(params *RuleParams) (string, error) {
	c.fillDefaultRuleValues(params)
	return c.doCreateResource("/rule/", params)
}

func (c *GalebClient) AddVirtualHost(params *VirtualHostParams) (string, error) {
	c.fillDefaultVirtualHostValues(params)
	return c.doCreateResource("/virtualhost/", params)
}

func (c *GalebClient) AddVirtualHostRule(params *VirtualHostRuleParams) (string, error) {
	return c.doCreateResource("/virtualhostrule/", params)
}

func (c *GalebClient) RemoveResource(resourceURI string) error {
	path := strings.TrimPrefix(resourceURI, c.apiUrl)
	rsp, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("DELETE %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	return nil
}
