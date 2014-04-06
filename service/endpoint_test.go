// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	stderrors "errors"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/errors"
	"io/ioutil"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
)

type FakeUnit struct {
	ip string
}

func (a *FakeUnit) GetIp() string {
	return a.ip
}

type FakeApp struct {
	ip   string
	name string
}

func (a *FakeApp) GetIp() string {
	return a.ip
}

func (a *FakeApp) GetName() string {
	return a.name
}

func (a *FakeApp) GetUnits() []bind.Unit {
	return []bind.Unit{
		&FakeUnit{ip: a.ip},
	}
}

func (a *FakeApp) InstanceEnv(name string) map[string]bind.EnvVar {
	return nil
}

func (a *FakeApp) SetEnvs(vars []bind.EnvVar, public bool) error {
	return nil
}

func (a *FakeApp) UnsetEnvs(vars []string, public bool) error {
	return nil
}

func noContentHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

type infoHandler struct {
	r *http.Request
}

func (h *infoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.r = r
	content := `[{"label": "some label", "value": "some value"}, {"label": "label2.0", "value": "v2"}]`
	w.Write([]byte(content))
}

type plansHandler struct {
	r *http.Request
}

func (h *plansHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.r = r
	content := `[{"name": "ignite", "description": "some value"}, {"name": "small", "description": "not space left for you"}]`
	w.Write([]byte(content))
}

func failHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Server failed to do its job."))
}

type TestHandler struct {
	body    []byte
	method  string
	url     string
	request *http.Request
	sync.Mutex
}

func (h *TestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Lock()
	defer h.Unlock()
	content := `{"MYSQL_DATABASE_NAME": "CHICO", "MYSQL_HOST": "localhost", "MYSQL_PORT": "3306"}`
	h.method = r.Method
	h.url = r.URL.String()
	h.body, _ = ioutil.ReadAll(r.Body)
	h.request = r
	w.Write([]byte(content))
}

func (s *S) TestEndpointCreate(c *gocheck.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Create(&instance)
	c.Assert(err, gocheck.IsNil)
	expectedURL := "/resources"
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, gocheck.Equals, expectedURL)
	c.Assert(h.method, gocheck.Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, gocheck.IsNil)
	c.Assert(map[string][]string(v), gocheck.DeepEquals, map[string][]string{"name": {"my-redis"}})
	c.Assert("application/x-www-form-urlencoded", gocheck.DeepEquals, h.request.Header.Get("Content-Type"))
	c.Assert("application/json", gocheck.Equals, h.request.Header.Get("Accept"))
}

func (s *S) TestEndpointCreatePlans(c *gocheck.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{
		Name:        "my-redis",
		ServiceName: "redis",
		PlanName:    "basic",
	}
	client := &Client{endpoint: ts.URL}
	err := client.Create(&instance)
	c.Assert(err, gocheck.IsNil)
	expectedURL := "/resources"
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, gocheck.Equals, expectedURL)
	c.Assert(h.method, gocheck.Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, gocheck.IsNil)
	c.Assert(map[string][]string(v), gocheck.DeepEquals, map[string][]string{"name": {"my-redis"}, "plan": {"basic"}})
	c.Assert("application/x-www-form-urlencoded", gocheck.DeepEquals, h.request.Header.Get("Content-Type"))
	c.Assert("application/json", gocheck.Equals, h.request.Header.Get("Accept"))
}

func (s *S) TestCreateShouldSendTheNameOfTheResourceToTheEndpoint(c *gocheck.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Create(&instance)
	c.Assert(err, gocheck.IsNil)
	expectedURL := "/resources"
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, gocheck.Equals, expectedURL)
	c.Assert(h.method, gocheck.Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, gocheck.IsNil)
	c.Assert(map[string][]string(v), gocheck.DeepEquals, map[string][]string{"name": {"my-redis"}})
	c.Assert("application/x-www-form-urlencoded", gocheck.DeepEquals, h.request.Header.Get("Content-Type"))
	c.Assert("application/json", gocheck.Equals, h.request.Header.Get("Accept"))
}

