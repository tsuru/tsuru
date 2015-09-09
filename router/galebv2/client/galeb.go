// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrItemNotFound    = errors.New("item not found")
	ErrAmbiguousSearch = errors.New("more than one item returned in search")
)

type GalebClient struct {
	ApiUrl            string
	Username          string
	Password          string
	Environment       string
	Project           string
	BalancePolicy     string
	RuleType          string
	TargetTypeBackend string
	TargetTypePool    string
}

var timeoutHttpClient = &http.Client{
	Transport: &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
	},
	Timeout: time.Minute,
}

func (c *GalebClient) doRequest(method, path string, params interface{}) (*http.Response, error) {
	buf := bytes.Buffer{}
	if params != nil {
		err := json.NewEncoder(&buf).Encode(params)
		if err != nil {
			return nil, err
		}
	}
	url := fmt.Sprintf("%s/%s", strings.TrimRight(c.ApiUrl, "/"), strings.TrimLeft(path, "/"))
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.Username, c.Password)
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
	var commonRsp commonPostResponse
	err = json.Unmarshal(responseData, &commonRsp)
	if err != nil {
		return "", fmt.Errorf("POST %s: unable to parse response: %s: %s - PARAMS: %#v", path, string(responseData), err.Error(), params)
	}
	return commonRsp.FullId(), nil
}

func (c *GalebClient) fillDefaultTargetValues(params *Target) {
	if params.Environment == "" {
		params.Environment = c.Environment
	}
	if params.Project == "" {
		params.Project = c.Project
	}
	if params.BalancePolicy == "" {
		params.BalancePolicy = c.BalancePolicy
	}
}

func (c *GalebClient) fillDefaultRuleValues(params *Rule) {
	if params.RuleType == "" {
		params.RuleType = c.RuleType
	}
	params.Properties.Match = "/"
	params.Default = true
	params.Order = 0
}

func (c *GalebClient) fillDefaultVirtualHostValues(params *VirtualHost) {
	if params.Environment == "" {
		params.Environment = c.Environment
	}
	if params.Project == "" {
		params.Project = c.Project
	}
}

func (c *GalebClient) AddVirtualHost(addr string) (string, error) {
	var params VirtualHost
	c.fillDefaultVirtualHostValues(&params)
	params.Name = addr
	return c.doCreateResource("/virtualhost", &params)
}

func (c *GalebClient) AddBackendPool(name string) (string, error) {
	var params Target
	c.fillDefaultTargetValues(&params)
	params.Name = name
	params.TargetType = c.TargetTypePool
	return c.doCreateResource("/target", &params)
}

func (c *GalebClient) AddBackend(backend *url.URL, poolName string) (string, error) {
	var params Target
	c.fillDefaultTargetValues(&params)
	params.Name = backend.String()
	poolID, err := c.findItemByName("target", poolName)
	if err != nil {
		return "", err
	}
	params.BackendPool = poolID
	params.TargetType = c.TargetTypeBackend
	return c.doCreateResource("/target", &params)
}

func (c *GalebClient) AddRuleToID(name, poolID string) (string, error) {
	var params Rule
	c.fillDefaultRuleValues(&params)
	params.Name = name
	params.BackendPool = poolID
	return c.doCreateResource("/rule", &params)
}

func (c *GalebClient) SetRuleVirtualHostIDs(ruleID, virtualHostID string) error {
	var params Rule
	params.VirtualHost = virtualHostID
	path := strings.TrimPrefix(ruleID, c.ApiUrl)
	rsp, err := c.doRequest("PATCH", path, &params)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusNoContent {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		return fmt.Errorf("PATCH %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	return err
}

func (c *GalebClient) SetRuleVirtualHost(ruleName, virtualHostName string) error {
	ruleID, err := c.findRuleByNameEmptyParent(ruleName)
	if err != nil {
		return err
	}
	virtualHostID, err := c.findItemByName("virtualhost", virtualHostName)
	if err != nil {
		return err
	}
	return c.SetRuleVirtualHostIDs(ruleID, virtualHostID)
}

func (c *GalebClient) RemoveBackendByID(backendID string) error {
	return c.removeResource(backendID)
}

func (c *GalebClient) RemoveBackendPool(poolName string) error {
	id, err := c.findItemByName("target", poolName)
	if err != nil {
		return err
	}
	return c.removeResource(id)
}

func (c *GalebClient) RemoveVirtualHost(virtualHostName string) error {
	id, err := c.findItemByName("virtualhost", virtualHostName)
	if err != nil {
		return err
	}
	return c.removeResource(id)
}

func (c *GalebClient) RemoveVirtualHostByID(virtualHostID string) error {
	return c.removeResource(virtualHostID)
}

func (c *GalebClient) RemoveRuleByID(ruleID string) error {
	return c.removeResource(ruleID)
}

func (c *GalebClient) FindTargetsByParent(poolName string) ([]Target, error) {
	path := fmt.Sprintf("/target/search/findByParentName?name=%s&size=99999", poolName)
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var rspObj struct {
		Embedded struct {
			Targets []Target `json:"target"`
		} `json:"_embedded"`
	}
	err = json.NewDecoder(rsp.Body).Decode(&rspObj)
	if err != nil {
		return nil, err
	}
	return rspObj.Embedded.Targets, nil
}

func (c *GalebClient) FindRulesByTargetName(targetName string) ([]Rule, error) {
	path := fmt.Sprintf("/rule/search/findByTargetName?name=%s&size=99999", targetName)
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var rspObj struct {
		Embedded struct {
			Rules []Rule `json:"rule"`
		} `json:"_embedded"`
	}
	err = json.NewDecoder(rsp.Body).Decode(&rspObj)
	if err != nil {
		return nil, err
	}
	return rspObj.Embedded.Rules, nil
}

func (c *GalebClient) removeResource(resourceURI string) error {
	path := strings.TrimPrefix(resourceURI, c.ApiUrl)
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

func (c *GalebClient) findRuleByNameEmptyParent(name string) (string, error) {
	path := fmt.Sprintf("/rule/search/findByNameAndParent?name=%s&parent=", name)
	return c.findItemByPath("rule", path)
}

func (c *GalebClient) findItemByName(item, name string) (string, error) {
	path := fmt.Sprintf("/%s/search/findByName?name=%s", item, name)
	return c.findItemByPath(item, path)
}

func (c *GalebClient) findItemByPath(item, path string) (string, error) {
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return "", err
	}
	var rspObj struct {
		Embedded map[string][]commonPostResponse `json:"_embedded"`
	}
	rspData, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(rspData, &rspObj)
	if err != nil {
		return "", fmt.Errorf("unable to parse find response %q: %s", string(rspData), err)
	}
	itemList := rspObj.Embedded[item]
	if len(itemList) == 0 {
		return "", ErrItemNotFound
	}
	if len(itemList) > 1 {
		return "", ErrAmbiguousSearch
	}
	id := rspObj.Embedded[item][0].FullId()
	if id == "" {
		return "", ErrItemNotFound
	}
	return id, nil
}
