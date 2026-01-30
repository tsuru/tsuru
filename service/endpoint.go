// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/cezarsa/form"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	poolMultiCluster "github.com/tsuru/tsuru/provision/pool/multicluster"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	provTypes "github.com/tsuru/tsuru/types/provision"
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
		"bind-job",
		"bind",
	}
)

func init() {
	prometheus.MustRegister(requestLatencies)
	prometheus.MustRegister(requestErrors)
}

var _ ServiceClient = &endpointClient{}

type validationError struct {
	Msg           string   `json:"msg"`
	MissingParams []string `json:"missing_params"`
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
		"user":    {evt.OwnerEmail()},
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
	header, err := baseHeader(ctx, evt, instance, requestID)
	if err != nil {
		return err
	}
	resp, err = c.issueRequest(ctx, "/resources", "POST", params, header)
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
		"user":        {evt.OwnerEmail()},
		"eventid":     {evt.UniqueID.Hex()},
	}

	addParameters(params, instance.Parameters)
	header, err := baseHeader(ctx, evt, instance, requestID)
	if err != nil {
		return err
	}
	resp, err := c.issueRequest(ctx, "/resources/"+instance.GetIdentifier(), "PUT", params, header)
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
		"user":    {evt.OwnerEmail()},
		"eventid": {evt.UniqueID.Hex()},
	}

	header, err := baseHeader(ctx, evt, instance, requestID)
	if err != nil {
		return err
	}
	resp, err := c.issueRequest(ctx, "/resources/"+instance.GetIdentifier(), "DELETE", params, header)
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