func (s *S) TestCreateShouldReturnErrorIfTheRequestFail(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Create(&instance)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Failed to create the instance "+instance.Name+": Server failed to do its job.$")
}

func (s *S) TestDestroyShouldSendADELETERequestToTheResourceURL(c *gocheck.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Destroy(&instance)
	h.Lock()
	defer h.Unlock()
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url, gocheck.Equals, "/resources/"+instance.Name)
	c.Assert(h.method, gocheck.Equals, "DELETE")
}

func (s *S) TestDestroyShouldReturnErrorIfTheRequestFails(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Destroy(&instance)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Failed to destroy the instance "+instance.Name+": Server failed to do its job.$")
}

func (s *S) TestBindWithEndpointDown(c *gocheck.C) {
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := FakeApp{
		name: "her-app",
		ip:   "10.0.10.1",
	}
	// Use http://tools.ietf.org/html/rfc5737 'TEST-NET' to avoid broken
	// resolvers redirecting to a search page.
	client := &Client{endpoint: "http://192.0.2.42"}
	_, err := client.Bind(&instance, &a, a.GetUnits()[0])
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^her-redis api is down.$")
}

func (s *S) TestBindShouldSendAPOSTToTheResourceURL(c *gocheck.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := FakeApp{
		name: "her-app",
		ip:   "10.0.10.1",
	}
	client := &Client{endpoint: ts.URL}
	_, err := client.Bind(&instance, &a, a.GetUnits()[0])
	h.Lock()
	defer h.Unlock()
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url, gocheck.Equals, "/resources/"+instance.Name)
	c.Assert(h.method, gocheck.Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, gocheck.IsNil)
	expected := map[string][]string{"unit-host": {"10.0.10.1"}, "app-host": {"10.0.10.1"}}
	c.Assert(map[string][]string(v), gocheck.DeepEquals, expected)
}

func (s *S) TestBindShouldReturnMapWithTheEnvironmentVariable(c *gocheck.C) {
	expected := map[string]string{
		"MYSQL_DATABASE_NAME": "CHICO",
		"MYSQL_HOST":          "localhost",
		"MYSQL_PORT":          "3306",
	}
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := FakeApp{
		name: "her-app",
		ip:   "10.0.10.1",
	}
	client := &Client{endpoint: ts.URL}
	env, err := client.Bind(&instance, &a, a.GetUnits()[0])
	c.Assert(err, gocheck.IsNil)
	c.Assert(env, gocheck.DeepEquals, expected)
}

func (s *S) TestBindShouldReturnErrorIfTheRequestFail(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := FakeApp{
		name: "her-app",
		ip:   "10.0.10.1",
	}
	client := &Client{endpoint: ts.URL}
	_, err := client.Bind(&instance, &a, a.GetUnits()[0])
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Failed to bind instance her-redis to the unit 10.0.10.1: Server failed to do its job.$")
}

func (s *S) TestBindShouldReturnPreconditionFailedIfServiceAPIReturnPreconditionFailed(c *gocheck.C) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(412)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := FakeApp{
		name: "her-app",
		ip:   "10.0.10.1",
	}
	client := &Client{endpoint: ts.URL}
	_, err := client.Bind(&instance, &a, a.GetUnits()[0])
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Message, gocheck.Equals, "You cannot bind any app to this service instance because it is not ready yet.")
}

func (s *S) TestUnbindSendADELETERequestToTheResourceURL(c *gocheck.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven"}
	a := FakeApp{
		name: "arch-enemy",
		ip:   "2.2.2.2",
	}
	client := &Client{endpoint: ts.URL}
	err := client.Unbind(&instance, a.GetUnits()[0])
	h.Lock()
	defer h.Unlock()
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url, gocheck.Equals, "/resources/heaven-can-wait/hostname/2.2.2.2")
	c.Assert(h.method, gocheck.Equals, "DELETE")
}

