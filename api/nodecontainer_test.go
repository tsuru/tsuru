// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"

	"github.com/ajg/form"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestNodeContainerList(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c2",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"B=1"},
		},
	})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.2/nodecontainers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var configEntries []nodecontainer.NodeContainerConfigGroup
	json.Unmarshal(recorder.Body.Bytes(), &configEntries)
	sort.Sort(nodecontainer.NodeContainerConfigGroupSlice(configEntries))
	c.Assert(configEntries, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"":   {Name: "c1", Config: docker.Config{Image: "img1", Env: []string{"A=1"}}},
			"p1": {Name: "c1", Config: docker.Config{Env: []string{"A=2"}}},
		}},
		{Name: "c2", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c2", Config: docker.Config{Image: "img1", Env: []string{"B=1"}}},
		}},
	})
}

func (s *S) TestNodeContainerListLimited(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p3", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=3"},
		},
	})
	c.Assert(err, check.IsNil)
	t := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermNodecontainerRead,
		Context: permission.Context(permTypes.CtxPool, "p3"),
	})
	request, err := http.NewRequest("GET", "/1.2/nodecontainers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var configEntries []nodecontainer.NodeContainerConfigGroup
	json.Unmarshal(recorder.Body.Bytes(), &configEntries)
	sort.Sort(nodecontainer.NodeContainerConfigGroupSlice(configEntries))
	c.Assert(configEntries, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"":   {Name: "c1", Config: docker.Config{Image: "img1", Env: []string{"A=1"}}},
			"p3": {Name: "c1", Config: docker.Config{Env: []string{"A=3"}}},
		}},
	})
}

func (s *S) TestNodeContainerInfoNotFound(c *check.C) {
	request, err := http.NewRequest("GET", "/1.2/nodecontainers/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestNodeContainerInfo(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c2",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"B=1"},
		},
	})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.2/nodecontainers/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var configEntries map[string]nodecontainer.NodeContainerConfig
	json.Unmarshal(recorder.Body.Bytes(), &configEntries)
	c.Assert(configEntries, check.DeepEquals, map[string]nodecontainer.NodeContainerConfig{
		"":   {Name: "c1", Config: docker.Config{Image: "img1", Env: []string{"A=1"}}},
		"p1": {Name: "c1", Config: docker.Config{Env: []string{"A=2"}}},
	})
}

func (s *S) TestNodeContainerInfoLimited(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c2",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"B=1"},
		},
	})
	c.Assert(err, check.IsNil)
	t := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermNodecontainerRead,
		Context: permission.Context(permTypes.CtxPool, "p-none"),
	})
	request, err := http.NewRequest("GET", "/1.2/nodecontainers/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var configEntries map[string]nodecontainer.NodeContainerConfig
	json.Unmarshal(recorder.Body.Bytes(), &configEntries)
	c.Assert(configEntries, check.DeepEquals, map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c1", Config: docker.Config{Image: "img1", Env: []string{"A=1"}}},
	})
}

