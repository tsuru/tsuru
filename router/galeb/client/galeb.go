// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
)

var (
	ErrItemNotFound      = errors.New("item not found")
	ErrItemAlreadyExists = errors.New("item already exists")
	ErrAmbiguousSearch   = errors.New("more than one item returned in search")
)

type GalebClient struct {
	ApiUrl        string
	Username      string
	Password      string
	Token         string
	TokenHeader   string
	Environment   string
	Project       string
	BalancePolicy string
	RuleType      string
	Debug         bool
}

func (c *GalebClient) doRequest(method, path string, params interface{}) (*http.Response, error) {
	buf := bytes.Buffer{}
	contentType := "application/json"
	if params != nil {
		switch val := params.(type) {
		case string:
			contentType = "text/uri-list"
			buf.WriteString(val)
		default:
			err := json.NewEncoder(&buf).Encode(params)
			if err != nil {
				return nil, err
			}
		}
	}
	url := fmt.Sprintf("%s/%s", strings.TrimRight(c.ApiUrl, "/"), strings.TrimLeft(path, "/"))
	var bodyData string
	if c.Debug {
		bodyData = buf.String()
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		header := c.TokenHeader
		if header == "" {
			header = "x-auth-token"
		}
		req.Header.Set(header, c.Token)
	} else {
		req.SetBasicAuth(c.Username, c.Password)
	}
	req.Header.Set("Content-Type", contentType)
	rsp, err := net.Dial5Full60Client.Do(req)
	if c.Debug {
		var code int
		if err == nil {
			code = rsp.StatusCode
		}
		log.Debugf("galeb %s %s %s: %d", method, url, bodyData, code)
	}
	return rsp, err
}

func (c *GalebClient) doCreateResource(path string, params interface{}) (string, error) {
	rsp, err := c.doRequest("POST", path, params)
	if err != nil {
		return "", err
	}
	if rsp.StatusCode == http.StatusConflict {
		return "", ErrItemAlreadyExists
	}
	if rsp.StatusCode != http.StatusCreated {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		return "", fmt.Errorf("POST %s: invalid response code: %d: %s - PARAMS: %#v", path, rsp.StatusCode, string(responseData), params)
	}
	location := rsp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("POST %s: empty location header. PARAMS: %#v", path, params)
	}
	return location, nil
}

func (c *GalebClient) fillDefaultTargetValues(params *Target) {
	if params.Environment == "" {
		params.Environment = c.Environment
	}
	if params.Project == "" {
		params.Project = c.Project
	}
}

func (c *GalebClient) fillDefaultPoolValues(params *Pool) {
	if params.Environment == "" {
		params.Environment = c.Environment
	}
	if params.Project == "" {
		params.Project = c.Project
	}
	if params.BalancePolicy == "" {
		params.BalancePolicy = c.BalancePolicy
	}
	params.Properties.HcPath = "/"
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
	resource, err := c.doCreateResource("/virtualhost", &params)
	if err != nil {
		return "", err
	}
	return resource, c.waitStatusOK(resource)
}

func (c *GalebClient) AddBackendPool(name string) (string, error) {
	var params Pool
	c.fillDefaultPoolValues(&params)
	params.Name = name
	resource, err := c.doCreateResource("/pool", &params)
	if err != nil {
		return "", err
	}
	return resource, c.waitStatusOK(resource)
}

func (c *GalebClient) AddBackend(backend *url.URL, poolName string) (string, error) {
	var params Target
	c.fillDefaultTargetValues(&params)
	params.Name = backend.String()
	poolID, err := c.findItemByName("pool", poolName)
	if err != nil {
		return "", err
	}
	params.BackendPool = poolID
	resource, err := c.doCreateResource("/target", &params)
	if err != nil {
		return "", err
	}
	return resource, c.waitStatusOK(resource)
}

func (c *GalebClient) AddRuleToID(name, poolID string) (string, error) {
	var params Rule
	c.fillDefaultRuleValues(&params)
	params.Name = name
	params.BackendPool = poolID
	return c.doCreateResource("/rule", &params)
}

