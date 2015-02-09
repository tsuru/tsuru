// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"bytes"
	stderrors "errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

type FakeUnit struct {
	ip string
}

func (a *FakeUnit) GetIp() string {
	return a.ip
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

func (s *S) TestEndpointCreate(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis", TeamOwner: "theteam"}
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.Create(&instance, "my@user")
	c.Assert(err, check.IsNil)
	expectedURL := "/resources"
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, check.Equals, expectedURL)
	c.Assert(h.method, check.Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	c.Assert(map[string][]string(v), check.DeepEquals, map[string][]string{
		"name": {"my-redis"},
		"user": {"my@user"},
		"team": {"theteam"},
	})
	c.Assert("application/x-www-form-urlencoded", check.DeepEquals, h.request.Header.Get("Content-Type"))
	c.Assert("application/json", check.Equals, h.request.Header.Get("Accept"))
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	c.Assert("close", check.Equals, h.request.Header.Get("Connection"))
}

func (s *S) TestEndpointCreatePlans(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{
		Name:        "my-redis",
		ServiceName: "redis",
		PlanName:    "basic",
		TeamOwner:   "myteam",
	}
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.Create(&instance, "my@user")
	c.Assert(err, check.IsNil)
	expectedURL := "/resources"
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, check.Equals, expectedURL)
	c.Assert(h.method, check.Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	c.Assert(map[string][]string(v), check.DeepEquals, map[string][]string{
		"name": {"my-redis"},
		"plan": {"basic"},
		"user": {"my@user"},
		"team": {"myteam"},
	})
	c.Assert("application/x-www-form-urlencoded", check.DeepEquals, h.request.Header.Get("Content-Type"))
	c.Assert("application/json", check.Equals, h.request.Header.Get("Accept"))
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	c.Assert("close", check.Equals, h.request.Header.Get("Connection"))
}

func (s *S) TestCreateShouldSendTheNameOfTheResourceToTheEndpoint(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis", TeamOwner: "myteam"}
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.Create(&instance, "my@user")
	c.Assert(err, check.IsNil)
	expectedURL := "/resources"
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, check.Equals, expectedURL)
	c.Assert(h.method, check.Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	c.Assert(map[string][]string(v), check.DeepEquals, map[string][]string{
		"name": {"my-redis"},
		"user": {"my@user"},
		"team": {"myteam"},
	})
	c.Assert("application/x-www-form-urlencoded", check.DeepEquals, h.request.Header.Get("Content-Type"))
	c.Assert("application/json", check.Equals, h.request.Header.Get("Accept"))
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	c.Assert("close", check.Equals, h.request.Header.Get("Connection"))
}

func (s *S) TestCreateShouldReturnErrorIfTheRequestFail(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.Create(&instance, "my@user")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Failed to create the instance "+instance.Name+": Server failed to do its job.$")
}

func (s *S) TestDestroyShouldSendADELETERequestToTheResourceURL(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.Destroy(&instance)
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/resources/"+instance.Name)
	c.Assert(h.method, check.Equals, "DELETE")
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
}

func (s *S) TestDestroyShouldReturnErrorIfTheRequestFails(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.Destroy(&instance)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Failed to destroy the instance "+instance.Name+": Server failed to do its job.$")
}

func (s *S) TestBindAppShouldSendAPOSTToTheResourceURL(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	_, err := client.BindApp(&instance, a)
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/resources/"+instance.Name+"/bind-app")
	c.Assert(h.method, check.Equals, "POST")
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	expected := map[string][]string{"app-host": {a.GetIp()}}
	c.Assert(map[string][]string(v), check.DeepEquals, expected)
}

func (s *S) TestBindAppBackwardCompatible(c *check.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if strings.HasSuffix(r.URL.Path, "bind-app") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var h TestHandler
		h.ServeHTTP(w, r)
	}))
	defer ts.Close()
	expected := map[string]string{
		"MYSQL_DATABASE_NAME": "CHICO",
		"MYSQL_HOST":          "localhost",
		"MYSQL_PORT":          "3306",
	}
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	env, err := client.BindApp(&instance, a)
	c.Assert(err, check.IsNil)
	c.Assert(env, check.DeepEquals, expected)
	c.Assert(atomic.LoadInt32(&calls), check.Equals, int32(2))
}