func (s *S) TestNodeContainerCreate(c *check.C) {
	doReq := func(cont nodecontainer.NodeContainerConfig, expected []nodecontainer.NodeContainerConfigGroup, pool ...string) {
		values, err := form.EncodeToValues(cont)
		c.Assert(err, check.IsNil)
		if len(pool) > 0 {
			values.Set("pool", pool[0])
		}
		values.Del("Disabled")
		reader := strings.NewReader(values.Encode())
		request, err := http.NewRequest("POST", "/1.2/nodecontainers", reader)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		server := RunServer(true)
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		request, err = http.NewRequest("GET", "/1.2/nodecontainers", nil)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder = httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		var configEntries []nodecontainer.NodeContainerConfigGroup
		json.Unmarshal(recorder.Body.Bytes(), &configEntries)
		sort.Sort(nodecontainer.NodeContainerConfigGroupSlice(configEntries))
		c.Assert(configEntries, check.DeepEquals, expected)
	}
	doReq(nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}}, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c1", Config: docker.Config{Image: "img1", Healthcheck: &docker.HealthConfig{}}},
		}},
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNodeContainer, Value: "c1"},
		Owner:  s.token.GetUserName(),
		Kind:   "nodecontainer.create",
		StartCustomData: []map[string]interface{}{
			{"name": "Name", "value": "c1"},
			{"name": "Config.Image", "value": "img1"},
		},
	}, eventtest.HasEvent)
	doReq(nodecontainer.NodeContainerConfig{
		Name:       "c2",
		Config:     docker.Config{Env: []string{"A=1"}, Image: "img2"},
		HostConfig: docker.HostConfig{Memory: 256, Privileged: true},
	}, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c1", Config: docker.Config{Image: "img1", Healthcheck: &docker.HealthConfig{}}},
		}},
		{Name: "c2", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c2", Config: docker.Config{Env: []string{"A=1"}, Image: "img2", Healthcheck: &docker.HealthConfig{}}, HostConfig: docker.HostConfig{Memory: 256, Privileged: true}},
		}},
	})
	doReq(nodecontainer.NodeContainerConfig{
		Name:       "c2",
		Config:     docker.Config{Env: []string{"Z=9"}, Image: "img2"},
		HostConfig: docker.HostConfig{Memory: 256},
	}, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c1", Config: docker.Config{Image: "img1", Healthcheck: &docker.HealthConfig{}}},
		}},
		{Name: "c2", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c2", Config: docker.Config{Env: []string{"Z=9"}, Image: "img2", Healthcheck: &docker.HealthConfig{}}, HostConfig: docker.HostConfig{Memory: 256}},
		}},
	})
	doReq(nodecontainer.NodeContainerConfig{
		Name:   "c2",
		Config: docker.Config{Env: []string{"X=1"}},
	}, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c1", Config: docker.Config{Image: "img1", Healthcheck: &docker.HealthConfig{}}},
		}},
		{Name: "c2", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"":   {Name: "c2", Config: docker.Config{Env: []string{"Z=9"}, Image: "img2", Healthcheck: &docker.HealthConfig{}}, HostConfig: docker.HostConfig{Memory: 256}},
			"p1": {Name: "c2", Config: docker.Config{Env: []string{"X=1"}, Healthcheck: &docker.HealthConfig{}}},
		}},
	}, "p1")
}

func (s *S) TestNodeContainerCreateInvalid(c *check.C) {
	reader := strings.NewReader("")
	request, err := http.NewRequest("POST", "/1.2/nodecontainers", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Matches, "node container config name cannot be empty\n")
	values, err := form.EncodeToValues(nodecontainer.NodeContainerConfig{Name: ""})
	c.Assert(err, check.IsNil)
	reader = strings.NewReader(values.Encode())
	request, err = http.NewRequest("POST", "/1.2/nodecontainers", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Matches, "node container config name cannot be empty\n")
	values, err = form.EncodeToValues(nodecontainer.NodeContainerConfig{Name: "x1"})
	c.Assert(err, check.IsNil)
	reader = strings.NewReader(values.Encode())
	request, err = http.NewRequest("POST", "/1.2/nodecontainers", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Matches, "node container config image cannot be empty\n")
}

