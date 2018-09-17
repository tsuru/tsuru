// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
)

const maxConnRetries = 3

type ErrItemNotFound struct {
	path string
}

func (e ErrItemNotFound) Error() string {
	return fmt.Sprintf("item not found: %s", e.path)
}

type ErrItemAlreadyExists struct {
	path   string
	params interface{}
}

func (e ErrItemAlreadyExists) Error() string {
	return fmt.Sprintf("item already exists: %s - %#v", e.path, e.params)
}

type ErrAmbiguousSearch struct {
	path  string
	items []commonPostResponse
}

func (e ErrAmbiguousSearch) Error() string {
	return fmt.Sprintf("more than one item returned in search: %s - %#v", e.path, e.items)
}

type GalebClient struct {
	token         string
	tokenMu       sync.RWMutex
	ApiURL        string
	Username      string
	Password      string
	TokenHeader   string
	Environment   string
	Project       string
	BalancePolicy string
	RuleType      string
	WaitTimeout   time.Duration
	UseToken      bool
	Debug         bool
	MaxRequests   int
}

func (c *GalebClient) getTokenHeader() string {
	if c.TokenHeader == "" {
		return "X-Auth-Token"
	}
	return http.CanonicalHeaderKey(c.TokenHeader)
}

func (c *GalebClient) getToken() (string, error) {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	if c.token == "" {
		c.tokenMu.RUnlock()
		err := c.regenerateToken()
		c.tokenMu.RLock()
		return c.token, err
	}
	return c.token, nil
}

func (c *GalebClient) regenerateToken() (err error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	url := fmt.Sprintf("%s/%s", strings.TrimRight(c.ApiURL, "/"), "token")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.Username, c.Password)
	rsp, err := net.Dial15Full60ClientWithPool.Do(req)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return errors.Errorf("invalid status code in request to /token: %d", rsp.StatusCode)
	}
	header := c.getTokenHeader()
	c.token = rsp.Header.Get(header)
	if c.token == "" {
		data, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			return err
		}
		tokenStruct := struct{ Token string }{}
		err = json.Unmarshal(data, &tokenStruct)
		if err != nil {
			return err
		}
		c.token = tokenStruct.Token
		if c.token == "" {
			return errors.Errorf("invalid empty token in request to %q: %q", url, string(data))
		}
	}
	return nil
}

func (c *GalebClient) doRequest(method, path string, params interface{}) (*http.Response, error) {
	return c.doRequestRetry(method, path, params, 0)
}

func (c *GalebClient) doRequestRetry(method, path string, params interface{}, retryCount int) (*http.Response, error) {
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
	url := fmt.Sprintf("%s/%s", strings.TrimRight(c.ApiURL, "/"), strings.TrimLeft(path, "/"))
	var bodyData string
	if c.Debug {
		bodyData = buf.String()
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return nil, err
	}
	if c.UseToken {
		var token string
		token, err = c.getToken()
		if err != nil {
			return nil, err
		}
		header := c.getTokenHeader()
		req.Header.Set(header, token)
	} else {
		req.SetBasicAuth(c.Username, c.Password)
	}
	req.Header.Set("Content-Type", contentType)
	rsp, err := net.Dial15Full60ClientWithPool.Do(req)
	if c.Debug {
		var code int
		if err == nil {
			code = rsp.StatusCode
		}
		log.Debugf("galeb %s %s %s: %d", method, url, bodyData, code)
	}
	if retryCount < maxConnRetries {
		if err == nil && rsp.StatusCode == http.StatusUnauthorized {
			err = c.regenerateToken()
			if err != nil {
				return nil, err
			}
			return c.doRequestRetry(method, path, params, retryCount+1)
		} else if err != nil && req.Method == http.MethodGet {
			return c.doRequestRetry(method, path, params, retryCount+1)
		}
	}
	return rsp, err
}

