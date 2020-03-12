// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/iaas"
	iaasTesting "github.com/tsuru/tsuru/iaas/testing"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	apiTypes "github.com/tsuru/tsuru/types/api"
	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestValidateNodeAddress(c *check.C) {
	err := validateNodeAddress("/invalid")
	c.Assert(err, check.ErrorMatches, "Invalid address url: host cannot be empty")
	err = validateNodeAddress("xxx://abc/invalid")
	c.Assert(err, check.ErrorMatches, `Invalid address url: scheme must be http\[s\]`)
	err = validateNodeAddress("")
	c.Assert(err, check.ErrorMatches, "address=url parameter is required")
}

func (s *S) TestAddNodeHandler(c *check.C) {
	opts := pool.AddPoolOptions{Name: "pool1"}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer pool.RemovePool("pool1")
	serverAddr := "http://mysrv1"
	params := provision.AddNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"address": serverAddr,
			"pool":    "pool1",
			"m1":      "v1",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("POST", "/1.2/node", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	c.Assert(rec.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	nodes, err := s.provisioner.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, serverAddr)
	c.Assert(nodes[0].Pool(), check.Equals, "pool1")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"m1": "v1",
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNode, Value: serverAddr},
		Owner:  s.token.GetUserName(),
		Kind:   "node.create",
		StartCustomData: []map[string]interface{}{
			{"name": "Metadata.address", "value": serverAddr},
			{"name": "Metadata.pool", "value": "pool1"},
			{"name": "Register", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddNodeHandlerExistingInDifferentProvisioner(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake-other"
	provision.Register("fake-other", func() (provision.Provisioner, error) {
		return p1, nil
	})
	defer provision.Unregister("fake-other")
	serverAddr := "http://mysrv1"
	err := p1.AddNode(provision.AddNodeOptions{
		Address: serverAddr,
	})
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "pool1"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer pool.RemovePool("pool1")
	params := provision.AddNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"address": serverAddr,
			"pool":    "pool1",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("POST", "/1.2/node", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result["Error"], check.Equals, "node with address \"http://mysrv1\" already exists in provisioner \"fake-other\"")
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
}

func (s *S) TestAddNodeHandlerExisting(c *check.C) {
	opts := pool.AddPoolOptions{Name: "pool1"}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer pool.RemovePool("pool1")
	serverAddr := "http://mysrv1"
	params := provision.AddNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"address": serverAddr,
			"pool":    "pool1",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("POST", "/1.2/node", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	req, err = http.NewRequest("POST", "/1.2/node", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec = httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result["Error"], check.Equals, "fake node already exists")
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
}

func (s *S) TestAddNodeHandlerCreatingAnIaasMachine(c *check.C) {
	iaas.RegisterIaasProvider("test-iaas", newTestIaaS)
	opts := pool.AddPoolOptions{Name: "pool1"}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer pool.RemovePool("pool1")
	params := provision.AddNodeOptions{
		Register: false,
		Metadata: map[string]string{
			"id":   "test1",
			"pool": "pool1",
			"iaas": "test-iaas",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	req, err := http.NewRequest("POST", "/node", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	c.Assert(rec.Body.String(), check.Equals, "")
	nodes, err := s.provisioner.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "http://test1.somewhere.com:2375")
	c.Assert(nodes[0].Pool(), check.Equals, "pool1")
	c.Assert(nodes[0].IaaSID(), check.Equals, "test1-pool1")
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"id":      "test1",
		"iaas":    "test-iaas",
		"iaas-id": "test1-pool1",
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNode, Value: "http://test1.somewhere.com:2375"},
		Owner:  s.token.GetUserName(),
		Kind:   "node.create",
		StartCustomData: []map[string]interface{}{
			{"name": "Metadata.id", "value": "test1"},
			{"name": "Metadata.pool", "value": "pool1"},
			{"name": "Register", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddNodeHandlerWithoutAddress(c *check.C) {
	opts := pool.AddPoolOptions{Name: "pool1"}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	params := provision.AddNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"pool": "pool1",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	req, err := http.NewRequest("POST", "/node", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result["Error"], check.Equals, "address=url parameter is required")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNode},
		Owner:  s.token.GetUserName(),
		Kind:   "node.create",
		StartCustomData: []map[string]interface{}{
			{"name": "Metadata.pool", "value": "pool1"},
			{"name": "Register", "value": "true"},
		},
		ErrorMatches: `address=url parameter is required`,
	}, eventtest.HasEvent)
}

func (s *S) TestAddNodeHandlerWithInvalidURLAddress(c *check.C) {
	opts := pool.AddPoolOptions{Name: "pool1"}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	params := provision.AddNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"address": "/invalid",
			"pool":    "pool1",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	req, err := http.NewRequest("POST", "/node", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	var result map[string]string
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result["Error"], check.Equals, "Invalid address url: host cannot be empty")
	params = provision.AddNodeOptions{
		Register: true,
		Metadata: map[string]string{
			"address": "xxx://abc/invalid",
			"pool":    "pool1",
		},
	}
	v, err = form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b = strings.NewReader(v.Encode())
	req, err = http.NewRequest("POST", "/node", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.token.GetValue())
	rec = httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	err = json.NewDecoder(rec.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result["Error"], check.Equals, "Invalid address url: scheme must be http[s]")
}

func (s *S) TestAddNodeHandlerNoPool(c *check.C) {
	b := bytes.NewBufferString(`{"address": "http://192.168.50.4:2375"}`)
	req, err := http.NewRequest("POST", "/node?register=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req.Header.Set("Authorization", s.token.GetValue())
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(rec.Body.String(), check.Equals, "pool is required\n")
}

func (s *S) TestRemoveNodeHandlerNotFound(c *check.C) {
	req, err := http.NewRequest("DELETE", "/node/host.com:2375", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRemoveNodeHandler(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "host.com:2375",
	})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("DELETE", "/node/host.com:2375", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Body.String(), check.Equals, "rebalancing...remove done!")
	nodes, err := s.provisioner.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNode, Value: "host.com:2375"},
		Owner:  s.token.GetUserName(),
		Kind:   "node.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":address", "value": "host.com:2375"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveNodeHandlerNoRebalance(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "host.com:2375",
	})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("DELETE", "/node/host.com:2375?no-rebalance=true", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Body.String(), check.Equals, "remove done!")
	nodes, err := s.provisioner.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNode, Value: "host.com:2375"},
		Owner:  s.token.GetUserName(),
		Kind:   "node.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":address", "value": "host.com:2375"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveNodeHandlerWithoutRemoveIaaS(c *check.C) {
	iaas.RegisterIaasProvider("some-iaas", newTestIaaS)
	machine, err := iaas.CreateMachineForIaaS("some-iaas", map[string]string{"id": "m1"})
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address: fmt.Sprintf("http://%s:2375", machine.Address),
	})
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/node/http://%s:2375?remove-iaas=false", machine.Address)
	req, err := http.NewRequest("DELETE", u, nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Body.String(), check.Equals, "rebalancing...remove done!")
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	nodes, err := s.provisioner.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
	dbM, err := iaas.FindMachineById(machine.Id)
	c.Assert(err, check.IsNil)
	c.Assert(dbM.Id, check.Equals, machine.Id)
}