func (s *S) TestNodeContainerCreateLimited(c *check.C) {
	t := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermNodecontainerCreate,
		Context: permission.Context(permTypes.CtxPool, "p1"),
	})
	values, err := form.EncodeToValues(nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	reader := strings.NewReader(values.Encode())
	request, err := http.NewRequest("POST", "/1.2/nodecontainers", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	values.Set("pool", "p1")
	reader = strings.NewReader(values.Encode())
	request, err = http.NewRequest("POST", "/1.2/nodecontainers", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestNodeContainerUpdate(c *check.C) {
	doReq := func(cont nodecontainer.NodeContainerConfig, expected map[string]nodecontainer.NodeContainerConfig, pool ...string) {
		values, err := form.EncodeToValues(cont)
		c.Assert(err, check.IsNil)
		if len(pool) > 0 {
			values.Set("pool", pool[0])
		}
		values.Del("Disabled")
		reader := strings.NewReader(values.Encode())
		request, err := http.NewRequest("POST", "/1.2/nodecontainers/"+cont.Name, reader)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		server := RunServer(true)
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		request, err = http.NewRequest("GET", "/1.2/nodecontainers/"+cont.Name, nil)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder = httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		var configEntries map[string]nodecontainer.NodeContainerConfig
		json.Unmarshal(recorder.Body.Bytes(), &configEntries)
		if len(pool) > 0 {
			for _, p := range pool {
				sort.Strings(configEntries[p].Config.Env)
				sort.Strings(expected[p].Config.Env)
			}
		}
		sort.Strings(configEntries[""].Config.Env)
		sort.Strings(expected[""].Config.Env)
		c.Assert(configEntries, check.DeepEquals, expected)
	}
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{Name: "c2", Config: docker.Config{Image: "img2"}})
	c.Assert(err, check.IsNil)
	doReq(nodecontainer.NodeContainerConfig{Name: "c1"}, map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c1", Config: docker.Config{Image: "img1"}},
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNodeContainer, Value: "c1"},
		Owner:  s.token.GetUserName(),
		Kind:   "nodecontainer.update",
		StartCustomData: []map[string]interface{}{
			{"name": "Name", "value": "c1"},
		},
	}, eventtest.HasEvent)
	doReq(nodecontainer.NodeContainerConfig{
		Name:       "c2",
		Config:     docker.Config{Env: []string{"A=1"}},
		HostConfig: docker.HostConfig{Memory: 256, Privileged: true},
	}, map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c2", Config: docker.Config{Env: []string{"A=1"}, Image: "img2"}, HostConfig: docker.HostConfig{Memory: 256, Privileged: true}},
	})
	doReq(nodecontainer.NodeContainerConfig{
		Name:       "c2",
		Config:     docker.Config{Env: []string{"Z=9"}},
		HostConfig: docker.HostConfig{Memory: 256},
	}, map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c2", Config: docker.Config{Env: []string{"A=1", "Z=9"}, Image: "img2"}, HostConfig: docker.HostConfig{Memory: 256, Privileged: true}},
	})
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{Name: "c2"})
	c.Assert(err, check.IsNil)
	doReq(nodecontainer.NodeContainerConfig{
		Name:   "c2",
		Config: docker.Config{Env: []string{"X=1"}},
	}, map[string]nodecontainer.NodeContainerConfig{
		"":   {Name: "c2", Config: docker.Config{Env: []string{"A=1", "Z=9"}, Image: "img2"}, HostConfig: docker.HostConfig{Memory: 256, Privileged: true}},
		"p1": {Name: "c2", Config: docker.Config{Env: []string{"X=1"}}},
	}, "p1")
}

func (s *S) TestNodeContainerUpdateInvalid(c *check.C) {
	cont := nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}}
	val, err := form.EncodeToValues(cont)
	c.Assert(err, check.IsNil)
	reader := strings.NewReader(val.Encode())
	request, err := http.NewRequest("POST", "/1.2/nodecontainers/c1", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Matches, "node container not found\n")
}

func (s *S) TestNodeContainerUpdateLimited(c *check.C) {
	err := nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	t := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermNodecontainerUpdate,
		Context: permission.Context(permTypes.CtxPool, "p1"),
	})
	values, err := form.EncodeToValues(nodecontainer.NodeContainerConfig{Name: "c1"})
	c.Assert(err, check.IsNil)
	reader := strings.NewReader(values.Encode())
	request, err := http.NewRequest("POST", "/1.2/nodecontainers/c1", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	values.Set("pool", "p1")
	reader = strings.NewReader(values.Encode())
	request, err = http.NewRequest("POST", "/1.2/nodecontainers/c1", reader)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+t.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestNodeContainerUpdateDisableEnable(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{Name: "c1", Config: docker.Config{Image: "img1"}})
	c.Assert(err, check.IsNil)
	values, err := form.EncodeToValues(nodecontainer.NodeContainerConfig{Name: "c1"})
	c.Assert(err, check.IsNil)
	doReq := func(expected map[string]nodecontainer.NodeContainerConfig) {
		reader := strings.NewReader(values.Encode())
		var request *http.Request
		request, err = http.NewRequest("POST", "/1.2/nodecontainers/c1", reader)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		server := RunServer(true)
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		request, err = http.NewRequest("GET", "/1.2/nodecontainers/c1", nil)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder = httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		var configEntries map[string]nodecontainer.NodeContainerConfig
		json.Unmarshal(recorder.Body.Bytes(), &configEntries)
		c.Assert(configEntries, check.DeepEquals, expected)
	}
	values.Set("Disabled", "true")
	disabled := true
	doReq(map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c1", Disabled: &disabled, Config: docker.Config{Image: "img1"}},
	})
	values.Del("Disabled")
	doReq(map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c1", Disabled: &disabled, Config: docker.Config{Image: "img1"}},
	})
	values.Set("Disabled", "false")
	disabled = false
	doReq(map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c1", Disabled: &disabled, Config: docker.Config{Image: "img1"}},
	})
	values.Del("Disabled")
	doReq(map[string]nodecontainer.NodeContainerConfig{
		"": {Name: "c1", Disabled: &disabled, Config: docker.Config{Image: "img1"}},
	})
}

