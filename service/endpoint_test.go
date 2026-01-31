// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	eventTypes "github.com/tsuru/tsuru/types/event"
	provTypes "github.com/tsuru/tsuru/types/provision"

	check "gopkg.in/check.v1"
)

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
	Err     error
	sync.Mutex
}

func (h *TestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Lock()
	defer h.Unlock()
	content := `{"MYSQL_DATABASE_NAME": "CHICO", "MYSQL_HOST": "localhost", "MYSQL_PORT": "3306"}`
	h.method = r.Method
	h.url = r.URL.String()
	h.body, _ = io.ReadAll(r.Body)
	h.request = r
	if h.Err != nil {
		http.Error(w, h.Err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(content))
}

func createEvt(c *check.C) *event.Event {
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: eventTypes.TargetTypeServiceInstance, Value: "x"},
		Kind:     permission.PermServiceInstanceCreate,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: "my@user"},
		Allowed:  event.Allowed(permission.PermServiceInstanceReadEvents),
	})
	c.Assert(err, check.IsNil)
	return evt
}

func createEvtWithTokenOwner(c *check.C) *event.Event {
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: eventTypes.TargetTypeServiceInstance, Value: "x"},
		Kind:     permission.PermServiceInstanceCreate,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeToken, Name: "my-team-token"},
		Allowed:  event.Allowed(permission.PermServiceInstanceReadEvents),
	})
	c.Assert(err, check.IsNil)
	return evt
}

func (s *S) TestEndpointCreate(c *check.C) {
	config.Set("request-id-header", "Request-ID")
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{
		Name:        "my-redis",
		ServiceName: "redis",
		TeamOwner:   "theteam",
		Description: "xyz",
		Tags:        []string{"tag 1", "tag 2"},
		Parameters: map[string]interface{}{
			"p1": "v1",
			"p2": map[string]interface{}{
				"complex1": "complexvalue1",
			},
		},
	}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Create(context.TODO(), &instance, evt, "Request-ID")
	c.Assert(err, check.IsNil)
	expectedURL := "/resources"
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, check.Equals, expectedURL)
	c.Assert(h.method, check.Equals, http.MethodPost)
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	c.Assert(map[string][]string(v), check.DeepEquals, map[string][]string{
		"name":                   {"my-redis"},
		"user":                   {"my@user"},
		"team":                   {"theteam"},
		"description":            {"xyz"},
		"eventid":                {evt.UniqueID.Hex()},
		"tags":                   {"tag 1", "tag 2"},
		"parameters.p1":          {"v1"},
		"parameters.p2.complex1": {"complexvalue1"},
	})
	c.Assert("Request-ID", check.Equals, h.request.Header.Get("Request-ID"))
	c.Assert("application/x-www-form-urlencoded", check.DeepEquals, h.request.Header.Get("Content-Type"))
	c.Assert("application/json", check.Equals, h.request.Header.Get("Accept"))
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	c.Assert("close", check.Equals, h.request.Header.Get("Connection"))
}

