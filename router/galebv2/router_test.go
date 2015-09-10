// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package galebv2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	galebClient "github.com/tsuru/tsuru/router/galebv2/client"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type fakeGalebServer struct {
	targets      map[string]interface{}
	virtualhosts map[string]interface{}
	rules        map[string]interface{}
	items        map[string]map[string]interface{}
	idCounter    int
	router       *mux.Router
}

func NewFakeGalebServer() (*fakeGalebServer, error) {
	server := &fakeGalebServer{
		targets:      make(map[string]interface{}),
		virtualhosts: make(map[string]interface{}),
		rules:        make(map[string]interface{}),
	}
	server.items = map[string]map[string]interface{}{
		"target":      server.targets,
		"virtualhost": server.virtualhosts,
		"rule":        server.rules,
	}
	r := mux.NewRouter()
	r.HandleFunc("/api/target", server.createTarget).Methods("POST")
	r.HandleFunc("/api/rule", server.createRule).Methods("POST")
	r.HandleFunc("/api/virtualhost", server.createVirtualhost).Methods("POST")
	r.HandleFunc("/api/rule/{id}", server.addRuleVirtualhost).Methods("PATCH")
	r.HandleFunc("/api/{item}/{id}", server.findItem).Methods("GET")
	r.HandleFunc("/api/{item}/{id}", server.destroyItem).Methods("DELETE")
	r.HandleFunc("/api/{item}/search/findByName", server.findItemByNameHandler).Methods("GET")
	r.HandleFunc("/api/target/search/findByParentName", server.findTargetByParentName).Methods("GET")
	r.HandleFunc("/api/rule/search/findByTargetName", server.findRuleByTargetName).Methods("GET")
	r.HandleFunc("/api/rule/search/findByNameAndParent", server.findRuleByNameAndParent).Methods("GET")
	server.router = r
	return server, nil
}

