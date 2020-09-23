// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/ajg/form"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
)

var (
	ErrInstanceAlreadyExistsInAPI = errors.New("instance already exists in the service API")
	ErrInstanceNotFoundInAPI      = errors.New("instance does not exist in the service API")
	ErrInstanceNotReady           = errors.New("instance is not ready yet")

	requestLatencies = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "tsuru_service_request_duration_seconds",
		Help: "The service requests latency distributions.",
	}, []string{"service"})
	requestErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_service_request_errors_total",
		Help: "The total number of service request errors.",
	}, []string{"service"})

	reservedProxyPaths = []string{
		"",
		"bind-app",
		"bind",
	}
)

func init() {
	prometheus.MustRegister(requestLatencies)
	prometheus.MustRegister(requestErrors)
}

type endpointClient struct {
	serviceName string
	endpoint    string
	username    string
	password    string
}

func (c *endpointClient) Create(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error {
	var err error
	var resp *http.Response
	params := map[string][]string{
		"name":    {instance.Name},
		"team":    {instance.TeamOwner},
		"user":    {evt.Owner.Name},
		"eventid": {evt.UniqueID.Hex()},
		"tags":    instance.Tags,
	}
	if instance.PlanName != "" {
		params["plan"] = []string{instance.PlanName}
	}
	if instance.Description != "" {
		params["description"] = []string{instance.Description}
	}
	addParameters(params, instance.Parameters)
	log.Debugf("Attempting to call creation of service instance for %q, params: %#v", instance.ServiceName, params)
	resp, err = c.issueRequest(ctx, "/resources", "POST", params, requestID)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode < 300 {
			return nil
		}
		if resp.StatusCode == http.StatusConflict {
			return ErrInstanceAlreadyExistsInAPI
		}
	}
	err = errors.Wrapf(c.buildErrorMessage(err, resp), "Failed to create the instance %s", instance.Name)
	return log.WrapError(err)
}

func (c *endpointClient) Update(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error {
	log.Debugf("Attempting to call update of service instance %q at %q api", instance.Name, instance.ServiceName)
	params := map[string][]string{
		"description": {instance.Description},
		"team":        {instance.TeamOwner},
		"tags":        instance.Tags,
		"plan":        {instance.PlanName},
		"user":        {evt.Owner.Name},
		"eventid":     {evt.UniqueID.Hex()},
	}
	addParameters(params, instance.Parameters)
	resp, err := c.issueRequest(ctx, "/resources/"+instance.GetIdentifier(), "PUT", params, requestID)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode > 299 {
			if resp.StatusCode == http.StatusNotFound {
				return nil
			}
			err = errors.Wrapf(c.buildErrorMessage(err, resp), "Failed to update the instance %s", instance.Name)
			return log.WrapError(err)
		}
	}
	return err
}

func (c *endpointClient) Destroy(ctx context.Context, instance *ServiceInstance, evt *event.Event, requestID string) error {
	log.Debugf("Attempting to call destroy of service instance %q at %q api", instance.Name, instance.ServiceName)
	params := map[string][]string{
		"user":    {evt.Owner.Name},
		"eventid": {evt.UniqueID.Hex()},
	}
	resp, err := c.issueRequest(ctx, "/resources/"+instance.GetIdentifier(), "DELETE", params, requestID)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode > 299 {
			if resp.StatusCode == http.StatusNotFound {
				return ErrInstanceNotFoundInAPI
			}
			err = errors.Wrapf(c.buildErrorMessage(err, resp), "Failed to destroy the instance %s", instance.Name)
			return log.WrapError(err)
		}
	}
	return err
}