func (c *GalebClient) SetRuleVirtualHostIDs(ruleID, virtualHostID string) error {
	path := fmt.Sprintf("%s/parents", strings.TrimPrefix(ruleID, c.ApiUrl))
	rsp, err := c.doRequest("PATCH", path, virtualHostID)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusNoContent {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		return fmt.Errorf("PATCH %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	return c.waitStatusOK(ruleID)
}

func (c *GalebClient) SetRuleVirtualHost(ruleName, virtualHostName string) error {
	ruleID, err := c.findItemByName("rule", ruleName)
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
	id, err := c.findItemByName("pool", poolName)
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

func (c *GalebClient) RemoveRule(ruleName string) error {
	ruleID, err := c.findItemByName("rule", ruleName)
	if err != nil {
		return err
	}
	return c.removeResource(ruleID)
}

func (c *GalebClient) RemoveRuleVirtualHostByID(ruleID, virtualHostID string) error {
	vhId := virtualHostID[strings.LastIndex(virtualHostID, "/")+1:]
	path := fmt.Sprintf("%s/parents/%s", ruleID, vhId)
	return c.removeResource(path)
}

func (c *GalebClient) RemoveRuleVirtualHost(ruleName, virtualHostName string) error {
	ruleID, err := c.findItemByName("rule", ruleName)
	if err != nil {
		return err
	}
	virtualHostID, err := c.findItemByName("virtualhost", virtualHostName)
	if err != nil {
		return err
	}
	return c.RemoveRuleVirtualHostByID(ruleID, virtualHostID)
}

func (c *GalebClient) FindTargetsByParent(poolName string) ([]Target, error) {
	path := fmt.Sprintf("/target/search/findByParentName?name=%s&size=999999", poolName)
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /target/search/findByParentName?name={parentName}: wrong status code: %d. content: %s", rsp.StatusCode, string(responseData))
	}
	var rspObj struct {
		Embedded struct {
			Targets []Target `json:"target"`
		} `json:"_embedded"`
	}
	err = json.Unmarshal(responseData, &rspObj)
	if err != nil {
		return nil, fmt.Errorf("GET /target/search/findByParentName?name={parentName}: unable to parse: %s: %s", string(responseData), err)
	}
	return rspObj.Embedded.Targets, nil
}

func (c *GalebClient) FindVirtualHostsByRule(ruleName string) ([]VirtualHost, error) {
	ruleID, err := c.findItemByName("rule", ruleName)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("%s/parents?size=999999", strings.TrimPrefix(ruleID, c.ApiUrl))
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /rule/{id}/parents: wrong status code: %d. content: %s", rsp.StatusCode, string(responseData))
	}
	var rspObj struct {
		Embedded struct {
			VirtualHosts []VirtualHost `json:"virtualhost"`
		} `json:"_embedded"`
	}
	err = json.Unmarshal(responseData, &rspObj)
	if err != nil {
		return nil, fmt.Errorf("GET /rule/{id}/parents: unable to parse: %s: %s", string(responseData), err)
	}
	return rspObj.Embedded.VirtualHosts, nil
}

func (c *GalebClient) Healthcheck() error {
	rsp, err := c.doRequest("GET", "/healthcheck", nil)
	if err != nil {
		return err
	}
	data, _ := ioutil.ReadAll(rsp.Body)
	dataStr := string(data)
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("wrong healthcheck status code: %d. content: %s", rsp.StatusCode, dataStr)
	}
	if !strings.HasPrefix(dataStr, "WORKING") {
		return fmt.Errorf("wrong healthcheck response: %s.", dataStr)
	}
	return nil
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

func (c *GalebClient) fetchPathStatus(path string) (string, error) {
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return "", fmt.Errorf("GET %s: unable to make request: %s", path, err)
	}
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	var response commonPostResponse
	err = json.Unmarshal(responseData, &response)
	if err != nil {
		return "", fmt.Errorf("GET %s: unable to unmarshal response: %s: %s", path, err, string(responseData))
	}
	return response.Status, nil
}

func (c *GalebClient) waitStatusOK(resourceURI string) error {
	path := strings.TrimPrefix(resourceURI, c.ApiUrl)
	maxWaitTime := 30 * time.Second
	timeout := time.After(maxWaitTime)
	var status string
	var err error
	for {
		status, err = c.fetchPathStatus(path)
		if err != nil || (status != STATUS_PENDING && status != STATUS_SYNCHRONIZING) {
			break
		}
		select {
		case <-timeout:
			err = fmt.Errorf("GET %s: timeout after %v waiting for status change from PENDING", path, maxWaitTime)
			break
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
	if err != nil {
		return err
	}
	if status != STATUS_OK {
		return fmt.Errorf("GET %s: invalid status %s", path, status)
	}
	return nil
}