func (s *S) TestBindAppShouldReturnMapWithTheEnvironmentVariable(c *check.C) {
	expected := map[string]string{
		"MYSQL_DATABASE_NAME": "CHICO",
		"MYSQL_HOST":          "localhost",
		"MYSQL_PORT":          "3306",
	}
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	env, err := client.BindApp(&instance, a)
	c.Assert(err, check.IsNil)
	c.Assert(env, check.DeepEquals, expected)
}

func (s *S) TestBindAppShouldReturnErrorIfTheRequestFail(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	_, err := client.BindApp(&instance, a)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `^Failed to bind the instance "her-redis" to the app "her-app": Server failed to do its job.$`)
}

func (s *S) TestBindAppShouldReturnPreconditionFailedIfServiceAPIReturnPreconditionFailed(c *check.C) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(412)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	_, err := client.BindApp(&instance, a)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Message, check.Equals, "You cannot bind any app to this service instance because it is not ready yet.")
}

func (s *S) TestBindUnit(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.BindUnit(&instance, a, a.GetUnits()[0])
	c.Assert(err, check.IsNil)
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, check.Equals, "/resources/"+instance.Name+"/bind")
	c.Assert(h.method, check.Equals, "POST")
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	expected := map[string][]string{"app-host": {a.GetIp()}, "unit-host": {a.GetUnits()[0].GetIp()}}
	c.Assert(map[string][]string(v), check.DeepEquals, expected)
}

func (s *S) TestBindUnitRequestFailure(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.BindUnit(&instance, a, a.GetUnits()[0])
	c.Assert(err, check.NotNil)
	expectedMsg := `^Failed to bind the instance "her-redis" to the unit "10.10.10.1": Server failed to do its job.$`
	c.Assert(err, check.ErrorMatches, expectedMsg)
}

func (s *S) TestBindUnitPreconditionFailed(c *check.C) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.BindUnit(&instance, a, a.GetUnits()[0])
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Message, check.Equals, "You cannot bind any app to this service instance because it is not ready yet.")
}