func (c *endpointClient) BindApp(ctx context.Context, instance *ServiceInstance, app bind.App, bindParams BindAppParameters, evt *event.Event, requestID string) (map[string]string, error) {
	log.Debugf("Calling bind of instance %q and %q app at %q API",
		instance.Name, app.GetName(), instance.ServiceName)
	appAddrs, err := app.GetAddresses()
	if err != nil {
		return nil, err
	}
	params := map[string][]string{
		"app-name":  {app.GetName()},
		"app-hosts": appAddrs,
		"user":      {evt.Owner.Name},
		"eventid":   {evt.UniqueID.Hex()},
	}
	addParameters(params, bindParams)
	if len(appAddrs) > 0 {
		params["app-host"] = []string{appAddrs[0]}
	}
	resp, err := c.issueRequest(ctx, "/resources/"+instance.GetIdentifier()+"/bind-app", "POST", params, requestID)
	if err != nil {
		return nil, log.WrapError(errors.Wrapf(err, `Failed to bind app %q to service instance "%s/%s"`, app.GetName(), instance.ServiceName, instance.Name))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		resp, err = c.issueRequest(ctx, "/resources/"+instance.GetIdentifier()+"/bind", "POST", params, requestID)
	}
	if err != nil {
		return nil, log.WrapError(errors.Wrapf(err, `Failed to bind app %q to service instance "%s/%s"`, app.GetName(), instance.ServiceName, instance.Name))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 {
		var result map[string]string
		err = c.jsonFromResponse(resp, &result)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	switch resp.StatusCode {
	case http.StatusPreconditionFailed:
		return nil, ErrInstanceNotReady
	case http.StatusNotFound:
		return nil, ErrInstanceNotFoundInAPI
	}
	err = errors.Wrapf(c.buildErrorMessage(err, resp), `Failed to bind the instance "%s/%s" to the app %q`, instance.ServiceName, instance.Name, app.GetName())
	return nil, log.WrapError(err)
}

func (c *endpointClient) BindUnit(ctx context.Context, instance *ServiceInstance, app bind.App, unit bind.Unit) error {
	log.Debugf("Calling bind of instance %q and %q unit at %q API", instance.Name, unit.GetIp(), instance.ServiceName)
	appAddrs, err := app.GetAddresses()
	if err != nil {
		return err
	}
	params := map[string][]string{
		"app-name":  {app.GetName()},
		"app-hosts": appAddrs,
		"unit-host": {unit.GetIp()},
	}
	if len(appAddrs) > 0 {
		params["app-host"] = []string{appAddrs[0]}
	}
	resp, err := c.issueRequest(ctx, "/resources/"+instance.GetIdentifier()+"/bind", "POST", params, "")
	if err != nil {
		return log.WrapError(errors.Wrapf(err, `Failed to bind the instance "%s/%s" to the unit %q`, instance.ServiceName, instance.Name, unit.GetIp()))
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusPreconditionFailed:
		return ErrInstanceNotReady
	case http.StatusNotFound:
		return ErrInstanceNotFoundInAPI
	}
	if resp.StatusCode > 299 {
		err = errors.Wrapf(c.buildErrorMessage(err, resp), `Failed to bind the instance "%s/%s" to the unit %q`, instance.ServiceName, instance.Name, unit.GetIp())
		return log.WrapError(err)
	}
	return nil
}

func (c *endpointClient) UnbindApp(ctx context.Context, instance *ServiceInstance, app bind.App, evt *event.Event, requestID string) error {
	log.Debugf("Calling unbind of service instance %q and app %q at %q", instance.Name, app.GetName(), instance.ServiceName)
	appAddrs, err := app.GetAddresses()
	if err != nil {
		return err
	}
	url := "/resources/" + instance.GetIdentifier() + "/bind-app"
	params := map[string][]string{
		"app-hosts": appAddrs,
		"app-name":  {app.GetName()},
		"user":      {evt.Owner.Name},
		"eventid":   {evt.UniqueID.Hex()},
	}
	if len(appAddrs) > 0 {
		params["app-host"] = []string{appAddrs[0]}
	}
	resp, err := c.issueRequest(ctx, url, "DELETE", params, requestID)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode > 299 {
			if resp.StatusCode == http.StatusNotFound {
				return ErrInstanceNotFoundInAPI
			}
			err = errors.Wrapf(c.buildErrorMessage(err, resp), "Failed to unbind (%q)", url)
			return log.WrapError(err)
		}
	}
	return err
}

func (c *endpointClient) UnbindUnit(ctx context.Context, instance *ServiceInstance, app bind.App, unit bind.Unit) error {
	log.Debugf("Calling unbind of service instance %q and unit %q at %q", instance.Name, unit.GetIp(), instance.ServiceName)
	appAddrs, err := app.GetAddresses()
	if err != nil {
		return err
	}
	url := "/resources/" + instance.GetIdentifier() + "/bind"
	params := map[string][]string{
		"app-hosts": appAddrs,
		"unit-host": {unit.GetIp()},
	}
	if len(appAddrs) > 0 {
		params["app-host"] = []string{appAddrs[0]}
	}
	resp, err := c.issueRequest(ctx, url, "DELETE", params, "")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode > 299 {
			if resp.StatusCode == http.StatusNotFound {
				return ErrInstanceNotFoundInAPI
			}
			err = errors.Wrapf(c.buildErrorMessage(err, resp), "Failed to unbind (%q)", url)
			return log.WrapError(err)
		}
	}
	return err
}

