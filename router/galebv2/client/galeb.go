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
	ErrTargetNotFound      = errors.New("target not found")
	ErrVirtualHostNotFound = errors.New("virtualhost not found")
	ErrRuleNotFound        = errors.New("rule not found")
	ErrItemNotFound        = errors.New("item not found")
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

func (c *GalebClient) AddRule(name, poolName, virtualHostName string) (string, error) {
	var params Rule
	c.fillDefaultRuleValues(&params)
	params.Name = name
	poolID, err := c.findItemByName("target", poolName)
	if err != nil {
		return "", err
	}
	virtualHostID, err := c.findItemByName("virtualhost", virtualHostName)
	if err != nil {
		return "", err
	}
	params.BackendPool = poolID
	params.VirtualHost = virtualHostID
	return c.doCreateResource("/rule", &params)
}

func (c *GalebClient) RemoveBackend(backend *url.URL) error {
	id, err := c.findItemByName("target", backend.String())
	if err != nil {
		return err
	}
	return c.removeResource(id)
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

func (c *GalebClient) RemoveRule(ruleName string) error {
	id, err := c.findItemByName("rule", ruleName)
	if err != nil {
		return err
	}
	return c.removeResource(id)
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

func (c *GalebClient) findItemByName(item, name string) (string, error) {
	path := fmt.Sprintf("/%s/search/findByName?name=%s", item, name)
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
	if len(rspObj.Embedded[item]) == 0 {
		return "", ErrItemNotFound
	}
	id := rspObj.Embedded[item][0].FullId()
	if id == "" {
		return "", ErrItemNotFound
	}
	return id, nil
}