func (s *S) TestNodeContainerDelete(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Env: []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.2/nodecontainers/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	all, err := nodecontainer.AllNodeContainers()
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"p1": {Name: "c1", Config: docker.Config{Env: []string{"A=2"}}},
		}},
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNodeContainer, Value: "c1"},
		Owner:  s.token.GetUserName(),
		Kind:   "nodecontainer.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "c1"},
		},
	}, eventtest.HasEvent)
	s.provisioner.UpgradeNodeContainer("c1", "p1", ioutil.Discard)
	request, err = http.NewRequest("DELETE", "/1.2/nodecontainers/c1?pool=p1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	all, err = nodecontainer.AllNodeContainers()
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{})
	c.Assert(s.provisioner.HasNodeContainer("c1", "p1"), check.Equals, true)
}

func (s *S) TestNodeContainerDeleteKillsRunningContainers(c *check.C) {
	err := nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img",
			Env:   []string{"A=2"},
		},
	})
	c.Assert(err, check.IsNil)
	s.provisioner.UpgradeNodeContainer("c1", "p1", ioutil.Discard)
	c.Assert(s.provisioner.HasNodeContainer("c1", "p1"), check.Equals, true)
	request, err := http.NewRequest("DELETE", "/1.2/nodecontainers/c1?pool=p1&kill=1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(s.provisioner.HasNodeContainer("c1", "p1"), check.Equals, false)
}

func (s *S) TestNodeContainerDeleteNotFound(c *check.C) {
	err := nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.2/nodecontainers/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "node container \"c1\" not found for pool \"\"\n")
	request, err = http.NewRequest("DELETE", "/1.2/nodecontainers/c1?pool=p1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestNodeContainerUpgrade(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name:        "c1",
		PinnedImage: "tsuru/c1@sha256:abcef384829283eff",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/docker/nodecontainers/c1/upgrade", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	all, err := nodecontainer.AllNodeContainers()
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"": {Name: "c1", Config: docker.Config{Env: []string{"A=1"}, Image: "img1"}},
		}},
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNodeContainer, Value: "c1"},
		Owner:  s.token.GetUserName(),
		Kind:   "nodecontainer.update.upgrade",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "c1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestNodeContainerUpgradeLimited(c *check.C) {
	err := nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name:        "c1",
		PinnedImage: "tsuru/c1@sha256:abcef384829283eff",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	form := url.Values{}
	form.Set("pool", "p1")
	request, err := http.NewRequest("POST", "/docker/nodecontainers/c1/upgrade", strings.NewReader(form.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	all, err := nodecontainer.AllNodeContainers()
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, []nodecontainer.NodeContainerConfigGroup{
		{Name: "c1", ConfigPools: map[string]nodecontainer.NodeContainerConfig{
			"p1": {Name: "c1", Config: docker.Config{Env: []string{"A=1"}, Image: "img1"}},
		}},
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNodeContainer, Value: "c1"},
		Owner:  s.token.GetUserName(),
		Kind:   "nodecontainer.update.upgrade",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "c1"},
			{"name": "pool", "value": "p1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestNodeContainerUpgradeNotFound(c *check.C) {
	err := nodecontainer.AddNewContainer("otherpool", &nodecontainer.NodeContainerConfig{
		Name:        "c2",
		PinnedImage: "tsuru/c1@sha256:abcef384829283eff",
		Config: docker.Config{
			Image: "img1",
			Env:   []string{"A=1"},
		},
	})
	c.Assert(err, check.IsNil)
	tt := []struct {
		name string
		pool string
	}{
		{"c1", ""},
		{"c2", "theonepool"},
	}
	server := RunServer(true)
	for _, t := range tt {
		recorder := httptest.NewRecorder()
		form := url.Values{}
		if t.pool != "" {
			form.Set("pool", t.pool)
		}
		request, errReq := http.NewRequest("POST", "/docker/nodecontainers/"+t.name+"/upgrade", strings.NewReader(form.Encode()))
		c.Assert(errReq, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	}
}
