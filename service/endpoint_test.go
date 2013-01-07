// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	stderrors "errors"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/errors"
	"io/ioutil"
	. "launchpad.net/gocheck"
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

func failHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Server failed to do its job."))
}

type TestHandler struct {
	body   []byte
	method string
	url    string
	sync.Mutex
}

func (h *TestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Lock()
	defer h.Unlock()
	content := `{"MYSQL_DATABASE_NAME": "CHICO", "MYSQL_HOST": "localhost", "MYSQL_PORT": "3306"}`
	h.method = r.Method
	h.url = r.URL.String()
	h.body, _ = ioutil.ReadAll(r.Body)
	w.Write([]byte(content))
}

func (s *S) TestCreateShouldSendTheNameOfTheResourceToTheEndpoint(c *C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Create(&instance)
	c.Assert(err, IsNil)
	expectedUrl := "/resources"
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, Equals, expectedUrl)
	c.Assert(h.method, Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, IsNil)
	c.Assert(map[string][]string(v), DeepEquals, map[string][]string{"name": {"my-redis"}})
}

func (s *S) TestCreateShouldReturnErrorIfTheRequestFail(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Create(&instance)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Failed to create the instance "+instance.Name+": Server failed to do its job.$")
}

func (s *S) TestDestroyShouldSendADELETERequestToTheResourceURL(c *C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Destroy(&instance)
	h.Lock()
	defer h.Unlock()
	c.Assert(err, IsNil)
	c.Assert(h.url, Equals, "/resources/"+instance.Name)
	c.Assert(h.method, Equals, "DELETE")
}

func (s *S) TestDestroyShouldReturnErrorIfTheRequestFails(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Destroy(&instance)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Failed to destroy the instance "+instance.Name+": Server failed to do its job.$")
}

func (s *S) TestBindShouldSendAPOSTToTheResourceURL(c *C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := FakeApp{
		name: "her-app",
		ip:   "10.0.10.1",
	}
	client := &Client{endpoint: ts.URL}
	_, err := client.Bind(&instance, a.GetUnits()[0])
	h.Lock()
	defer h.Unlock()
	c.Assert(err, IsNil)
	c.Assert(h.url, Equals, "/resources/"+instance.Name)
	c.Assert(h.method, Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, IsNil)
	c.Assert(map[string][]string(v), DeepEquals, map[string][]string{"hostname": {"10.0.10.1"}})
}

func (s *S) TestBindShouldReturnMapWithTheEnvironmentVariable(c *C) {
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
	env, err := client.Bind(&instance, a.GetUnits()[0])
	c.Assert(err, IsNil)
	c.Assert(env, DeepEquals, expected)
}

func (s *S) TestBindShouldReturnErrorIfTheRequestFail(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := FakeApp{
		name: "her-app",
		ip:   "10.0.10.1",
	}
	client := &Client{endpoint: ts.URL}
	_, err := client.Bind(&instance, a.GetUnits()[0])
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Failed to bind instance her-redis to the unit 10.0.10.1: Server failed to do its job.$")
}

func (s *S) TestBindShouldReturnPreconditionFailedIfServiceAPIReturnPreconditionFailed(c *C) {
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
	_, err := client.Bind(&instance, a.GetUnits()[0])
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Message, Equals, "You cannot bind any app to this service instance because it is not ready yet.")
}

func (s *S) TestUnbindSendADELETERequestToTheResourceURL(c *C) {
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
	c.Assert(err, IsNil)
	c.Assert(h.url, Equals, "/resources/heaven-can-wait/hostname/2.2.2.2")
	c.Assert(h.method, Equals, "DELETE")
}

func (s *S) TestUnbindReturnsErrorIfTheRequestFails(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven"}
	a := FakeApp{
		name: "arch-enemy",
		ip:   "2.2.2.2",
	}
	client := &Client{endpoint: ts.URL}
	err := client.Unbind(&instance, a.GetUnits()[0])
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Failed to unbind instance heaven-can-wait from the unit 2.2.2.2: Server failed to do its job.$")
}

func (s *S) TestBuildErrorMessageWithNilResponse(c *C) {
	cli := Client{}
	err := stderrors.New("epic fail")
	c.Assert(cli.buildErrorMessage(err, nil), Equals, "epic fail")
}

func (s *S) TestBuildErrorMessageWithNilErrorAndNilResponse(c *C) {
	cli := Client{}
	c.Assert(cli.buildErrorMessage(nil, nil), Equals, "")
}

func (s *S) TestBuildErrorMessageWithNonNilResponseAndNilError(c *C) {
	cli := Client{}
	body := strings.NewReader("something went wrong")
	resp := &http.Response{Body: ioutil.NopCloser(body)}
	c.Assert(cli.buildErrorMessage(nil, resp), Equals, "something went wrong")
}

func (s *S) TestBuildErrorMessageWithNonNilResponseAndNonNilError(c *C) {
	cli := Client{}
	err := stderrors.New("epic fail")
	body := strings.NewReader("something went wrong")
	resp := &http.Response{Body: ioutil.NopCloser(body)}
	c.Assert(cli.buildErrorMessage(err, resp), Equals, "epic fail")
}

func (s *S) TestStatusShouldSendTheNameAndHostOfTheService(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(noContentHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	state, err := client.Status(&instance)
	c.Assert(err, IsNil)
	c.Assert(state, Equals, "up")
}

func (s *S) TestStatusShouldReturnDownWhenApiReturns500(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	state, err := client.Status(&instance)
	c.Assert(err, IsNil)
	c.Assert(state, Equals, "down")
}

func (s *S) TestStatusShouldReturnPendingWhenApiReturns202(c *C) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	ts := httptest.NewServer(h)
	defer ts.Close()
	instance := ServiceInstance{Name: "hi_there", ServiceName: "redis"}
	client := Client{endpoint: ts.URL}
	state, err := client.Status(&instance)
	c.Assert(err, IsNil)
	c.Assert(state, Equals, "pending")
}