func (s *fakeGalebServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *fakeGalebServer) findItem(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	item := mux.Vars(r)["item"]
	obj, ok := s.items[item][id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(obj)
}

type searchRsp struct {
	Embedded map[string][]interface{} `json:"_embedded"`
}

func makeSearchRsp(itemName string, items ...interface{}) searchRsp {
	return searchRsp{Embedded: map[string][]interface{}{itemName: items}}
}

func (s *fakeGalebServer) findItemByNameHandler(w http.ResponseWriter, r *http.Request) {
	itemName := mux.Vars(r)["item"]
	wantedName := r.URL.Query().Get("name")
	ret := s.findItemByName(itemName, wantedName)
	json.NewEncoder(w).Encode(makeSearchRsp(itemName, ret...))
}

func (s *fakeGalebServer) findItemByName(itemName string, wantedName string) []interface{} {
	items := s.items[itemName]
	var ret []interface{}
	for i, item := range items {
		name := item.(interface {
			GetName() string
		}).GetName()
		if name == wantedName {
			ret = append(ret, items[i])
		}
	}
	return ret
}

func (s *fakeGalebServer) findTargetByParentName(w http.ResponseWriter, r *http.Request) {
	wantedName := r.URL.Query().Get("name")
	var ret []interface{}
	for i, item := range s.targets {
		target := item.(*galebClient.Target)
		if target.BackendPool == "" {
			continue
		}
		poolId := target.BackendPool[strings.LastIndex(target.BackendPool, "/")+1:]
		parentTarget := s.targets[poolId].(*galebClient.Target)
		if parentTarget.Name == wantedName {
			ret = append(ret, s.targets[i])
		}
	}
	json.NewEncoder(w).Encode(makeSearchRsp("target", ret...))
}

func (s *fakeGalebServer) findRuleByTargetName(w http.ResponseWriter, r *http.Request) {
	wantedName := r.URL.Query().Get("name")
	var ret []interface{}
	for i, item := range s.rules {
		rule := item.(*galebClient.Rule)
		if rule.BackendPool == "" {
			continue
		}
		poolId := rule.BackendPool[strings.LastIndex(rule.BackendPool, "/")+1:]
		target := s.targets[poolId].(*galebClient.Target)
		if target.Name == wantedName {
			ret = append(ret, s.rules[i])
		}
	}
	json.NewEncoder(w).Encode(makeSearchRsp("rule", ret...))
}

func (s *fakeGalebServer) findRuleByNameAndParent(w http.ResponseWriter, r *http.Request) {
	wantedName := r.URL.Query().Get("name")
	wantedParent := r.URL.Query().Get("parent")
	var ret []interface{}
	for i, item := range s.rules {
		rule := item.(*galebClient.Rule)
		if wantedParent == "" && rule.VirtualHost == "" && rule.Name == wantedName {
			ret = append(ret, s.rules[i])
			continue
		}
		if rule.VirtualHost == "" {
			continue
		}
		vhId := rule.VirtualHost[strings.LastIndex(rule.VirtualHost, "/")+1:]
		vh := s.virtualhosts[vhId].(*galebClient.VirtualHost)
		if vh.Name != wantedParent {
			continue
		}
		if rule.Name == wantedName {
			ret = append(ret, s.rules[i])
		}
	}
	json.NewEncoder(w).Encode(makeSearchRsp("rule", ret...))
}

func (s *fakeGalebServer) destroyItem(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	item := mux.Vars(r)["item"]
	_, ok := s.items[item][id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	delete(s.items[item], id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *fakeGalebServer) createTarget(w http.ResponseWriter, r *http.Request) {
	var target galebClient.Target
	json.NewDecoder(r.Body).Decode(&target)
	targetsWithName := s.findItemByName("target", target.Name)
	for _, item := range targetsWithName {
		otherTarget := item.(*galebClient.Target)
		if otherTarget.BackendPool == target.BackendPool {
			w.WriteHeader(http.StatusConflict)
			return
		}
	}
	s.idCounter++
	target.ID = s.idCounter
	target.Links.Self.Href = fmt.Sprintf("http://%s%s/%d", r.Host, r.URL.String(), target.ID)
	s.targets[strconv.Itoa(target.ID)] = &target
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(&target)
}

func (s *fakeGalebServer) createRule(w http.ResponseWriter, r *http.Request) {
	var rule galebClient.Rule
	json.NewDecoder(r.Body).Decode(&rule)
	s.idCounter++
	rule.ID = s.idCounter
	rule.Links.Self.Href = fmt.Sprintf("http://%s%s/%d", r.Host, r.URL.String(), rule.ID)
	s.rules[strconv.Itoa(rule.ID)] = &rule
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(&rule)
}

func (s *fakeGalebServer) addRuleVirtualhost(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	baseRule := s.rules[id].(*galebClient.Rule)
	rule := *baseRule
	var tmpRule galebClient.Rule
	json.NewDecoder(r.Body).Decode(&tmpRule)
	s.idCounter++
	rule.ID = s.idCounter
	rule.VirtualHost = tmpRule.VirtualHost
	rule.Links.Self.Href = fmt.Sprintf("http://%s/api/rule/%d", r.Host, rule.ID)
	s.rules[strconv.Itoa(rule.ID)] = &rule
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(&rule)
}

func (s *fakeGalebServer) createVirtualhost(w http.ResponseWriter, r *http.Request) {
	var virtualhost galebClient.VirtualHost
	json.NewDecoder(r.Body).Decode(&virtualhost)
	if len(s.findItemByName("virtualhost", virtualhost.Name)) > 0 {
		w.WriteHeader(http.StatusConflict)
		return
	}
	s.idCounter++
	virtualhost.ID = s.idCounter
	virtualhost.Links.Self.Href = fmt.Sprintf("http://%s%s/%d", r.Host, r.URL.String(), virtualhost.ID)
	s.virtualhosts[strconv.Itoa(virtualhost.ID)] = &virtualhost
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(&virtualhost)
}

func init() {
	suite := &routertest.RouterSuite{
		SetUpSuiteFunc: func(c *check.C) {
			config.Set("routers:galeb:username", "myusername")
			config.Set("routers:galeb:password", "mypassword")
			config.Set("routers:galeb:domain", "galeb.com")
			config.Set("routers:galeb:type", "galeb")
			config.Set("database:url", "127.0.0.1:27017")
			config.Set("database:name", "router_galebv2_tests")
		},
	}
	var server *httptest.Server
	suite.SetUpTestFunc = func(c *check.C) {
		handler, err := NewFakeGalebServer()
		c.Assert(err, check.IsNil)
		server = httptest.NewServer(handler)
		config.Set("routers:galeb:api-url", server.URL+"/api")
		gRouter, err := createRouter("routers:galeb")
		c.Assert(err, check.IsNil)
		suite.Router = gRouter
		conn, err := db.Conn()
		c.Assert(err, check.IsNil)
		defer conn.Close()
		dbtest.ClearAllCollections(conn.Collection("router_galebv2_tests").Database)
	}
	suite.TearDownTestFunc = func(c *check.C) {
		server.Close()
	}
	check.Suite(suite)
}