func (s *S) TestEndpointCreateEndpointDown(c *check.C) {
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis", TeamOwner: "theteam", Description: "xyz"}
	client := &endpointClient{endpoint: "http://127.0.0.1:19999", username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Create(context.TODO(), &instance, evt, "Request-ID")
	c.Assert(err, check.ErrorMatches, `Failed to create the instance my-redis: Post .*http://127.0.0.1:19999/resources.*`)
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
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Create(context.TODO(), &instance, evt, "")
	c.Assert(err, check.IsNil)
	expectedURL := "/resources"
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, check.Equals, expectedURL)
	c.Assert(h.method, check.Equals, http.MethodPost)
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	c.Assert(map[string][]string(v), check.DeepEquals, map[string][]string{
		"name":    {"my-redis"},
		"plan":    {"basic"},
		"user":    {"my@user"},
		"team":    {"myteam"},
		"eventid": {evt.UniqueID.Hex()},
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
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Create(context.TODO(), &instance, evt, "")
	c.Assert(err, check.IsNil)
	expectedURL := "/resources"
	h.Lock()
	defer h.Unlock()
	c.Assert(h.url, check.Equals, expectedURL)
	c.Assert(h.method, check.Equals, http.MethodPost)
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	c.Assert(map[string][]string(v), check.DeepEquals, map[string][]string{
		"name":    {"my-redis"},
		"team":    {"myteam"},
		"user":    {"my@user"},
		"eventid": {evt.UniqueID.Hex()},
	})
	c.Assert("application/x-www-form-urlencoded", check.DeepEquals, h.request.Header.Get("Content-Type"))
	c.Assert("application/json", check.Equals, h.request.Header.Get("Accept"))
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	c.Assert("close", check.Equals, h.request.Header.Get("Connection"))
}

func (s *S) TestCreateDuplicate(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Create(context.TODO(), &instance, evt, "")
	c.Assert(err, check.Equals, ErrInstanceAlreadyExistsInAPI)
}

func (s *S) TestCreateShouldReturnErrorIfTheRequestFail(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Create(context.TODO(), &instance, evt, "")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `^Failed to create the instance `+instance.Name+`: invalid response: Server failed to do its job. \(code: 500\)$`)
}

func (s *S) TestEndpointCreateWithTokenOwner(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{
		Name:        "my-redis",
		ServiceName: "redis",
		TeamOwner:   "theteam",
	}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvtWithTokenOwner(c)
	err := client.Create(context.TODO(), &instance, evt, "")
	c.Assert(err, check.IsNil)
	h.Lock()
	defer h.Unlock()
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	c.Assert(v.Get("user"), check.Equals, "my-team-token@tsuru-team-token")
	c.Assert(h.request.Header.Get("X-Tsuru-User"), check.Equals, "my-team-token@tsuru-team-token")
}

func (s *S) TestUpdateShouldSendAPutRequestToTheResourceURL(c *check.C) {
	requestIDHeader := "request-id"
	requestIDValue := "request-id-value"
	config.Set("request-id-header", requestIDHeader)
	defer config.Unset("request-id-header")
	var requests int32
	instance := ServiceInstance{
		Name:        "his-redis",
		ServiceName: "redis",
		TeamOwner:   "team-owner",
		Description: "my service",
		Tags:        []string{"tag1", "tag2"},
		PlanName:    "small",
		Parameters: map[string]interface{}{
			"p1": "v1",
			"p2": map[string]interface{}{
				"complex1": "complexvalue1",
			},
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		c.Check(r.Method, check.Equals, http.MethodPut)
		c.Check(r.URL.Path, check.Equals, "/resources/"+instance.Name)
		r.ParseForm()
		c.Check(r.FormValue("description"), check.Equals, instance.Description)
		c.Check(r.Form["tags"], check.DeepEquals, instance.Tags)
		c.Check(r.FormValue("team"), check.Equals, instance.TeamOwner)
		c.Check(r.FormValue("plan"), check.Equals, instance.PlanName)
		c.Assert(r.FormValue("parameters.p1"), check.Equals, "v1")
		c.Assert(r.FormValue("parameters.p2.complex1"), check.Equals, "complexvalue1")
		c.Check(r.Header.Get(requestIDHeader), check.Equals, requestIDValue)
		c.Assert(r.Header.Get("Authorization"), check.Equals, "Basic dXNlcjphYmNkZQ==")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Update(context.TODO(), &instance, evt, requestIDValue)
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
}

func (s *S) TestUpdateShouldReturnErrorIfTheRequestFails(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Update(context.TODO(), &instance, evt, "")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `Failed to update the instance `+instance.Name+`: invalid response: Server failed to do its job. \(code: 500\)$`)
}

func (s *S) TestUpdateWithTokenOwner(c *check.C) {
	var requests int32
	instance := ServiceInstance{
		Name:        "his-redis",
		ServiceName: "redis",
		TeamOwner:   "team-owner",
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		r.ParseForm()
		c.Check(r.FormValue("user"), check.Equals, "my-team-token@tsuru-team-token")
		c.Check(r.Header.Get("X-Tsuru-User"), check.Equals, "my-team-token@tsuru-team-token")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvtWithTokenOwner(c)
	err := client.Update(context.TODO(), &instance, evt, "")
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
}

func (s *S) TestUpdateShouldIgnoreNotFound(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Update(context.TODO(), &instance, evt, "")
	c.Assert(err, check.IsNil)
}

func (s *S) TestDestroyShouldSendADELETERequestToTheResourceURL(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Destroy(context.TODO(), &instance, evt, "")
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/resources/"+instance.Name)
	c.Assert(h.method, check.Equals, http.MethodDelete)
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
}

func (s *S) TestDestroyShouldReturnErrorIfTheRequestFails(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Destroy(context.TODO(), &instance, evt, "")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `Failed to destroy the instance `+instance.Name+`: invalid response: Server failed to do its job. \(code: 500\)$`)
}

func (s *S) TestDestroyWithTokenOwner(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvtWithTokenOwner(c)
	err := client.Destroy(context.TODO(), &instance, evt, "")
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	c.Assert(v.Get("user"), check.Equals, "my-team-token@tsuru-team-token")
	c.Assert(h.request.Header.Get("X-Tsuru-User"), check.Equals, "my-team-token@tsuru-team-token")
}

func (s *S) TestDestroyNotFound(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.Destroy(context.TODO(), &instance, evt, "")
	c.Assert(err, check.Equals, ErrInstanceNotFoundInAPI)
}

func (s *S) TestBindAppEndpointDown(c *check.C) {
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &endpointClient{endpoint: "http://localhost:1234", username: "user", password: "abcde"}
	evt := createEvt(c)
	_, err := client.BindApp(context.TODO(), &instance, a, nil, evt, "")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `Failed to bind app "her-app" to service instance "redis/her-redis": Post .*http://localhost:1234/resources/her-redis/bind-app.*:.*connection refused.*`)
}

func (s *S) TestBindAppShouldSendAPOSTToTheResourceURL(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	s.mockService.Pool.OnFindByName = func(name string) (*provTypes.Pool, error) {
		return &provTypes.Pool{Name: "her-pool", Provisioner: "kubernetes"}, nil
	}
	s.mockService.Cluster.OnFindByPool = func(provisioner, pool string) (*provTypes.Cluster, error) {
		return &provTypes.Cluster{
			Name:        "her-cluster",
			Provisioner: "kubernetes",
			Addresses:   []string{"https://kubernetes.example.com", "https://backup.kubernetes.example.com"},
		}, nil
	}
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeAppWithPool("her-app", "python", "her-pool", 1)
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	_, err := client.BindApp(context.TODO(), &instance, a, nil, evt, "")
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/resources/"+instance.Name+"/bind-app")
	c.Assert(h.method, check.Equals, http.MethodPost)
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	expected := map[string][]string{
		"app-name":                {"her-app"},
		"app-host":                {"her-app.fakerouter.com"},
		"app-hosts":               {"her-app.fakerouter.com"},
		"app-pool-name":           {"her-pool"},
		"app-pool-provisioner":    {"kubernetes"},
		"app-cluster-name":        {"her-cluster"},
		"app-cluster-provisioner": {"kubernetes"},
		"app-cluster-addresses":   {"https://kubernetes.example.com", "https://backup.kubernetes.example.com"},
		"user":                    {"my@user"},
		"eventid":                 {evt.UniqueID.Hex()},
	}
	c.Assert(map[string][]string(v), check.DeepEquals, expected)
}

func (s *S) TestBindAppWithParams(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	s.mockService.Pool.OnFindByName = func(name string) (*provTypes.Pool, error) {
		return &provTypes.Pool{Name: "her-pool", Provisioner: "kubernetes"}, nil
	}
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeAppWithPool("her-app", "python", "her-pool", 1)
	s.mockService.App.OnGetInternalBindableAddresses = func(app *appTypes.App) ([]string, error) {
		return []string{
			"tcp://aclfromhell-web.tsuru.svc.cluster.local:8888",
			"tcp://aclfromhell-web-v1.tsuru.svc.cluster.local:8888",
		}, nil
	}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	bindParams := map[string]interface{}{
		"p1": "v1",
		"p2": map[string]interface{}{
			"complex1": "complexvalue1",
		},
	}
	_, err := client.BindApp(context.TODO(), &instance, a, bindParams, evt, "")
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/resources/"+instance.Name+"/bind-app")
	c.Assert(h.method, check.Equals, http.MethodPost)
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	expected := map[string][]string{
		"app-name":               {"her-app"},
		"app-host":               {"her-app.fakerouter.com"},
		"app-hosts":              {"her-app.fakerouter.com"},
		"app-internal-hosts":     {"tcp://aclfromhell-web.tsuru.svc.cluster.local:8888", "tcp://aclfromhell-web-v1.tsuru.svc.cluster.local:8888"},
		"app-pool-name":          {"her-pool"},
		"app-pool-provisioner":   {"kubernetes"},
		"user":                   {"my@user"},
		"eventid":                {evt.UniqueID.Hex()},
		"parameters.p1":          {"v1"},
		"parameters.p2.complex1": {"complexvalue1"},
	}
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
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	env, err := client.BindApp(context.TODO(), &instance, a, nil, evt, "")
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
	originalPoolService := servicemanager.Pool
	servicemanager.Pool = &provTypes.MockPoolService{
		OnFindByName: func(name string) (*provTypes.Pool, error) {
			return &provTypes.Pool{Name: "test-default", Provisioner: "docker"}, nil
		},
	}
	defer func() { servicemanager.Pool = originalPoolService }()
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	env, err := client.BindApp(context.TODO(), &instance, a, nil, evt, "")
	c.Assert(err, check.IsNil)
	c.Assert(env, check.DeepEquals, expected)
}

func (s *S) TestBindAppShouldReturnErrorIfTheRequestFail(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	_, err := client.BindApp(context.TODO(), &instance, a, nil, evt, "")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `^Failed to bind the instance "redis/her-redis" to the app "her-app": invalid response: Server failed to do its job. \(code: 500\)$`)
}