func (s *S) TestUnbindReturnsErrorIfTheRequestFails(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven"}
	a := FakeApp{
		name: "arch-enemy",
		ip:   "2.2.2.2",
	}
	client := &Client{endpoint: ts.URL}
	err := client.Unbind(&instance, a.GetUnits()[0])
	c.Assert(err, gocheck.NotNil)
	expected := `Failed to unbind ("/resources/heaven-can-wait/hostname/2.2.2.2"): Server failed to do its job.`
	c.Assert(err.Error(), gocheck.Equals, expected)
}

func (s *S) TestBuildErrorMessageWithNilResponse(c *gocheck.C) {
	cli := Client{}
	err := stderrors.New("epic fail")
	c.Assert(cli.buildErrorMessage(err, nil), gocheck.Equals, "epic fail")
}

func (s *S) TestBuildErrorMessageWithNilErrorAndNilResponse(c *gocheck.C) {
	cli := Client{}
	c.Assert(cli.buildErrorMessage(nil, nil), gocheck.Equals, "")
}

func (s *S) TestBuildErrorMessageWithNonNilResponseAndNilError(c *gocheck.C) {
	cli := Client{}
	body := strings.NewReader("something went wrong")
	resp := &http.Response{Body: ioutil.NopCloser(body)}
	c.Assert(cli.buildErrorMessage(nil, resp), gocheck.Equals, "something went wrong")
}

func (s *S) TestBuildErrorMessageWithNonNilResponseAndNonNilError(c *gocheck.C) {
	cli := Client{}
	err := stderrors.New("epic fail")
	body := strings.NewReader("something went wrong")
	resp := &http.Response{Body: ioutil.NopCloser(body)}
	c.Assert(cli.buildErrorMessage(err, resp), gocheck.Equals, "epic fail")
}

func (s *S) TestStatusShouldSendTheNameAndHostOfTheService(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(noContentHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	state, err := client.Status(&instance)
	c.Assert(err, gocheck.IsNil)
	c.Assert(state, gocheck.Equals, "up")
}

func (s *S) TestStatusShouldReturnDownWhenAPIReturns500(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	state, err := client.Status(&instance)
	c.Assert(err, gocheck.IsNil)
	c.Assert(state, gocheck.Equals, "down")
}

func (s *S) TestStatusShouldReturnPendingWhenAPIReturns202(c *gocheck.C) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	ts := httptest.NewServer(h)
	defer ts.Close()
	instance := ServiceInstance{Name: "hi_there", ServiceName: "redis"}
	client := Client{endpoint: ts.URL}
	state, err := client.Status(&instance)
	c.Assert(err, gocheck.IsNil)
	c.Assert(state, gocheck.Equals, "pending")
}

func (s *S) TestInfo(c *gocheck.C) {
	h := infoHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	result, err := client.Info(&instance)
	c.Assert(err, gocheck.IsNil)
	expected := []map[string]string{
		{"label": "some label", "value": "some value"},
		{"label": "label2.0", "value": "v2"},
	}
	c.Assert(result, gocheck.DeepEquals, expected)
	c.Assert(h.r.URL.Path, gocheck.Equals, "/resources/my-redis")
}

func (s *S) TestInfoNotFound(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(notFoundHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	result, err := client.Info(&instance)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.IsNil)
}

func (s *S) TestPlans(c *gocheck.C) {
	h := plansHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := &Client{endpoint: ts.URL}
	result, err := client.Plans()
	c.Assert(err, gocheck.IsNil)
	expected := []Plan{
		{Name: "ignite", Description: "some value"},
		{Name: "small", Description: "not space left for you"},
	}
	c.Assert(result, gocheck.DeepEquals, expected)
	c.Assert(h.r.URL.Path, gocheck.Equals, "/resources/plans")
}