func (s *S) TestRemoveNodeHandlerWithRemoveIaaS(c *check.C) {
	iaas.RegisterIaasProvider("some-iaas", newTestIaaS)
	machine, err := iaas.CreateMachineForIaaS("some-iaas", map[string]string{"id": "m1"})
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address: fmt.Sprintf("http://%s:2375", machine.Address),
	})
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/node/http://%s:2375?remove-iaas=true", machine.Address)
	req, err := http.NewRequest("DELETE", u, nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Body.String(), check.Equals, "rebalancing...remove done!")
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	nodes, err := s.provisioner.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
	_, err = iaas.FindMachineById(machine.Id)
	c.Assert(err, check.Equals, iaas.ErrMachineNotFound)
}

func (s *S) TestListNodeHandlerNoContent(c *check.C) {
	req, err := http.NewRequest("GET", "/node", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req.Header.Set("Authorization", s.token.GetValue())
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestListNodeHandler(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "host1.com:2375",
		Pool:    "pool1",
	})
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address:  "host2.com:2375",
		Pool:     "pool2",
		Metadata: map[string]string{"foo": "bar"},
	})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("GET", "/node", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req.Header.Set("Authorization", s.token.GetValue())
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Header().Get("Content-Type"), check.Equals, "application/json")
	var result apiTypes.ListNodeResponse
	err = json.Unmarshal(rec.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	sort.Slice(result.Nodes, func(i, j int) bool {
		return result.Nodes[i].Address+result.Nodes[i].Pool < result.Nodes[j].Address+result.Nodes[j].Pool
	})
	c.Assert(result.Nodes, check.DeepEquals, []provision.NodeSpec{
		{Address: "host1.com:2375", Provisioner: "fake", Pool: "pool1", Status: "enabled", Metadata: map[string]string{}},
		{Address: "host2.com:2375", Provisioner: "fake", Pool: "pool2", Status: "enabled", Metadata: map[string]string{"foo": "bar"}},
	})
}