func (s *S) TestBindAppInstanceNotReady(c *check.C) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	_, err := client.BindApp(context.TODO(), &instance, a, nil, evt, "")
	c.Assert(err, check.Equals, ErrInstanceNotReady)
}

func (s *S) TestBindAppInstanceNotFound(c *check.C) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis"}
	a := provisiontest.NewFakeApp("her-app", "python", 1)
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	_, err := client.BindApp(context.TODO(), &instance, a, nil, evt, "")
	c.Assert(err, check.Equals, ErrInstanceNotFoundInAPI)
}

func (s *S) TestBindJobEndpointDown(c *check.C) {
	instance := ServiceInstance{Name: "job-redis", ServiceName: "redis"}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	client := &endpointClient{endpoint: "http://localhost:1234", username: "user", password: "abcde"}
	evt := createEvt(c)
	_, err := client.BindJob(context.TODO(), &instance, job, evt, "")

	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `Failed to bind job "test-job" to service instance "redis/job-redis": Put .*http://localhost:1234/resources/job-redis/binds/jobs/test-job.*:.*connection refused.*`)
}

func (s *S) TestBindJobShouldSendAPOSTToTheResourceURL(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	s.mockService.Pool.OnFindByName = func(name string) (*provTypes.Pool, error) {
		return &provTypes.Pool{Name: "test-pool", Provisioner: "kubernetes"}, nil
	}
	s.mockService.Cluster.OnFindByPool = func(provisioner, pool string) (*provTypes.Cluster, error) {
		return &provTypes.Cluster{
			Name:        "test-cluster",
			Provisioner: "kubernetes",
			Addresses:   []string{"https://kubernetes.example.com", "https://backup.kubernetes.example.com"},
		}, nil
	}
	instance := ServiceInstance{Name: "job-redis", ServiceName: "redis"}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	_, err := client.BindJob(context.TODO(), &instance, job, evt, "")
	h.Lock()
	defer h.Unlock()

	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/resources/"+instance.Name+"/binds/jobs/"+job.Name)
	c.Assert(h.method, check.Equals, http.MethodPut)
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	expected := map[string][]string{
		"job-name":         {"test-job"},
		"job-pool-name":    {"test-pool"},
		"job-cluster-name": {"test-cluster"},
		"user":             {"my@user"},
		"eventid":          {evt.UniqueID.Hex()},
	}
	c.Assert(map[string][]string(v), check.DeepEquals, expected)
}

