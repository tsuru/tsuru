// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type Client struct {
	endpoint string
}

func (c *Client) buildErrorMessage(err error, resp *http.Response) string {
	if err != nil {
		return err.Error()
	}
	if resp != nil {
		b, _ := ioutil.ReadAll(resp.Body)
		return string(b)
	}
	return ""
}

func (c *Client) issueRequest(path, method string, params map[string][]string) (*http.Response, error) {
	log.Debug("Issuing request...")
	v := url.Values(params)
	var suffix string
	var body io.Reader
	if method == "DELETE" || method == "GET" {
		suffix = "?" + v.Encode()
	} else {
		body = strings.NewReader(v.Encode())
	}
	url := strings.TrimRight(c.endpoint, "/") + "/" + strings.Trim(path, "/") + suffix
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		log.Errorf("Got error while creating request: %s", err)
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	return http.DefaultClient.Do(req)
}

func (c *Client) jsonFromResponse(resp *http.Response, v interface{}) error {
	log.Debug("Parsing response json...")
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Got error while parsing json: %s", err)
		return err
	}
	return json.Unmarshal(body, &v)
}

func (c *Client) Create(instance *ServiceInstance) error {
	var err error
	log.Debug("Attempting to call creation of service instance " + instance.Name + " at " + instance.ServiceName + " api")
	var resp *http.Response
	params := map[string][]string{
		"name": {instance.Name},
	}
	if resp, err = c.issueRequest("/resources", "POST", params); err == nil && resp.StatusCode < 300 {
		return nil
	}
	msg := "Failed to create the instance " + instance.Name + ": " + c.buildErrorMessage(err, resp)
	log.Error(msg)
	return &errors.HTTP{Code: http.StatusInternalServerError, Message: msg}
}

func (c *Client) Destroy(instance *ServiceInstance) error {
	log.Debug("Attempting to call destroy of service instance " + instance.Name + " at " + instance.ServiceName + " api")
	resp, err := c.issueRequest("/resources/"+instance.Name, "DELETE", nil)
	if err == nil && resp.StatusCode > 299 {
		msg := "Failed to destroy the instance " + instance.Name + ": " + c.buildErrorMessage(err, resp)
		log.Error(msg)
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: msg}
	}
	return err
}

func (c *Client) Bind(instance *ServiceInstance, app bind.App, unit bind.Unit) (map[string]string, error) {
	log.Debugf("Calling bind of instance %q and unit %q at %q API",
		instace.Name, unit.GetIp(), instance.ServiceName)
	var resp *http.Response
	params := map[string][]string{
		"unit-host": {unit.GetIp()},
		"app-host":  {app.GetIp()},
	}
	resp, err := c.issueRequest("/resources/"+instance.Name, "POST", params)
	if err != nil {
		if m, _ := regexp.MatchString("", err.Error()); m {
			return nil, fmt.Errorf("%s api is down.", instance.Name)
		}
		return nil, err
	}
	if err == nil && resp.StatusCode < 300 {
		var result map[string]string
		err = c.jsonFromResponse(resp, &result)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	if resp.StatusCode == http.StatusPreconditionFailed {
		return nil, &errors.HTTP{Code: resp.StatusCode, Message: "You cannot bind any app to this service instance because it is not ready yet."}
	}
	msg := "Failed to bind instance " + instance.Name + " to the unit " + unit.GetIp() + ": " + c.buildErrorMessage(err, resp)
	log.Error(msg)
	return nil, &errors.HTTP{Code: http.StatusInternalServerError, Message: msg}
}

func (c *Client) Unbind(instance *ServiceInstance, unit bind.Unit) error {
	log.Debug("Attempting to call unbind of service instance " + instance.Name + " and unit " + unit.GetIp() + " at " + instance.ServiceName + " api")
	var resp *http.Response
	url := "/resources/" + instance.Name + "/hostname/" + unit.GetIp()
	resp, err := c.issueRequest(url, "DELETE", nil)
	if err == nil && resp.StatusCode > 299 {
		msg := "Failed to unbind instance " + instance.Name + " from the unit " + unit.GetIp() + ": " + c.buildErrorMessage(err, resp)
		log.Error(msg)
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: msg}
	}
	return err
}

// Connects into service's api
// The api should be prepared to receive the request,
// like below:
// GET /resources/<name>/status/
// The service host here is the private ip of the service instance
// 204 means the service is up, 500 means the service is down
func (c *Client) Status(instance *ServiceInstance) (string, error) {
	log.Debug("Attempting to call status of service instance " + instance.Name + " at " + instance.ServiceName + " api")
	var (
		resp *http.Response
		err  error
	)
	url := "/resources/" + instance.Name + "/status"
	if resp, err = c.issueRequest(url, "GET", nil); err == nil {
		switch resp.StatusCode {
		case 202:
			return "pending", nil
		case 204:
			return "up", nil
		case 500:
			return "down", nil
		}
	}
	msg := "Failed to get status of instance " + instance.Name + ": " + c.buildErrorMessage(err, resp)
	log.Error(msg)
	err = &errors.HTTP{Code: http.StatusInternalServerError, Message: msg}
	return "", err
}

// Info returns the additional info about a service instance.
// The api should be prepared to receive the request,
// like below:
// GET /resources/<name>
func (c *Client) Info(instance *ServiceInstance) ([]map[string]string, error) {
	log.Debug("Attempting to call info of service instance " + instance.Name + " at " + instance.ServiceName + " api")
	url := "/resources/" + instance.Name
	resp, err := c.issueRequest(url, "GET", nil)
	if err != nil || resp.StatusCode != 200 {
		return nil, err
	}
	result := []map[string]string{}
	err = c.jsonFromResponse(resp, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