func (c *GalebClient) doCreateResource(path string, params interface{}) (string, error) {
	rsp, err := c.doRequest("POST", path, params)
	if err != nil {
		return "", err
	}
	if rsp.StatusCode == http.StatusConflict {
		return "", ErrItemAlreadyExists{path: path, params: params}
	}
	if rsp.StatusCode != http.StatusCreated {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		rsp.Body.Close()
		return "", errors.Errorf("POST %s: invalid response code: %d: %s - PARAMS: %#v", path, rsp.StatusCode, string(responseData), params)
	}
	location := rsp.Header.Get("Location")
	if location == "" {
		return "", errors.Errorf("POST %s: empty location header. PARAMS: %#v", path, params)
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

func (c *GalebClient) AddVirtualHost(addr string, wait bool) (string, error) {
	var params VirtualHost
	c.fillDefaultVirtualHostValues(&params)
	params.Name = addr
	resource, err := c.doCreateResource("/virtualhost", &params)
	if err != nil {
		return "", err
	}
	if wait {
		err = c.waitStatusOK(resource)
		if err != nil {
			c.removeResource(resource)
			return "", err
		}
	}
	return resource, nil
}

func (c *GalebClient) AddBackendPool(name string, wait bool) (string, error) {
	var params Pool
	c.fillDefaultPoolValues(&params)
	params.Name = name
	resource, err := c.doCreateResource("/pool", &params)
	if err != nil {
		return "", err
	}
	if wait {
		err = c.waitStatusOK(resource)
		if err != nil {
			c.removeResource(resource)
			return "", err
		}
	}
	return resource, nil
}

func (c *GalebClient) getPoolProperties(poolID string) (BackendPoolProperties, error) {
	var properties BackendPoolProperties
	path := strings.TrimPrefix(poolID, c.ApiURL)
	rsp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return properties, err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusNoContent {
		var rspObj struct {
			Properties BackendPoolProperties
		}
		responseData, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			return properties, err
		}
		err = json.Unmarshal(responseData, &rspObj)
		if err != nil {
			return properties, err
		}
		properties = rspObj.Properties
	}
	return properties, nil
}

func (c *GalebClient) UpdatePoolProperties(poolName string, properties BackendPoolProperties) error {
	poolID, err := c.findItemByName("pool", poolName)
	if err != nil {
		return err
	}
	currPropeties, err := c.getPoolProperties(poolID)
	if err == nil && currPropeties == properties {
		log.Debugf("skipping properties update for pool %q", poolName)
		return nil
	}
	path := strings.TrimPrefix(poolID, c.ApiURL)
	var poolParam Pool
	c.fillDefaultPoolValues(&poolParam)
	poolParam.Name = poolName
	poolParam.Properties = properties
	rsp, err := c.doRequest("PATCH", path, poolParam)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusNoContent {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		rsp.Body.Close()
		return errors.Errorf("PATCH %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	return c.waitStatusOK(poolID)
}

func (c *GalebClient) AddBackends(backends []*url.URL, poolName string, wait bool) error {
	poolID, err := c.findItemByName("pool", poolName)
	if err != nil {
		return err
	}
	errCh := make(chan error, len(backends))
	wg := sync.WaitGroup{}
	var limiter chan struct{}
	if c.MaxRequests > 0 {
		limiter = make(chan struct{}, c.MaxRequests)
	}
	for i := range backends {
		wg.Add(1)
		go func(i int) {
			if limiter != nil {
				limiter <- struct{}{}
				defer func() { <-limiter }()
			}
			defer wg.Done()
			var params Target
			c.fillDefaultTargetValues(&params)
			params.Name = backends[i].String()
			params.BackendPool = poolID
			resource, cerr := c.doCreateResource("/target", &params)
			if cerr != nil {
				if _, ok := cerr.(ErrItemAlreadyExists); ok {
					return
				}
				errCh <- cerr
			}
			if wait {
				cerr = c.waitStatusOK(resource)
				if cerr != nil {
					c.removeResource(resource)
					errCh <- cerr
				}
			}
		}(i)
	}
	done := make(chan bool)
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case err = <-errCh:
		return err
	}
	return nil
}

func (c *GalebClient) AddRuleToPool(name, poolName string) (string, error) {
	id, err := c.findItemByName("pool", poolName)
	if err != nil {
		return "", err
	}
	return c.addRuleToID(name, id)
}

func (c *GalebClient) addRuleToID(name, poolID string) (string, error) {
	var params Rule
	c.fillDefaultRuleValues(&params)
	params.Name = name
	params.BackendPool = poolID
	return c.doCreateResource("/rule", &params)
}

func (c *GalebClient) setRuleVirtualHostIDs(ruleID, virtualHostID string, wait bool) error {
	path := fmt.Sprintf("%s/parents", strings.TrimPrefix(ruleID, c.ApiURL))
	rsp, err := c.doRequest("PATCH", path, virtualHostID)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusNoContent {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		rsp.Body.Close()
		return errors.Errorf("PATCH %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	if wait {
		err = c.waitStatusOK(ruleID)
		if err != nil {
			c.removeRuleVirtualHostByID(ruleID, virtualHostID)
			return err
		}
	}
	return nil
}

func (c *GalebClient) SetRuleVirtualHost(ruleName, virtualHostName string, wait bool) error {
	ruleID, err := c.findItemByName("rule", ruleName)
	if err != nil {
		return err
	}
	virtualHostID, err := c.findItemByName("virtualhost", virtualHostName)
	if err != nil {
		return err
	}
	return c.setRuleVirtualHostIDs(ruleID, virtualHostID, wait)
}

func (c *GalebClient) RemoveBackendByID(backendID string) error {
	return c.removeResource(backendID)
}

func (c *GalebClient) RemoveBackendsByIDs(backendIDs []string) error {
	errCh := make(chan error, len(backendIDs))
	wg := sync.WaitGroup{}
	var limiter chan struct{}
	if c.MaxRequests > 0 {
		limiter = make(chan struct{}, c.MaxRequests)
	}
	for i := range backendIDs {
		wg.Add(1)
		go func(i int) {
			if limiter != nil {
				limiter <- struct{}{}
				defer func() { <-limiter }()
			}
			defer wg.Done()
			err := c.removeResource(backendIDs[i])
			if err != nil {
				errCh <- err
			}
		}(i)
	}
	done := make(chan bool)
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case err := <-errCh:
		return err
	}
	return nil
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

func (c *GalebClient) getRuleVirtualhosts(ruleID string) ([]VirtualHost, error) {
	var rspObj struct {
		Embedded struct {
			VirtualHost []VirtualHost `json:"virtualhost"`
		} `json:"_embedded"`
	}
	path := fmt.Sprintf("%s/parents", strings.TrimPrefix(ruleID, c.ApiURL))
	err := c.getObj(path, &rspObj)
	if err != nil {
		return nil, err
	}
	return rspObj.Embedded.VirtualHost, nil
}

func (c *GalebClient) removeRuleVirtualHostByID(ruleID, virtualHostID string) error {
	vhId := virtualHostID[strings.LastIndex(virtualHostID, "/")+1:]
	path := fmt.Sprintf("%s/parents/%s", ruleID, vhId)
	err := c.removeResource(path)
	if err != nil {
		return err
	}
	virtualhosts, err := c.getRuleVirtualhosts(ruleID)
	if err != nil {
		return nil
	}
	if len(virtualhosts) > 0 {
		return c.waitStatusOK(ruleID)
	}
	return nil
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
	return c.removeRuleVirtualHostByID(ruleID, virtualHostID)
}

func (c *GalebClient) FindTargetsByParent(poolName string) ([]Target, error) {
	path := fmt.Sprintf("/target/search/findByParentName?name=%s&size=999999", poolName)
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("GET /target/search/findByParentName?name={parentName}: wrong status code: %d. content: %s", rsp.StatusCode, string(responseData))
	}
	var rspObj struct {
		Embedded struct {
			Targets []Target `json:"target"`
		} `json:"_embedded"`
	}
	err = json.Unmarshal(responseData, &rspObj)
	if err != nil {
		return nil, errors.Wrapf(err, "GET /target/search/findByParentName?name={parentName}: unable to parse: %s", string(responseData))
	}
	return rspObj.Embedded.Targets, nil
}

func (c *GalebClient) FindVirtualHostsByRule(ruleName string) ([]VirtualHost, error) {
	ruleID, err := c.findItemByName("rule", ruleName)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("%s/parents?size=999999", strings.TrimPrefix(ruleID, c.ApiURL))
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("GET /rule/{id}/parents: wrong status code: %d. content: %s", rsp.StatusCode, string(responseData))
	}
	var rspObj struct {
		Embedded struct {
			VirtualHosts []VirtualHost `json:"virtualhost"`
		} `json:"_embedded"`
	}
	err = json.Unmarshal(responseData, &rspObj)
	if err != nil {
		return nil, errors.Wrapf(err, "GET /rule/{id}/parents: unable to parse: %s", string(responseData))
	}
	return rspObj.Embedded.VirtualHosts, nil
}

func (c *GalebClient) Healthcheck() error {
	rsp, err := c.doRequest("GET", "/healthcheck", nil)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	data, _ := ioutil.ReadAll(rsp.Body)
	dataStr := string(data)
	if rsp.StatusCode != http.StatusOK {
		return errors.Errorf("wrong healthcheck status code: %d. content: %s", rsp.StatusCode, dataStr)
	}
	if !strings.HasPrefix(dataStr, "WORKING") {
		return errors.Errorf("wrong healthcheck response: %s.", dataStr)
	}
	return nil
}

func (c *GalebClient) UpdateVirtualHostRule(virtualHostName, ruleName string, wait bool) error {
	vhID, err := c.findItemByName("virtualhost", virtualHostName)
	if err != nil {
		return err
	}
	_, ruleID, err := c.findItemIDsByName("rule", ruleName)
	if err != nil {
		return err
	}
	path := strings.TrimPrefix(vhID, c.ApiURL)
	var vh VirtualHost
	err = c.getObj(path, &vh)
	if err != nil {
		return err
	}
	vh.RulesOrdered = []RuleOrdered{
		{RuleId: ruleID, RuleOrder: 0},
	}
	rsp, err := c.doRequest("PATCH", path, vh)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusNoContent {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		rsp.Body.Close()
		return errors.Errorf("PATCH %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	if wait {
		return c.waitStatusOK(vhID)
	}
	return nil
}

func (c *GalebClient) removeResource(resourceURI string) error {
	path := strings.TrimPrefix(resourceURI, c.ApiURL)
	rsp, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusNoContent {
		return errors.Errorf("DELETE %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	return nil
}

func (c *GalebClient) findItemByName(item, name string) (string, error) {
	idStr, _, err := c.findItemIDsByName(item, name)
	return idStr, err
}

func (c *GalebClient) findItemIDsByName(item, name string) (string, int, error) {
	path := fmt.Sprintf("/%s/search/findByName?name=%s", item, name)
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return "", 0, err
	}
	var rspObj struct {
		Embedded map[string][]commonPostResponse `json:"_embedded"`
	}
	defer rsp.Body.Close()
	rspData, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return "", 0, err
	}
	err = json.Unmarshal(rspData, &rspObj)
	if err != nil {
		return "", 0, errors.Wrapf(err, "unable to parse find response %q", string(rspData))
	}
	itemList := rspObj.Embedded[item]
	if len(itemList) == 0 {
		return "", 0, ErrItemNotFound{path: path}
	}
	if len(itemList) > 1 {
		return "", 0, ErrAmbiguousSearch{path: path, items: itemList}
	}
	itemObj := rspObj.Embedded[item][0]
	id := itemObj.FullId()
	if id == "" {
		return "", 0, ErrItemNotFound{path: path}
	}
	return id, itemObj.ID, nil
}

func (c *GalebClient) fetchPathStatus(path string) (string, error) {
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return "", errors.Wrapf(err, "GET %s: unable to make request", path)
	}
	defer rsp.Body.Close()
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusOK {
		return "", errors.Errorf("GET %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	var response commonPostResponse
	err = json.Unmarshal(responseData, &response)
	if err != nil {
		return "", errors.Wrapf(err, "GET %s: unable to unmarshal response. data: %s", path, string(responseData))
	}
	return response.Status, nil
}

func (c *GalebClient) waitStatusOK(resourceURI string) error {
	path := strings.TrimPrefix(resourceURI, c.ApiURL)
	var timeout <-chan time.Time
	if c.WaitTimeout != 0 {
		timeout = time.After(c.WaitTimeout)
	}
	var status string
	var err error
loop:
	for {
		status, err = c.fetchPathStatus(path)
		if err != nil || (status != STATUS_PENDING && status != STATUS_SYNCHRONIZING) {
			break
		}
		select {
		case <-timeout:
			err = errors.Errorf("GET %s: timeout after %v waiting for status change from %s", path, c.WaitTimeout, status)
			break loop
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
	if err != nil {
		return err
	}
	if status != STATUS_OK {
		return errors.Errorf("GET %s: invalid status %s", path, status)
	}
	return nil
}

func (c *GalebClient) getObj(path string, data interface{}) error {
	rsp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusOK {
		return errors.Errorf("GET %s: wrong status code: %d. content: %s", path, rsp.StatusCode, string(responseData))
	}
	err = json.Unmarshal(responseData, data)
	if err != nil {
		return errors.Wrapf(err, "GET %s: unable to parse: %s", path, string(responseData))
	}
	return nil
}

func IsErrExists(err error) bool {
	if err == nil {
		return false
	}
	_, ok := errors.Cause(err).(ErrItemAlreadyExists)
	return ok
}