func (s *S) TestListNodeHandlerWithFilter(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "host1.com:2375",
		Pool:    "pool1",
	})
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address:  "host2.com:2375",
		Pool:     "pool2",
		Metadata: map[string]string{"foo": "bar"},
	})
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address:  "host3.com:2375",
		Pool:     "pool3",
		Metadata: map[string]string{"foo": "bar", "key": "value"},
	})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("GET", "/node?metadata.foo=bar", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req.Header.Set("Authorization", s.token.GetValue())
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Header().Get("Content-Type"), check.Equals, "application/json")
	var result apiTypes.ListNodeResponse
	err = json.Unmarshal(rec.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	sort.Slice(result.Nodes, func(i, j int) bool {
		return result.Nodes[i].Address < result.Nodes[j].Address
	})
	c.Assert(result.Nodes, check.DeepEquals, []provision.NodeSpec{
		{Address: "host2.com:2375", Provisioner: "fake", Pool: "pool2", Status: "enabled", Metadata: map[string]string{"foo": "bar"}},
		{Address: "host3.com:2375", Provisioner: "fake", Pool: "pool3", Status: "enabled", Metadata: map[string]string{"foo": "bar", "key": "value"}},
	})
	req, err = http.NewRequest("GET", "/node?metadata.foo=bar&metadata.key=value", nil)
	c.Assert(err, check.IsNil)
	rec = httptest.NewRecorder()
	req.Header.Set("Authorization", s.token.GetValue())
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Header().Get("Content-Type"), check.Equals, "application/json")
	err = json.Unmarshal(rec.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	sort.Slice(result.Nodes, func(i, j int) bool {
		return result.Nodes[i].Address < result.Nodes[j].Address
	})
	c.Assert(result.Nodes, check.DeepEquals, []provision.NodeSpec{
		{Address: "host3.com:2375", Provisioner: "fake", Pool: "pool3", Status: "enabled", Metadata: map[string]string{"foo": "bar", "key": "value"}},
	})
}

func (s *S) TestListUnitsByHostNotFound(c *check.C) {
	req, err := http.NewRequest("GET", "/node/http://notfound.com:4243/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestListUnitsByHostNoContent(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "http://node1.company:4243",
	})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("GET", "/node/http://node1.company:4243/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestListUnitsByHostHandler(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "http://node1.company:4243",
	})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(2, "", "", nil)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("GET", "/node/http://node1.company:4243/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, check.IsNil)
	var result []provision.Unit
	var resultMap []map[string]interface{}
	err = json.Unmarshal(body, &result)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(body, &resultMap)
	c.Assert(err, check.IsNil)
	c.Assert(result[0].ID, check.Equals, "myapp-0")
	c.Assert(result[0].Type, check.Equals, "zend")
	c.Assert(result[1].ID, check.Equals, "myapp-1")
	c.Assert(result[1].Type, check.Equals, "zend")
	c.Assert(resultMap[0]["HostAddr"], check.Equals, "node1.company")
	c.Assert(resultMap[1]["HostAddr"], check.Equals, "node1.company")
	c.Assert(resultMap[0]["HostPort"], check.Equals, "1")
	c.Assert(resultMap[1]["HostPort"], check.Equals, "2")
	c.Assert(resultMap[0]["IP"], check.Equals, "node1.company")
	c.Assert(resultMap[1]["IP"], check.Equals, "node1.company")
}