func (c *endpointClient) BindApp(ctx context.Context, instance *ServiceInstance, app *appTypes.App, bindParams BindAppParameters, evt *event.Event, requestID string) (map[string]string, error) {
	log.Debugf("Calling bind of instance %q and %q app at %q API",
		instance.Name, app.Name, instance.ServiceName)
	params, err := buildBindAppParams(ctx, evt, app, bindParams)
	if err != nil {
		log.Errorf("Ignoring some errors found while building the bind app parameters: %v", err)
		return nil, err
	}
	header, err := baseHeader(ctx, evt, instance, requestID)
	if err != nil {
		return nil, err
	}
	resp, err := c.issueRequest(ctx, "/resources/"+instance.GetIdentifier()+"/bind-app", "POST", params, header)
	if err != nil {
		return nil, log.WrapError(errors.Wrapf(err, `Failed to bind app %q to service instance "%s/%s"`, app.Name, instance.ServiceName, instance.Name))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		resp, err = c.issueRequest(ctx, "/resources/"+instance.GetIdentifier()+"/bind", "POST", params, header)
	}
	if err != nil {
		return nil, log.WrapError(errors.Wrapf(err, `Failed to bind app %q to service instance "%s/%s"`, app.Name, instance.ServiceName, instance.Name))
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
	err = errors.Wrapf(c.buildErrorMessage(err, resp), `Failed to bind the instance "%s/%s" to the app %q`, instance.ServiceName, instance.Name, app.Name)
	return nil, log.WrapError(err)
}

func (c *endpointClient) BindJob(ctx context.Context, instance *ServiceInstance, job *jobTypes.Job, evt *event.Event, requestID string) (map[string]string, error) {
	log.Debugf("Calling bind of instance %q and %q job at %q API",
		instance.Name, job.Name, instance.ServiceName)

	params, err := buildBindJobParams(ctx, evt, job)
	if err != nil {
		log.Errorf("Errors found while building the bind job parameters: %v", err)
		return nil, err
	}

	header, err := baseHeader(ctx, evt, instance, requestID)
	if err != nil {
		return nil, err
	}
	resp, err := c.issueRequest(ctx, "/resources/"+instance.GetIdentifier()+"/binds/jobs/"+job.Name, http.MethodPut, params, header)
	if err != nil {
		return nil, log.WrapError(errors.Wrapf(err, `Failed to bind job %q to service instance "%s/%s"`, job.Name, instance.ServiceName, instance.Name))
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusPreconditionFailed:
		return nil, ErrInstanceNotReady
	case http.StatusNotFound:
		return nil, ErrInstanceNotFoundInAPI
	}

	if resp.StatusCode >= 300 {
		err = errors.Wrapf(c.buildErrorMessage(err, resp), `Failed to bind the instance "%s/%s" to the job %q`, instance.ServiceName, instance.Name, job.Name)
		return nil, log.WrapError(err)
	}

	var result map[string]string
	err = c.jsonFromResponse(resp, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *endpointClient) UnbindApp(ctx context.Context, instance *ServiceInstance, app *appTypes.App, evt *event.Event, requestID string) error {
	log.Debugf("Calling unbind of service instance %q and app %q at %q", instance.Name, app.Name, instance.ServiceName)
	appAddrs, err := servicemanager.App.GetAddresses(ctx, app)
	if err != nil {
		return err
	}
	url := "/resources/" + instance.GetIdentifier() + "/bind-app"
	params := map[string][]string{
		"app-hosts": appAddrs,
		"app-name":  {app.Name},
		"user":      {evt.OwnerEmail()},
		"eventid":   {evt.UniqueID.Hex()},
	}

	if len(appAddrs) > 0 {
		params["app-host"] = []string{appAddrs[0]}
	}
	header, err := baseHeader(ctx, evt, instance, requestID)
	if err != nil {
		return err
	}
	resp, err := c.issueRequest(ctx, url, "DELETE", params, header)
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

func (c *endpointClient) UnbindJob(ctx context.Context, instance *ServiceInstance, job *jobTypes.Job, evt *event.Event, requestID string) error {
	log.Debugf("Calling unbind of service instance %q and job %q at %q", instance.Name, job.Name, instance.ServiceName)

	url := "/resources/" + instance.GetIdentifier() + "/binds/jobs/" + job.Name

	header, err := baseHeader(ctx, evt, instance, requestID)
	if err != nil {
		return err
	}

	resp, err := c.issueRequest(ctx, url, http.MethodDelete, nil, header)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		if resp.StatusCode == http.StatusNotFound {
			return ErrInstanceNotFoundInAPI
		}

		err = errors.Wrapf(c.buildErrorMessage(err, resp), "Failed to unbind (%q)", url)
		return log.WrapError(err)
	}

	return nil
}

func (c *endpointClient) Status(ctx context.Context, instance *ServiceInstance, requestID string) (string, error) {
	log.Debugf("Attempting to call status of service instance %q at %q api", instance.Name, instance.ServiceName)
	var (
		resp *http.Response
		err  error
	)
	header, err := baseHeader(ctx, nil, instance, requestID)
	if err != nil {
		return "", err
	}
	url := "/resources/" + instance.GetIdentifier() + "/status"
	if resp, err = c.issueRequest(ctx, url, "GET", nil, header); err == nil {
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
			var data []byte
			data, err = io.ReadAll(resp.Body)
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
	header, err := baseHeader(ctx, nil, instance, requestID)
	if err != nil {
		return nil, err
	}
	url := "/resources/" + instance.GetIdentifier()
	resp, err := c.issueRequest(ctx, url, "GET", nil, header)
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
func (c *endpointClient) Plans(ctx context.Context, pool, requestID string) ([]Plan, error) {
	header, err := baseHeader(ctx, nil, nil, requestID)
	if err != nil {
		return nil, err
	}
	if pool != "" {
		header, err = poolMultiCluster.Header(ctx, pool, header)
		if err != nil {
			return nil, err
		}
	}
	url := "/resources/plans"
	resp, err := c.issueRequest(ctx, url, "GET", nil, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest {
		validationErr := &validationError{}
		err = c.jsonFromResponse(resp, &validationErr)
		if err == nil {
			for _, param := range validationErr.MissingParams {
				if param == "cluster" {
					return nil, ErrMissingPool
				}
			}
		}
	}
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
func (c *endpointClient) Proxy(ctx context.Context, opts *ProxyOpts) error {
	q := opts.Request.URL.Query()
	delete(q, "callback")
	delete(q, ":service")           // injected as named param by DelayedRouter
	delete(q, ":instance")          // injected as named param by DelayedRouter
	delete(q, ":path")              // injected as named param by DelayedRouter
	delete(q, ":version")           // injected as named param by DelayedRouter
	delete(q, ":mux-route-name")    // injected as named param by DelayedRouter
	delete(q, ":mux-path-template") // injected as named param by DelayedRouter
	qstring := q.Encode()
	if qstring != "" {
		qstring = fmt.Sprintf("?%s", qstring)
	}
	rawurl := strings.TrimRight(c.endpoint, "/") + "/" + strings.Trim(opts.Path, "/") + qstring
	url, err := url.Parse(rawurl)
	if err != nil {
		log.Errorf("Got error while creating service proxy url %s: %s", rawurl, err)
		return err
	}
	header, err := baseHeader(ctx, opts.Event, opts.Instance, opts.RequestID)
	if err != nil {
		return err
	}
	for k, v := range header {
		opts.Request.Header[k] = v
	}
	director := func(req *http.Request) {
		req.Header = opts.Request.Header
		req.SetBasicAuth(c.username, c.password)
		req.Host = url.Host
		req.URL = url
		*req = *req.WithContext(ctx)
	}
	proxy := &httputil.ReverseProxy{
		Transport: net.Dial15Full300ClientWithPool.Transport,
		Director:  director,
	}
	proxy.ServeHTTP(opts.Writer, opts.Request)
	return nil
}

func (c *endpointClient) buildErrorMessage(err error, resp *http.Response) error {
	if err != nil {
		return err
	}
	if resp != nil {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return errors.Errorf("invalid response: %s (code: %d)", string(b), resp.StatusCode)
	}
	return nil
}

func (c *endpointClient) issueRequest(ctx context.Context, path, method string, params map[string][]string, header http.Header) (*http.Response, error) {
	var suffix string
	var body io.Reader = nil

	if params != nil {
		v := url.Values(params)

		if method == "GET" {
			suffix = "?" + v.Encode()
		} else {
			body = strings.NewReader(v.Encode())
		}
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
	req.Header = header
	req.Header.Set("Accept", "application/json")
	if method != "GET" && method != "HEAD" && method != "OPTIONS" {
		header.Set("Content-Type", "application/x-www-form-urlencoded")
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
	err := json.NewDecoder(resp.Body).Decode(v)
	if err != nil {
		log.Errorf("Got error while parsing service json: %s", err)
		return err
	}
	return nil
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

func baseHeader(ctx context.Context, evt *event.Event, si *ServiceInstance, requestID string) (http.Header, error) {
	header := make(http.Header)
	if evt != nil {
		header.Set("X-Tsuru-User", evt.OwnerEmail())
		header.Set("X-Tsuru-Eventid", evt.UniqueID.Hex())
	}
	requestIDHeader, _ := config.GetString("request-id-header")
	if requestIDHeader != "" && requestID != "" {
		header.Set(requestIDHeader, requestID)
	}

	if si == nil || si.Pool == "" {
		return header, nil
	}

	return poolMultiCluster.Header(ctx, si.Pool, header)
}

func buildBindAppParams(ctx context.Context, evt *event.Event, app *appTypes.App, bindParams BindAppParameters) (url.Values, error) {
	if app == nil {
		return nil, errors.New("app cannot be nil")
	}
	params := url.Values{}
	addParameters(params, bindParams)
	params.Set("app-name", app.Name)
	if evt != nil {
		params.Set("user", evt.OwnerEmail())
		params.Set("eventid", evt.UniqueID.Hex())
	}
	appAddrs, err := servicemanager.App.GetAddresses(ctx, app)
	if err != nil {
		return nil, err
	}
	params["app-hosts"] = appAddrs
	if len(appAddrs) > 0 {
		params.Set("app-host", appAddrs[0])
	}
	internalAddrs, err := servicemanager.App.GetInternalBindableAddresses(ctx, app)
	if err != nil {
		return nil, err
	}
	params["app-internal-hosts"] = internalAddrs

	p, err := servicemanager.Pool.FindByName(ctx, app.Pool)
	if err != nil {
		if err == provTypes.ErrPoolNotFound {
			return params, nil
		}
		return nil, err
	}
	if p == nil {
		return params, nil
	}
	params.Set("app-pool-name", p.Name)
	params.Set("app-pool-provisioner", p.Provisioner)
	c, err := servicemanager.Cluster.FindByPool(ctx, p.Provisioner, p.Name)
	if err != nil || c == nil {
		return params, nil
	}
	params.Set("app-cluster-name", c.Name)
	params.Set("app-cluster-provisioner", c.Provisioner)
	for _, addr := range c.Addresses {
		params.Add("app-cluster-addresses", addr)
	}
	return params, nil
}

func buildBindJobParams(ctx context.Context, evt *event.Event, job *jobTypes.Job) (url.Values, error) {
	if job == nil {
		return nil, errors.New("job cannot be nil")
	}

	params := url.Values{}
	params.Set("job-name", job.Name)
	if evt != nil {
		params.Set("user", evt.OwnerEmail())
		params.Set("eventid", evt.UniqueID.Hex())
	}

	p, err := servicemanager.Pool.FindByName(ctx, job.Pool)
	if err != nil {
		if err == provTypes.ErrPoolNotFound {
			return params, nil
		}
		return nil, err
	}
	if p == nil {
		return params, nil
	}
	params.Set("job-pool-name", p.Name)

	c, err := servicemanager.Cluster.FindByPool(ctx, p.Provisioner, p.Name)
	if err != nil || c == nil {
		return params, nil
	}
	params.Set("job-cluster-name", c.Name)

	return params, nil
}
