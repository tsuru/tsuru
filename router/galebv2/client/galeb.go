// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"bytes"
	"context"
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

// Target - One for each unit
// |
// v
// Pool - Created on app create (tsuru-backendpool-<router name>-<app name>)
// |
// v
// Rule - Created on app create (tsuru-rootrule-<router name>-<app name>)
// |
// v
// Rule Ordered - Created on app create.
// |
// v
// VirtualHostGroup - Created automatically when first virtual host is created
//                    on app create.
// | 1
// |
// v *
// VirtualHost - Created for each cname, all added to the same VirtualHostGroup
//               as the one created for the first virtual host.

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
	path := "/token"
	url := fmt.Sprintf("%s%s", strings.TrimRight(c.ApiURL, "/"), path)
	req, err := http.NewRequest(http.MethodGet, url, nil)
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
		return errors.Errorf("GET %s: invalid status code in request to /token: %d", path, rsp.StatusCode)
	}
	data, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return err
	}
	tokenStruct := struct{ Token string }{}
	err = json.Unmarshal(data, &tokenStruct)
	if err != nil {
		return errors.Wrapf(err, "GET %s: unable to parse json response", path)
	}
	c.token = tokenStruct.Token
	if c.token == "" {
		return errors.Errorf("GET %s: invalid empty token in request: %q", path, string(data))
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
		log.Debugf("Use token: %s", token)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(c.Username, token)
	} else {
		log.Debugf("Use basic auth: %s, %s", c.Username, c.Password)
		req.SetBasicAuth(c.Username, c.Password)
	}
	req.Header.Set("Content-Type", contentType)
	rsp, err := net.Dial15Full60ClientWithPool.Do(req)
	if c.Debug {
		var code int
		if err == nil {
			code = rsp.StatusCode
		}
		var rspData []byte
		if rsp != nil {
			rspData, err = ioutil.ReadAll(rsp.Body)
			if err != nil {
				return nil, errors.Wrapf(err, "error reading request body %s %s", method, url)
			}
			rsp.Body = ioutil.NopCloser(bytes.NewReader(rspData))
		}
		log.Debugf("galebv2 debug %s %s %q: %d %q", method, url, bodyData, code, rspData)
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
	rsp, err := c.doRequest(http.MethodPost, path, params)
	if err != nil {
		return "", err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode == http.StatusConflict {
		return "", ErrItemAlreadyExists{path: path, params: params}
	}
	if rsp.StatusCode != http.StatusCreated {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		return "", errors.Errorf("POST %s: invalid response code: %d: %s - PARAMS: %#v", path, rsp.StatusCode, string(responseData), params)
	}
	location := rsp.Header.Get("Location")
	if location == "" {
		return "", errors.Errorf("POST %s: empty location header. PARAMS: %#v", path, params)
	}
	return location, nil
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
}

func (c *GalebClient) fillDefaultRuleValues(params *Rule) {
	params.Matching = "/"
	if params.Project == "" {
		params.Project = c.Project
	}
}

func (c *GalebClient) fillDefaultRuleOrderedValues(params *RuleOrdered) {
	if params.Environment == "" {
		params.Environment = c.Environment
	}
	params.Order = 1
}

func (c *GalebClient) fillDefaultVirtualHostValues(params *VirtualHost) {
	if len(params.Environment) == 0 {
		params.Environment = []string{c.Environment}
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

func (c *GalebClient) getVirtualHostWithGroup(addr string, virtualHostWithGroup string) (VirtualHost, error) {
	virtualHostID, err := c.findItemByName("virtualhost", virtualHostWithGroup)
	if err != nil {
		return VirtualHost{}, err
	}
	virtualhostGroupId, err := c.FindVirtualHostGroupByVirtualHostId(virtualHostID)
	if err != nil {
		return VirtualHost{}, err
	}

	var params VirtualHost
	c.fillDefaultVirtualHostValues(&params)
	params.Name = addr
	params.VirtualHostGroup = fmt.Sprintf("%s/virtualhostgroup/%d", c.ApiURL, virtualhostGroupId)
	return params, nil
}

func (c *GalebClient) AddVirtualHostWithGroup(addr string, virtualHostWithGroup string, wait bool) (string, error) {
	params, err := c.getVirtualHostWithGroup(addr, virtualHostWithGroup)
	if err != nil {
		return "", err
	}
	resource, err := c.doCreateResource("/virtualhost", &params)
	if err != nil {
		return "", err
	}
	if wait {
		err = c.waitStatusOK(resource)
	}
	return resource, err
}

func (c *GalebClient) UpdateVirtualHostWithGroup(addr string, virtualHostWithGroup string, wait bool) error {
	virtualHostFullID, virtualHostID, err := c.findItemIDsByName("virtualhost", addr)
	if err != nil {
		return err
	}
	params, err := c.getVirtualHostWithGroup(addr, virtualHostWithGroup)
	if err != nil {
		return err
	}
	params.ID = virtualHostID
	path := fmt.Sprintf("/virtualhost/%d", virtualHostID)
	rsp, err := c.doRequest(http.MethodPatch, path, &params)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusNoContent {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		return errors.Errorf("PATCH %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	if wait {
		return c.waitStatusOK(virtualHostFullID)
	}
	return nil
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
	}
	return resource, err
}

func (c *GalebClient) getPoolProperties(poolID string) (BackendPoolHealthCheck, error) {
	var pool Pool
	path := strings.TrimPrefix(poolID, c.ApiURL)
	err := c.getObj(path, &pool)
	return pool.BackendPoolHealthCheck, err
}

func (c *GalebClient) UpdatePoolProperties(poolName string, properties BackendPoolHealthCheck) error {
	poolID, err := c.findItemByName("pool", poolName)
	if err != nil {
		return err
	}
	currProperties, err := c.getPoolProperties(poolID)
	if err == nil && currProperties == properties {
		log.Debugf("skipping properties update for pool %q", poolName)
		return nil
	} else if err != nil {
		log.Errorf("ignored error getting pool properties, proceeding with PATH: %v", err)
	}
	path := strings.TrimPrefix(poolID, c.ApiURL)
	var poolParam Pool
	c.fillDefaultPoolValues(&poolParam)
	poolParam.Name = poolName
	poolParam.BackendPoolHealthCheck = properties
	rsp, err := c.doRequest(http.MethodPatch, path, poolParam)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusNoContent {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		return errors.Errorf("PATCH %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	return c.waitStatusOK(poolID)
}

func (c *GalebClient) AddBackends(backends []*url.URL, poolName string, wait bool) error {
	poolID, err := c.findItemByName("pool", poolName)
	if err != nil {
		return err
	}
	return DoLimited(context.Background(), c.MaxRequests, len(backends), func(i int) error {
		var params Target
		params.Name = backends[i].String()
		params.BackendPool = poolID
		resource, cerr := c.doCreateResource("/target", &params)
		if cerr != nil {
			if _, ok := cerr.(ErrItemAlreadyExists); ok {
				return nil
			}
			return cerr
		}
		if wait {
			cerr = c.waitStatusOK(resource)
			if cerr != nil {
				c.removeResource(resource)
				return cerr
			}
		}
		return nil
	})
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
	params.BackendPool = []string{poolID}
	return c.doCreateResource("/rule", &params)
}

func (c *GalebClient) setRuleVirtualHostIDs(ruleID, virtualHostID string, wait bool) error {
	virtualHostGroupId, err := c.FindVirtualHostGroupByVirtualHostId(virtualHostID)
	if err != nil {
		return err
	}

	var params RuleOrdered
	c.fillDefaultRuleOrderedValues(&params)
	params.Rule = ruleID
	params.VirtualHostGroup = fmt.Sprintf("%s/virtualhostgroup/%d", c.ApiURL, virtualHostGroupId)

	resource, err := c.doCreateResource("/ruleordered", &params)
	if err != nil {
		return err
	}
	if wait {
		return c.waitStatusOK(resource)
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

func (c *GalebClient) RemoveResourceByID(resourceID string) error {
	resource, err := c.removeResource(resourceID)
	if err != nil {
		return err
	}
	err = c.waitStatusOK(resource)
	return err
}

func (c *GalebClient) RemoveResourcesByIDs(resourceIDs []string, wait bool) error {
	return DoLimited(context.Background(), c.MaxRequests, len(resourceIDs), func(i int) error {
		resource, err := c.removeResource(resourceIDs[i])
		if err != nil {
			return err
		}
		if wait {
			return c.waitStatusOK(resource)
		}
		return nil
	})
}

func (c *GalebClient) RemoveBackendPool(poolName string) error {
	id, err := c.findItemByName("pool", poolName)
	if err != nil {
		return err
	}
	return c.RemoveResourceByID(id)
}

func (c *GalebClient) RemoveVirtualHost(virtualHostName string) error {
	id, err := c.findItemByName("virtualhost", virtualHostName)
	if err != nil {
		return err
	}
	return c.RemoveResourceByID(id)
}

func (c *GalebClient) RemoveRule(ruleName string) error {
	ruleID, err := c.findItemByName("rule", ruleName)
	if err != nil {
		return err
	}
	return c.RemoveResourceByID(ruleID)
}

func (c *GalebClient) RemoveRulesOrderedByRule(ruleName string) error {
	_, ruleID, err := c.findItemIDsByName("rule", ruleName)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/rule/%d/rulesOrdered", ruleID)
	var rspObj struct {
		Embedded struct {
			RuleOrdered []RuleOrdered `json:"ruleordered"`
		} `json:"_embedded"`
	}
	err = c.getObj(path, &rspObj)
	if err != nil {
		return err
	}
	for _, ruleOrdered := range rspObj.Embedded.RuleOrdered {
		fullID := ruleOrdered.FullId()
		err = c.RemoveResourceByID(fullID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *GalebClient) FindVirtualHostGroupByVirtualHostId(virtualHostId string) (int, error) {
	path := fmt.Sprintf("%s/virtualhostgroup", strings.TrimPrefix(virtualHostId, c.ApiURL))
	var rspObj struct {
		VirtualHostGroupId int `json:"id"`
	}
	err := c.getObj(path, &rspObj)
	if err != nil {
		return 0, err
	}
	return rspObj.VirtualHostGroupId, nil
}

func (c *GalebClient) FindTargetsByPool(poolName string) ([]Target, error) {
	_, poolID, err := c.findItemIDsByName("pool", poolName)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/pool/%d/targets", poolID)
	var rspObj struct {
		Embedded struct {
			Targets []Target `json:"target"`
		} `json:"_embedded"`
	}
	err = c.getObj(path, &rspObj)
	if err != nil {
		return nil, err
	}
	return rspObj.Embedded.Targets, nil
}

func (c *GalebClient) FindVirtualHostsByGroup(virtualHostName string) ([]VirtualHost, error) {
	virtualHostID, err := c.findItemByName("virtualhost", virtualHostName)
	if err != nil {
		return nil, err
	}
	virtualHostGroupId, err := c.FindVirtualHostGroupByVirtualHostId(virtualHostID)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/virtualhostgroup/%d/virtualhosts", virtualHostGroupId)
	var rspObj struct {
		Embedded struct {
			VirtualHosts []VirtualHost `json:"virtualhost"`
		} `json:"_embedded"`
	}
	err = c.getObj(path, &rspObj)
	if err != nil {
		return nil, err
	}
	return rspObj.Embedded.VirtualHosts, nil

}

func (c *GalebClient) Healthcheck() error {
	rsp, err := c.doRequest(http.MethodGet, "/healthcheck", nil)
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

func (c *GalebClient) removeResource(resourceURI string) (string, error) {
	path := strings.TrimPrefix(resourceURI, c.ApiURL)
	rsp, err := c.doRequest(http.MethodDelete, path, nil)
	if err != nil {
		return "", err
	}
	defer rsp.Body.Close()
	responseData, _ := ioutil.ReadAll(rsp.Body)

	if rsp.StatusCode != http.StatusNoContent {
		return "", errors.Errorf("DELETE %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	return path, nil
}

func (c *GalebClient) findItemByName(item, name string) (string, error) {
	idStr, _, err := c.findItemIDsByName(item, name)
	return idStr, err
}

func (c *GalebClient) findItemIDsByName(item, name string) (string, int, error) {
	path := fmt.Sprintf("/%s/search/findByName?name=%s", item, name)
	var rspObj struct {
		Embedded map[string][]commonPostResponse `json:"_embedded"`
	}
	err := c.getObj(path, &rspObj)
	if err != nil {
		return "", 0, err
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

func (c *GalebClient) fetchPathStatus(path string) (map[string]string, int, error) {
	rsp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "GET %s: unable to make request", path)
	}
	defer rsp.Body.Close()
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusOK && rsp.StatusCode != http.StatusNotFound {
		return nil, -1, errors.Errorf("GET %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	if rsp.StatusCode == http.StatusNotFound {
		return nil, http.StatusNotFound, nil
	}
	var response commonPostResponse
	err = json.Unmarshal(responseData, &response)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "GET %s: unable to unmarshal response. data: %s", path, string(responseData))
	}
	return response.Status, rsp.StatusCode, nil
}

func (c *GalebClient) waitStatusOK(resourceURI string) error {
	path := strings.TrimPrefix(resourceURI, c.ApiURL)
	var timeout <-chan time.Time
	if c.WaitTimeout != 0 {
		timeout = time.After(c.WaitTimeout)
	}
	var mapStatus map[string]string
	var err error
	var statusCode int
loop:
	for {
		mapStatus, statusCode, err = c.fetchPathStatus(path)
		if err != nil {
			break
		}
		if c.containsStatus(mapStatus, STATUS_OK) || statusCode == http.StatusNotFound {
			return nil
		}
		select {
		case <-timeout:
			stringStatus, _ := json.Marshal(mapStatus)
			err = errors.Errorf("GET %s: timeout after %v waiting for status change from %s", path, c.WaitTimeout, stringStatus)
			break loop
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
	if err != nil {
		return err
	}
	return errors.Errorf("GET %s: invalid status %s", path, mapStatus)
}

func (c *GalebClient) containsStatus(status map[string]string, statusCheck string) (contains bool) {
	for _, value := range status {
		if value != statusCheck {
			return false
		}
	}
	return true
}

func (c *GalebClient) getObj(path string, data interface{}) error {
	rsp, err := c.doRequest(http.MethodGet, path, nil)
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

func DoLimited(ctx context.Context, limit, n int, fn func(i int) error) error {
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, n)
	wg := sync.WaitGroup{}
	var limiter chan struct{}
	if limit > 0 {
		limiter = make(chan struct{}, limit)
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case <-cancelCtx.Done():
				return
			default:
			}
			if limiter != nil {
				select {
				case limiter <- struct{}{}:
				case <-cancelCtx.Done():
					return
				}
				defer func() { <-limiter }()
			}
			err := fn(i)
			if err != nil {
				cancel()
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	return <-errCh
}