func (s *S) TestListUnitsByAppNotFound(c *check.C) {
	req, err := http.NewRequest("GET", "/node/apps/notfound/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestListUnitsByAppNoContent(c *check.C) {
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("GET", "/node/apps/myapp/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestListUnitsByAppHandler(c *check.C) {
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(2, "", "", nil)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("GET", "/node/apps/myapp/containers", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, check.IsNil)
	var result []provision.Unit
	var resultMap []map[string]interface{}
	err = json.Unmarshal(body, &result)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(body, &resultMap)
	c.Assert(err, check.IsNil)
	c.Assert(result[0].ID, check.Equals, "myapp-0")
	c.Assert(result[0].Type, check.Equals, "zend")
	c.Assert(result[1].ID, check.Equals, "myapp-1")
	c.Assert(result[1].Type, check.Equals, "zend")
	c.Assert(resultMap[0]["HostAddr"], check.Equals, "10.10.10.1")
	c.Assert(resultMap[1]["HostAddr"], check.Equals, "10.10.10.2")
	c.Assert(resultMap[0]["HostPort"], check.Equals, "1")
	c.Assert(resultMap[1]["HostPort"], check.Equals, "2")
	c.Assert(resultMap[0]["IP"], check.Equals, "10.10.10.1")
	c.Assert(resultMap[1]["IP"], check.Equals, "10.10.10.2")
}

func (s *S) TestListUnitsByAppHandlerNotAdminUser(c *check.C) {
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(2, "", "", nil)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("GET", "/node/apps/myapp/containers", nil)
	c.Assert(err, check.IsNil)
	t := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	req.Header.Set("Authorization", "bearer "+t.GetValue())
	rec := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(rec.Body)
	c.Assert(err, check.IsNil)
	var result []provision.Unit
	var resultMap []map[string]interface{}
	err = json.Unmarshal(body, &result)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(body, &resultMap)
	c.Assert(err, check.IsNil)
	c.Assert(result[0].ID, check.Equals, "myapp-0")
	c.Assert(result[0].Type, check.Equals, "zend")
	c.Assert(result[1].ID, check.Equals, "myapp-1")
	c.Assert(result[1].Type, check.Equals, "zend")
	c.Assert(resultMap[0]["HostAddr"], check.Equals, "10.10.10.1")
	c.Assert(resultMap[1]["HostAddr"], check.Equals, "10.10.10.2")
	c.Assert(resultMap[0]["HostPort"], check.Equals, "1")
	c.Assert(resultMap[1]["HostPort"], check.Equals, "2")
	c.Assert(resultMap[0]["IP"], check.Equals, "10.10.10.1")
	c.Assert(resultMap[1]["IP"], check.Equals, "10.10.10.2")
}

func (s *S) TestUpdateNodeHandler(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "localhost:1999",
	})
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "pool1"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	params := provision.UpdateNodeOptions{
		Address: "localhost:1999",
		Metadata: map[string]string{
			"m1": "",
			"m2": "v9",
			"m3": "v8",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/1.2/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	nodes, err := s.provisioner.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"m1": "",
		"m2": "v9",
		"m3": "v8",
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeNode, Value: "localhost:1999"},
		Owner:  s.token.GetUserName(),
		Kind:   "node.update",
		StartCustomData: []map[string]interface{}{
			{"name": "Metadata.m1", "value": ""},
			{"name": "Metadata.m2", "value": "v9"},
			{"name": "Metadata.m3", "value": "v8"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateNodeHandlerNoAddress(c *check.C) {
	params := provision.UpdateNodeOptions{}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestUpdateNodeHandlerNodeDoesNotExist(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "localhost1:1999",
		Metadata: map[string]string{
			"m1": "v1",
			"m2": "v2",
		},
	})
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "pool1"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer pool.RemovePool("pool1")
	params := provision.UpdateNodeOptions{
		Address: "localhost2:1999",
		Metadata: map[string]string{
			"m1": "",
			"m2": "v9",
			"m3": "v8",
		},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, provision.ErrNodeNotFound.Error()+"\n")
	nodes, err := s.provisioner.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Metadata(), check.DeepEquals, map[string]string{
		"m1": "v1",
		"m2": "v2",
	})
}

func (s *S) TestUpdateNodeDisableNodeHandler(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "localhost:1999",
	})
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "pool1"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer pool.RemovePool("pool1")
	params := provision.UpdateNodeOptions{
		Address: "localhost:1999",
		Disable: true,
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	nodes, err := s.provisioner.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Status(), check.Equals, "disabled")
}