func (c *endpointClient) Status(ctx context.Context, instance *ServiceInstance, requestID string) (string, error) {
	log.Debugf("Attempting to call status of service instance %q at %q api", instance.Name, instance.ServiceName)
	var (
		resp *http.Response
		err  error
	)
	url := "/resources/" + instance.GetIdentifier() + "/status"
	if resp, err = c.issueRequest(ctx, url, "GET", nil, requestID); err == nil {
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
			var data []byte
			data, err = ioutil.ReadAll(resp.Body)
			return string(data), err
		case http.StatusAccepted:
			return "pending", nil
		case http.StatusNoContent:
			return "up", nil
		case http.StatusNotFound:
			return "not implemented for this service", nil
		case http.StatusInternalServerError:
			return "down", nil
		}
	}
	err = errors.Wrapf(c.buildErrorMessage(err, resp), "Failed to get status of instance %s", instance.Name)
	return "", log.WrapError(err)
}

// Info returns the additional info about a service instance.
// The api should be prepared to receive the request,
// like below:
// GET /resources/<name>
func (c *endpointClient) Info(ctx context.Context, instance *ServiceInstance, requestID string) ([]map[string]string, error) {
	log.Debugf("Attempting to call info of service instance %q at %q api", instance.Name, instance.ServiceName)
	url := "/resources/" + instance.GetIdentifier()
	resp, err := c.issueRequest(ctx, url, "GET", nil, requestID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	result := []map[string]string{}
	err = c.jsonFromResponse(resp, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Plans returns the service plans.
// The api should be prepared to receive the request,
// like below:
// GET /resources/plans
func (c *endpointClient) Plans(ctx context.Context, requestID string) ([]Plan, error) {
	url := "/resources/plans"
	resp, err := c.issueRequest(ctx, url, "GET", nil, requestID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	result := []Plan{}
	err = c.jsonFromResponse(resp, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Proxy is a proxy between tsuru and the service.
// This method allow customized service methods.
func (c *endpointClient) Proxy(ctx context.Context, path string, evt *event.Event, requestID string, w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	delete(q, "callback")
	delete(q, ":service")           // injected as named param by DelayedRouter
	delete(q, ":instance")          // injected as named param by DelayedRouter
	delete(q, ":mux-route-name")    // injected as named param by DelayedRouter
	delete(q, ":mux-path-template") // injected as named param by DelayedRouter
	qstring := q.Encode()
	if qstring != "" {
		qstring = fmt.Sprintf("?%s", qstring)
	}
	rawurl := strings.TrimRight(c.endpoint, "/") + "/" + strings.Trim(path, "/") + qstring
	url, err := url.Parse(rawurl)
	if err != nil {
		log.Errorf("Got error while creating service proxy url %s: %s", rawurl, err)
		return err
	}
	director := func(req *http.Request) {
		if evt != nil {
			req.Header.Set("X-Tsuru-User", evt.Owner.Name)
			req.Header.Set("X-Tsuru-Eventid", evt.UniqueID.Hex())
		}
		requestIDHeader, err := config.GetString("request-id-header")
		if err == nil && requestID != "" && requestIDHeader != "" {
			req.Header.Set(requestIDHeader, requestID)
		}
		req.SetBasicAuth(c.username, c.password)
		req.Host = url.Host
		req.URL = url
	}
	proxy := &httputil.ReverseProxy{
		Director: director,
	}
	proxy.ServeHTTP(w, r)
	return nil
}

func (c *endpointClient) buildErrorMessage(err error, resp *http.Response) error {
	if err != nil {
		return err
	}
	if resp != nil {
		defer resp.Body.Close()
		b, _ := ioutil.ReadAll(resp.Body)
		return errors.Errorf("invalid response: %s (code: %d)", string(b), resp.StatusCode)
	}
	return nil
}

func (c *endpointClient) issueRequest(ctx context.Context, path, method string, params map[string][]string, requestID string) (*http.Response, error) {
	v := url.Values(params)
	var suffix string
	var body io.Reader
	if method == "GET" {
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
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	requestIDHeader, err := config.GetString("request-id-header")
	if err == nil && requestID != "" && requestIDHeader != "" {
		req.Header.Add(requestIDHeader, requestID)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Close = true
	t0 := time.Now()
	resp, err := net.Dial15Full300ClientWithPool.Do(req)
	requestLatencies.WithLabelValues(c.serviceName).Observe(time.Since(t0).Seconds())
	if err != nil {
		requestErrors.WithLabelValues(c.serviceName).Inc()
	}
	return resp, err
}

func (c *endpointClient) jsonFromResponse(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Got error while parsing service json: %s", err)
		return err
	}
	return json.Unmarshal(body, &v)
}

func addParameters(dst url.Values, params map[string]interface{}) {
	if params == nil || dst == nil {
		return
	}
	encoded, err := form.EncodeToValues(params)
	if err != nil {
		errors.Wrapf(err, "unable to encode parameters")
	}
	for key, value := range encoded {
		dst["parameters."+key] = value
	}
}