func (s *S) TestBindJobShouldReturnMapWithTheEnvironmentVariable(c *check.C) {
	expected := map[string]string{
		"MYSQL_DATABASE_NAME": "CHICO",
		"MYSQL_HOST":          "localhost",
		"MYSQL_PORT":          "3306",
	}
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "job-redis", ServiceName: "redis"}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	originalPoolService := servicemanager.Pool
	servicemanager.Pool = &provTypes.MockPoolService{
		OnFindByName: func(name string) (*provTypes.Pool, error) {
			return &provTypes.Pool{Name: "test-default", Provisioner: "docker"}, nil
		},
	}
	defer func() { servicemanager.Pool = originalPoolService }()
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	env, err := client.BindJob(context.TODO(), &instance, job, evt, "")

	c.Assert(err, check.IsNil)
	c.Assert(env, check.DeepEquals, expected)
}

func (s *S) TestBindJobShouldReturnErrorIfTheRequestFail(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "test-redis", ServiceName: "redis"}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	_, err := client.BindJob(context.TODO(), &instance, job, evt, "")

	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `^Failed to bind the instance "redis/test-redis" to the job "test-job": invalid response: Server failed to do its job. \(code: 500\)$`)
}

func (s *S) TestBindJobInstanceNotReady(c *check.C) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()
	instance := ServiceInstance{Name: "test-redis", ServiceName: "redis"}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	_, err := client.BindJob(context.TODO(), &instance, job, evt, "")

	c.Assert(err, check.Equals, ErrInstanceNotReady)
}