func (s *S) TestUpdateNodeEnableNodeHandler(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "localhost:1999",
	})
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "pool1"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer pool.RemovePool("pool1")
	params := provision.UpdateNodeOptions{
		Address: "localhost:1999",
		Enable:  true,
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	nodes, err := s.provisioner.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Status(), check.Equals, "enabled")
}

func (s *S) TestUpdateNodeEnableAndDisableCantBeDone(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "localhost:1999",
	})
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "pool1"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer pool.RemovePool("pool1")
	params := provision.UpdateNodeOptions{
		Address: "localhost:1999",
		Enable:  true,
		Disable: true,
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/node", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "A node can't be enabled and disabled simultaneously.\n")
}

func (s *S) TestNodeHealingUpdateRead(c *check.C) {
	doRequest := func(str string) map[string]healer.NodeHealerConfig {
		body := bytes.NewBufferString(str)
		request, err := http.NewRequest("POST", "/docker/healing/node", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		server := RunServer(true)
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		request, err = http.NewRequest("GET", "/docker/healing/node", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder = httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		var configMap map[string]healer.NodeHealerConfig
		json.Unmarshal(recorder.Body.Bytes(), &configMap)
		return configMap
	}
	tests := []struct {
		A string
		B map[string]healer.NodeHealerConfig
	}{
		{"", map[string]healer.NodeHealerConfig{
			"": {},
		}},
		{"Enabled=true&MaxTimeSinceSuccess=60", map[string]healer.NodeHealerConfig{
			"": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60)},
		}},
		{"MaxUnresponsiveTime=10", map[string]healer.NodeHealerConfig{
			"": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(10)},
		}},
		{"Enabled=false", map[string]healer.NodeHealerConfig{
			"": {Enabled: boolPtr(false), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(10)},
		}},
		{"MaxUnresponsiveTime=20", map[string]healer.NodeHealerConfig{
			"": {Enabled: boolPtr(false), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
		}},
		{"Enabled=true", map[string]healer.NodeHealerConfig{
			"": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
		}},
		{"pool=p1&Enabled=false", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(false), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(20), MaxUnresponsiveTimeInherited: true},
		}},
		{"pool=p1&Enabled=true", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(20), MaxUnresponsiveTimeInherited: true},
		}},
		{"pool=p1", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(20), MaxUnresponsiveTimeInherited: true},
		}},
		{"pool=p1&MaxUnresponsiveTime=30", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(30), MaxUnresponsiveTimeInherited: false},
		}},
		{"pool=p1&MaxUnresponsiveTime=0", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(0), MaxUnresponsiveTimeInherited: false},
		}},
		{"pool=p1&Enabled=false", map[string]healer.NodeHealerConfig{
			"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
			"p1": {Enabled: boolPtr(false), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(0), MaxUnresponsiveTimeInherited: false},
		}},
	}
	for i, t := range tests {
		configMap := doRequest(t.A)
		c.Assert(configMap, check.DeepEquals, t.B, check.Commentf("test %d", i+1))
	}
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePool, Value: "p1"},
		Owner:  s.token.GetUserName(),
		Kind:   "healing.update",
		StartCustomData: []map[string]interface{}{
			{"name": "pool", "value": "p1"},
			{"name": "MaxUnresponsiveTime", "value": "30"},
		},
	}, eventtest.HasEvent)
	request, err := http.NewRequest("DELETE", "/docker/healing/node?pool=p1&name=MaxUnresponsiveTime", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePool, Value: "p1"},
		Owner:  s.token.GetUserName(),
		Kind:   "healing.delete",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "MaxUnresponsiveTime"},
		},
	}, eventtest.HasEvent)
	configMap := doRequest("")
	c.Assert(configMap, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60), MaxUnresponsiveTime: intPtr(20)},
		"p1": {Enabled: boolPtr(false), MaxTimeSinceSuccess: intPtr(60), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTime: intPtr(20), MaxUnresponsiveTimeInherited: true},
	})
	request, err = http.NewRequest("DELETE", "/docker/healing/node", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	configMap = doRequest("")
	c.Assert(configMap, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"":   {},
		"p1": {Enabled: boolPtr(false), MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTimeInherited: true},
	})
	request, err = http.NewRequest("DELETE", "/docker/healing/node?pool=p1&name=Enabled", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	configMap = doRequest("")
	c.Assert(configMap, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"":   {},
		"p1": {EnabledInherited: true, MaxTimeSinceSuccessInherited: true, MaxUnresponsiveTimeInherited: true},
	})
}