func (s *S) TestUnbindApp(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven"}
	a := provisiontest.NewFakeApp("arch-enemy", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.UnbindApp(&instance, a)
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/resources/heaven-can-wait/bind-app")
	c.Assert(h.method, check.Equals, "DELETE")
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	expected := map[string][]string{"app-host": {a.GetIp()}}
	c.Assert(map[string][]string(v), check.DeepEquals, expected)
}

func (s *S) TestUnbindAppRequestFailure(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven"}
	a := provisiontest.NewFakeApp("arch-enemy", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.UnbindApp(&instance, a)
	c.Assert(err, check.NotNil)
	expected := `Failed to unbind ("/resources/heaven-can-wait/bind-app"): Server failed to do its job.`
	c.Assert(err.Error(), check.Equals, expected)
}

func (s *S) TestUnbindUnit(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven"}
	a := provisiontest.NewFakeApp("arch-enemy", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.UnbindUnit(&instance, a, a.GetUnits()[0])
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/resources/heaven-can-wait/bind")
	c.Assert(h.method, check.Equals, "DELETE")
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	expected := map[string][]string{"app-host": {a.GetIp()}, "unit-host": {a.GetUnits()[0].GetIp()}}
	c.Assert(map[string][]string(v), check.DeepEquals, expected)
}

func (s *S) TestUnbindUnitRequestFailure(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven"}
	a := provisiontest.NewFakeApp("arch-enemy", "python", 1)
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	err := client.UnbindUnit(&instance, a, a.GetUnits()[0])
	c.Assert(err, check.NotNil)
	expected := `Failed to unbind ("/resources/heaven-can-wait/bind"): Server failed to do its job.`
	c.Assert(err.Error(), check.Equals, expected)
}

func (s *S) TestBuildErrorMessageWithNilResponse(c *check.C) {
	cli := Client{}
	err := stderrors.New("epic fail")
	c.Assert(cli.buildErrorMessage(err, nil), check.Equals, "epic fail")
}

func (s *S) TestBuildErrorMessageWithNilErrorAndNilResponse(c *check.C) {
	cli := Client{}
	c.Assert(cli.buildErrorMessage(nil, nil), check.Equals, "")
}

func (s *S) TestBuildErrorMessageWithNonNilResponseAndNilError(c *check.C) {
	cli := Client{}
	body := strings.NewReader("something went wrong")
	resp := &http.Response{Body: ioutil.NopCloser(body)}
	c.Assert(cli.buildErrorMessage(nil, resp), check.Equals, "something went wrong")
}

func (s *S) TestBuildErrorMessageWithNonNilResponseAndNonNilError(c *check.C) {
	cli := Client{}
	err := stderrors.New("epic fail")
	body := strings.NewReader("something went wrong")
	resp := &http.Response{Body: ioutil.NopCloser(body)}
	c.Assert(cli.buildErrorMessage(err, resp), check.Equals, "epic fail")
}

func (s *S) TestStatusShouldSendTheNameAndHostOfTheService(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(noContentHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	state, err := client.Status(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(state, check.Equals, "up")
}

func (s *S) TestStatusShouldReturnDownWhenAPIReturns500(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	state, err := client.Status(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(state, check.Equals, "down")
}

func (s *S) TestStatusShouldReturnPendingWhenAPIReturns202(c *check.C) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	ts := httptest.NewServer(h)
	defer ts.Close()
	instance := ServiceInstance{Name: "hi_there", ServiceName: "redis"}
	client := Client{endpoint: ts.URL}
	state, err := client.Status(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(state, check.Equals, "pending")
}

func (s *S) TestInfo(c *check.C) {
	h := infoHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	result, err := client.Info(&instance)
	c.Assert(err, check.IsNil)
	expected := []map[string]string{
		{"label": "some label", "value": "some value"},
		{"label": "label2.0", "value": "v2"},
	}
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(h.r.URL.Path, check.Equals, "/resources/my-redis")
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.r.Header.Get("Authorization"))
}

func (s *S) TestInfoNotFound(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(notFoundHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	result, err := client.Info(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.IsNil)
}

func (s *S) TestPlans(c *check.C) {
	h := plansHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	result, err := client.Plans()
	c.Assert(err, check.IsNil)
	expected := []Plan{
		{Name: "ignite", Description: "some value"},
		{Name: "small", Description: "not space left for you"},
	}
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(h.r.URL.Path, check.Equals, "/resources/plans")
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.r.Header.Get("Authorization"))
}

func (s *S) TestProxy(c *check.C) {
	handlerTest := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
	ts := httptest.NewServer(http.HandlerFunc(handlerTest))
	defer ts.Close()
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = client.Proxy("/backup", recorder, request)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
	client = &Client{endpoint: "http://10.1.2.3:12345", username: "user", password: "abcde"}
	recorder = httptest.NewRecorder()
	err = client.Proxy("/backup", recorder, request)
	c.Assert(recorder.Code, check.Equals, 500)
	c.Assert(recorder.Body.String(), check.Equals, "")
}

func (s *S) TestProxyWithBodyAndHeaders(c *check.C) {
	var proxiedRequest *http.Request
	var readBodyStr []byte
	handlerTest := func(w http.ResponseWriter, r *http.Request) {
		readBodyStr, _ = ioutil.ReadAll(r.Body)
		proxiedRequest = r
		w.WriteHeader(http.StatusNoContent)
	}
	ts := httptest.NewServer(http.HandlerFunc(handlerTest))
	defer ts.Close()
	client := &Client{endpoint: ts.URL, username: "user", password: "abcde"}
	b := bytes.NewBufferString(`{"bla": "bla"}`)
	request, err := http.NewRequest("POST", "http://somewhere.com/", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "text/new-crobuzon")
	recorder := httptest.NewRecorder()
	err = client.Proxy("/backup", recorder, request)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
	c.Assert(proxiedRequest.Header.Get("Content-Type"), check.Equals, "text/new-crobuzon")
	c.Assert(proxiedRequest.Method, check.Equals, "POST")
	c.Assert(proxiedRequest.URL.String(), check.Equals, "/backup")
	tsUrl, err := url.Parse(ts.URL)
	c.Assert(err, check.IsNil)
	c.Assert(proxiedRequest.Host, check.Equals, tsUrl.Host)
	c.Assert(string(readBodyStr), check.Equals, `{"bla": "bla"}`)
}