func (s *S) TestBindJobInstanceNotFound(c *check.C) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()
	instance := ServiceInstance{Name: "test-redis", ServiceName: "redis"}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	_, err := client.BindJob(context.TODO(), &instance, job, evt, "")

	c.Assert(err, check.Equals, ErrInstanceNotFoundInAPI)
}

func (s *S) TestUnbindApp(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven"}
	a := provisiontest.NewFakeApp("arch-enemy", "python", 1)
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.UnbindApp(context.TODO(), &instance, a, evt, "")
	h.Lock()
	defer h.Unlock()
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/resources/heaven-can-wait/bind-app")
	c.Assert(h.method, check.Equals, http.MethodDelete)
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, check.IsNil)
	expected := map[string][]string{
		"app-name":  {"arch-enemy"},
		"app-host":  {"arch-enemy.fakerouter.com"},
		"app-hosts": {"arch-enemy.fakerouter.com"},
		"user":      {"my@user"},
		"eventid":   {evt.UniqueID.Hex()},
	}
	c.Assert(map[string][]string(v), check.DeepEquals, expected)
}

func (s *S) TestUnbindAppRequestFailure(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven"}
	a := provisiontest.NewFakeApp("arch-enemy", "python", 1)
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.UnbindApp(context.TODO(), &instance, a, evt, "")
	c.Assert(err, check.NotNil)
	expected := `Failed to unbind ("/resources/heaven-can-wait/bind-app"): invalid response: Server failed to do its job. (code: 500)`
	c.Assert(err.Error(), check.Equals, expected)
}

func (s *S) TestUnbindAppInstanceNotFound(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven"}
	a := provisiontest.NewFakeApp("arch-enemy", "python", 1)
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.UnbindApp(context.TODO(), &instance, a, evt, "")
	c.Assert(err, check.Equals, ErrInstanceNotFoundInAPI)
}

func (s *S) TestUnbindJob(c *check.C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "test-redis", ServiceName: "redis"}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.UnbindJob(context.TODO(), &instance, job, evt, "")
	h.Lock()
	defer h.Unlock()

	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/resources/test-redis/binds/jobs/test-job")
	c.Assert(h.method, check.Equals, http.MethodDelete)
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.request.Header.Get("Authorization"))
	c.Assert(string(h.body), check.Equals, "")
}

func (s *S) TestUnbindJobRequestFailure(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "test-redis", ServiceName: "redis"}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.UnbindJob(context.TODO(), &instance, job, evt, "")

	c.Assert(err, check.NotNil)
	expected := `Failed to unbind ("/resources/test-redis/binds/jobs/test-job"): invalid response: Server failed to do its job. (code: 500)`
	c.Assert(err.Error(), check.Equals, expected)
}

func (s *S) TestUnbindJobInstanceNotFound(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	instance := ServiceInstance{Name: "test-redis", ServiceName: "redis"}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	evt := createEvt(c)
	err := client.UnbindJob(context.TODO(), &instance, job, evt, "")

	c.Assert(err, check.Equals, ErrInstanceNotFoundInAPI)
}

func (s *S) TestBuildErrorMessageWithNilResponse(c *check.C) {
	cli := endpointClient{}
	err := errors.New("epic fail")
	c.Assert(cli.buildErrorMessage(err, nil), check.ErrorMatches, "epic fail")
}

func (s *S) TestBuildErrorMessageWithNilErrorAndNilResponse(c *check.C) {
	cli := endpointClient{}
	c.Assert(cli.buildErrorMessage(nil, nil), check.IsNil)
}