func (s *S) TestNodeHealingConfigUpdateReadLimited(c *check.C) {
	doRequest := func(t auth.Token, code int, str string) map[string]healer.NodeHealerConfig {
		body := bytes.NewBufferString(str)
		request, err := http.NewRequest("POST", "/docker/healing/node", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+t.GetValue())
		recorder := httptest.NewRecorder()
		server := RunServer(true)
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, code)
		request, err = http.NewRequest("GET", "/docker/healing/node", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Authorization", "bearer "+t.GetValue())
		recorder = httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusOK)
		var configMap map[string]healer.NodeHealerConfig
		json.Unmarshal(recorder.Body.Bytes(), &configMap)
		return configMap
	}
	t := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermHealingUpdate,
		Context: permission.Context(permTypes.CtxPool, "p2"),
	}, permission.Permission{
		Scheme:  permission.PermHealingRead,
		Context: permission.Context(permTypes.CtxPool, "p2"),
	})
	data := doRequest(t, http.StatusForbidden, "Enabled=true&MaxTimeSinceSuccess=60")
	c.Assert(data, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"": {},
	})
	data = doRequest(s.token, http.StatusOK, "Enabled=true&MaxTimeSinceSuccess=60")
	c.Assert(data, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60)},
	})
	data = doRequest(t, http.StatusForbidden, "pool=p1&Enabled=true&MaxTimeSinceSuccess=20")
	c.Assert(data, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60)},
	})
	data = doRequest(t, http.StatusOK, "pool=p2&Enabled=true&MaxTimeSinceSuccess=20")
	c.Assert(data, check.DeepEquals, map[string]healer.NodeHealerConfig{
		"":   {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(60)},
		"p2": {Enabled: boolPtr(true), MaxTimeSinceSuccess: intPtr(20), MaxUnresponsiveTimeInherited: true},
	})
}

func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}

func (s *S) TestNodeRebalanceEmptyBodyHandler(c *check.C) {
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "n1",
		Pool:    "test1",
	})
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "n2",
		Pool:    "test1",
	})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	_, err = s.provisioner.AddUnitsToNode(&a, 4, "web", nil, "n1")
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/node/rebalance", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Matches, "(?s).*rebalancing - dry: false, force: true.*Units successfully rebalanced.*")
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	units, err := s.provisioner.Units(&a)
	c.Assert(err, check.IsNil)
	var nodes []string
	for _, u := range units {
		nodes = append(nodes, u.IP)
	}
	sort.Strings(nodes)
	c.Assert(nodes, check.DeepEquals, []string{"n1", "n1", "n2", "n2"})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypeGlobal},
		Owner:  s.token.GetUserName(),
		Kind:   "node.update.rebalance",
	}, eventtest.HasEvent)
}