func (s *S) TestBuildErrorMessageWithNonNilResponseAndNilError(c *check.C) {
	cli := endpointClient{}
	body := strings.NewReader("something went wrong")
	resp := &http.Response{Body: io.NopCloser(body)}
	c.Assert(cli.buildErrorMessage(nil, resp), check.ErrorMatches, `invalid response: something went wrong \(code: 0\)`)
}

func (s *S) TestBuildErrorMessageWithNonNilResponseAndNonNilError(c *check.C) {
	cli := endpointClient{}
	err := errors.New("epic fail")
	body := strings.NewReader("something went wrong")
	resp := &http.Response{Body: io.NopCloser(body)}
	c.Assert(cli.buildErrorMessage(err, resp), check.ErrorMatches, "epic fail")
}

func (s *S) TestStatus(c *check.C) {
	tests := []struct {
		Input    int
		Expected string
	}{
		{http.StatusOK, "working"},
		{http.StatusNoContent, "up"},
		{http.StatusAccepted, "pending"},
		{http.StatusNotFound, "not implemented for this service"},
		{http.StatusInternalServerError, "down"},
	}
	var request int
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(tests[request].Input)
		w.Write([]byte("working"))
		request++
	})
	ts := httptest.NewServer(h)
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	for _, t := range tests {
		state, err := client.Status(context.TODO(), &instance, "")
		c.Check(err, check.IsNil)
		c.Check(state, check.Equals, t.Expected)
	}
}

func (s *S) TestInfo(c *check.C) {
	h := infoHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	result, err := client.Info(context.TODO(), &instance, "")
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
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	result, err := client.Info(context.TODO(), &instance, "")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.IsNil)
}

func (s *S) TestPlans(c *check.C) {
	h := plansHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	result, err := client.Plans(context.TODO(), "", "")
	c.Assert(err, check.IsNil)
	expected := []Plan{
		{Name: "ignite", Description: "some value"},
		{Name: "small", Description: "not space left for you"},
	}
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(h.r.URL.Path, check.Equals, "/resources/plans")
	c.Assert("Basic dXNlcjphYmNkZQ==", check.Equals, h.r.Header.Get("Authorization"))
}

func (s *S) TestEndpointProxy(c *check.C) {
	handlerTest := func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.URL.Query().Get("callback"), check.Equals, "")
		c.Assert(r.URL.Query().Get("foo"), check.Equals, "bar")
		c.Assert(r.URL.Query()["names"], check.DeepEquals, []string{"joe", "doe"})
		w.WriteHeader(http.StatusNoContent)
	}
	ts := httptest.NewServer(http.HandlerFunc(handlerTest))
	defer ts.Close()
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	request, err := http.NewRequest(http.MethodGet, "/?callback=/backup&foo=bar&names=joe&names=doe", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	evt := createEvt(c)
	err = client.Proxy(context.TODO(), &ProxyOpts{
		Path:    "/backup",
		Event:   evt,
		Writer:  recorder,
		Request: request,
	})
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestProxyWithBodyAndHeaders(c *check.C) {
	var proxiedRequest *http.Request
	var readBodyStr []byte
	handlerTest := func(w http.ResponseWriter, r *http.Request) {
		readBodyStr, _ = io.ReadAll(r.Body)
		proxiedRequest = r
		w.WriteHeader(http.StatusNoContent)
	}
	ts := httptest.NewServer(http.HandlerFunc(handlerTest))
	defer ts.Close()
	client := &endpointClient{endpoint: ts.URL, username: "user", password: "abcde"}
	b := bytes.NewBufferString(`{"bla": "bla"}`)
	request, err := http.NewRequest(http.MethodPost, "http://somewhere.com/", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "text/new-crobuzon")
	recorder := httptest.NewRecorder()
	evt := createEvt(c)
	err = client.Proxy(context.TODO(), &ProxyOpts{
		Path:    "/backup",
		Event:   evt,
		Writer:  recorder,
		Request: request,
	})
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
	c.Assert(proxiedRequest.Header.Get("Content-Type"), check.Equals, "text/new-crobuzon")
	c.Assert(proxiedRequest.Method, check.Equals, http.MethodPost)
	c.Assert(proxiedRequest.URL.String(), check.Equals, "/backup")
	tsURL, err := url.Parse(ts.URL)
	c.Assert(err, check.IsNil)
	c.Assert(proxiedRequest.Host, check.Equals, tsURL.Host)
	c.Assert(string(readBodyStr), check.Equals, `{"bla": "bla"}`)
}