func (s *S) TestNodeRebalanceFilters(c *check.C) {
	poolOpts := pool.AddPoolOptions{Name: "pool1"}
	err := pool.AddPool(poolOpts)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "n1",
	})
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "n2",
	})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	_, err = s.provisioner.AddUnitsToNode(&a, 4, "web", nil, "n1")
	c.Assert(err, check.IsNil)
	opts := provision.RebalanceNodesOptions{
		MetadataFilter: map[string]string{"pool": "pool1"},
		AppFilter:      []string{"myapp"},
		Dry:            true,
	}
	v, err := form.EncodeToValues(&opts)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/node/rebalance", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %s", recorder.Body.String()))
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Matches, `(?s).*rebalancing - dry: true, force: true.*filtering apps: \[myapp\].*filtering pool: pool1.*`)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePool, Value: "pool1"},
		Owner:  s.token.GetUserName(),
		Kind:   "node.update.rebalance",
		StartCustomData: []map[string]interface{}{
			{"name": "AppFilter.0", "value": "myapp"},
			{"name": "Dry", "value": "true"},
			{"name": "Force", "value": ""},
			{"name": "MetadataFilter.pool", "value": "pool1"},
			{"name": "Pool", "value": ""},
			{"name": "Event", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestInfoNodeHandlerNotFound(c *check.C) {
	nodeAddr := "http://host1.com:2375"
	req, err := http.NewRequest("GET", "/node/"+nodeAddr, nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req.Header.Set("Authorization", s.token.GetValue())
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestInfoNodeHandlerNodeOnly(c *check.C) {
	nodeAddr := "http://host1.com:2375"
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: nodeAddr,
		Pool:    "pool1",
		IaaSID:  "teste123",
	})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("GET", "/node/"+nodeAddr, nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req.Header.Set("Authorization", s.token.GetValue())
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Header().Get("Content-Type"), check.Equals, "application/json")
	var result apiTypes.InfoNodeResponse
	err = json.Unmarshal(rec.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Node, check.DeepEquals, provision.NodeSpec{
		Address: nodeAddr, Provisioner: "fake", Pool: "pool1", Status: "enabled", IaaSID: "teste123", Metadata: map[string]string{},
	})
}

func (s *S) TestInfoNodeHandler(c *check.C) {
	nodeAddr := "host1.com:2375"
	err := s.provisioner.AddNode(provision.AddNodeOptions{
		Address: nodeAddr,
		Pool:    "pool1",
	})
	c.Assert(err, check.IsNil)
	node, err := s.provisioner.GetNode(nodeAddr)
	c.Assert(err, check.IsNil)
	nodeHealer := &healer.NodeHealer{}
	checks := []provision.NodeCheckResult{
		{Name: "ok1", Successful: true},
		{Name: "ok2", Successful: true},
	}
	err = nodeHealer.UpdateNodeData([]string{node.Address()}, checks)
	c.Assert(err, check.IsNil)
	factory, _ := iaasTesting.NewHealerIaaSConstructorWithInst(nodeAddr)
	iaas.RegisterIaasProvider("test123", factory)
	_, err = iaas.CreateMachineForIaaS("test123", map[string]string{"id": "teste123", "host": "host1.com", "port": "2375"})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "fake", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{
		ID:      "a834h983j498j",
		AppName: "fake",
		Address: &url.URL{
			Host: "host1.com:2375",
		},
	}
	s.provisioner.AddUnit(&a, unit)
	req, err := http.NewRequest("GET", "/node/"+nodeAddr, nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	req.Header.Set("Authorization", s.token.GetValue())
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(rec.Header().Get("Content-Type"), check.Equals, "application/json")
	var result apiTypes.InfoNodeResponse
	err = json.Unmarshal(rec.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Node, check.DeepEquals, provision.NodeSpec{
		Address: nodeAddr, Provisioner: "fake", Pool: "pool1", Status: "enabled", IaaSID: "test123", Metadata: map[string]string{},
	})
	c.Assert(result.Status.Address, check.Equals, nodeAddr)
	c.Assert(result.Status.Checks, check.HasLen, 1)
	c.Assert(result.Status.Checks[0].Checks, check.DeepEquals, checks)
	c.Assert(result.Units, check.DeepEquals, []provision.Unit{unit})
}
