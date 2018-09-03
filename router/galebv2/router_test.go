// Copyright 2018 tsuru authors. All rights reserved.
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
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
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
	sync.Mutex
	targets      map[string]interface{}
	pools        map[string]interface{}
	virtualhosts map[string]interface{}
	rules        map[string]interface{}
	ruleordered  map[string]interface{}
	items        map[string]map[string]interface{}
	idCounter    int
	router       *mux.Router
	errors       map[string]error
}

func NewFakeGalebServer() (*fakeGalebServer, error) {
	server := &fakeGalebServer{
		targets:      make(map[string]interface{}),
		pools:        make(map[string]interface{}),
		virtualhosts: make(map[string]interface{}),
		rules:        make(map[string]interface{}),
		ruleordered:  make(map[string]interface{}),
	}
	server.items = map[string]map[string]interface{}{
		"target":      server.targets,
		"pool":        server.pools,
		"virtualhost": server.virtualhosts,
		"rule":        server.rules,
		"ruleordered": server.ruleordered,
	}
	r := mux.NewRouter()
	r.HandleFunc("/api/token", server.getToken).Methods("GET")
	r.HandleFunc("/api/target", server.createTarget).Methods("POST")
	r.HandleFunc("/api/pool", server.createPool).Methods("POST")
	r.HandleFunc("/api/pool/{id}", server.updatePool).Methods("PATCH")
	r.HandleFunc("/api/pool/{id}/targets", server.findTargetsByPool).Methods("GET")
	r.HandleFunc("/api/rule", server.createRule).Methods("POST")
	r.HandleFunc("/api/rule/{id}/rulesOrdered", server.findRulesOrderedByRule).Methods("GET")
	r.HandleFunc("/api/ruleordered", server.createRuleOrdered).Methods("POST")
	r.HandleFunc("/api/virtualhost", server.createVirtualhost).Methods("POST")
	r.HandleFunc("/api/virtualhost/{id}", server.updateVirtualHost).Methods("PATCH")
	r.HandleFunc("/api/virtualhost/{id}/virtualhostgroup", server.findVirtualhostGroupByVirtualHost).Methods("GET")
	r.HandleFunc("/api/virtualhostgroup/{id}/virtualhosts", server.findVirtualhostsByVirtualHostGroup).Methods("GET")
	r.HandleFunc("/api/{item}/{id}", server.findItem).Methods("GET")
	r.HandleFunc("/api/{item}/{id}", server.destroyItem).Methods("DELETE")
	r.HandleFunc("/api/{item}/search/findByName", server.findItemByNameHandler).Methods("GET")
	server.router = r
	return server, nil
}

func (s *fakeGalebServer) prepareError(method, path, msg string) {
	if s.errors == nil {
		s.errors = map[string]error{}
	}
	s.errors[method+"_"+path] = errors.New(msg)
}

func (s *fakeGalebServer) checkError(method, path string) error {
	if s.errors == nil {
		return nil
	}
	return s.errors[method+"_"+path]
}

func (s *fakeGalebServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Lock()
	defer s.Unlock()
	s.router.ServeHTTP(w, r)
}

func (s *fakeGalebServer) getToken(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"token":"abc"}`))
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

func (s *fakeGalebServer) findVirtualhostGroupByVirtualHost(w http.ResponseWriter, r *http.Request) {
	virtualhostId := mux.Vars(r)["id"]
	var virtualhost *galebClient.VirtualHost
	virtualhost, ok := s.virtualhosts[virtualhostId].(*galebClient.VirtualHost)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	vhId, _ := strconv.Atoi(virtualhost.VirtualHostGroup[strings.LastIndex(virtualhost.VirtualHostGroup, "/")+1:])
	json.NewEncoder(w).Encode(struct {
		ID int `json:"id"`
	}{
		ID: vhId,
	})
}

func (s *fakeGalebServer) findVirtualhostsByVirtualHostGroup(w http.ResponseWriter, r *http.Request) {
	virtualhostgroupId := mux.Vars(r)["id"]
	var ret []interface{}
	for _, item := range s.virtualhosts {
		vh := item.(*galebClient.VirtualHost)
		if strings.HasSuffix(vh.VirtualHostGroup, "/"+virtualhostgroupId) {
			ret = append(ret, item)
		}
	}
	json.NewEncoder(w).Encode(makeSearchRsp("virtualhost", ret...))
}

func (s *fakeGalebServer) findTargetsByPool(w http.ResponseWriter, r *http.Request) {
	poolId := mux.Vars(r)["id"]
	var pool *galebClient.Pool
	pool, ok := s.pools[poolId].(*galebClient.Pool)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var ret []interface{}
	for i, item := range s.targets {
		target := item.(*galebClient.Target)
		if target.BackendPool == pool.FullId() {
			ret = append(ret, s.targets[i])
		}
	}
	json.NewEncoder(w).Encode(makeSearchRsp("target", ret...))
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
	target.Status = map[string]string{"1": "OK"}
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
	w.Header().Set("Location", target.Links.Self.Href)
	w.WriteHeader(http.StatusCreated)
}

func (s *fakeGalebServer) createPool(w http.ResponseWriter, r *http.Request) {
	err := s.checkError(r.Method, r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var pool galebClient.Pool
	pool.Status = map[string]string{"1": "OK"}
	json.NewDecoder(r.Body).Decode(&pool)
	poolsWithName := s.findItemByName("pool", pool.Name)
	if len(poolsWithName) > 0 {
		w.WriteHeader(http.StatusConflict)
		return
	}
	s.idCounter++
	pool.ID = s.idCounter
	pool.Links.Self.Href = fmt.Sprintf("http://%s%s/%d", r.Host, r.URL.String(), pool.ID)
	s.pools[strconv.Itoa(pool.ID)] = &pool
	w.Header().Set("Location", pool.Links.Self.Href)
	w.WriteHeader(http.StatusCreated)
}

func (s *fakeGalebServer) updatePool(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var pool galebClient.Pool
	json.NewDecoder(r.Body).Decode(&pool)
	existingPool, ok := s.pools[id].(*galebClient.Pool)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	existingPool.HcBody = pool.HcBody
	existingPool.HcHttpStatusCode = pool.HcHttpStatusCode
	existingPool.HcPath = pool.HcPath
	w.WriteHeader(http.StatusNoContent)
}

func (s *fakeGalebServer) createRule(w http.ResponseWriter, r *http.Request) {
	err := s.checkError(r.Method, r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var rule galebClient.Rule
	rule.Status = map[string]string{"1": "OK"}
	json.NewDecoder(r.Body).Decode(&rule)
	for _, rInt := range s.rules {
		r := rInt.(*galebClient.Rule)
		if r.Name == rule.Name {
			w.WriteHeader(http.StatusConflict)
			return
		}
	}
	s.idCounter++
	rule.ID = s.idCounter
	rule.Links.Self.Href = fmt.Sprintf("http://%s%s/%d", r.Host, r.URL.String(), rule.ID)
	s.rules[strconv.Itoa(rule.ID)] = &rule
	w.Header().Set("Location", rule.Links.Self.Href)
	w.WriteHeader(http.StatusCreated)
}

func (s *fakeGalebServer) createRuleOrdered(w http.ResponseWriter, r *http.Request) {
	var ruleOrdered galebClient.RuleOrdered
	ruleOrdered.Status = map[string]string{"1": "OK"}
	json.NewDecoder(r.Body).Decode(&ruleOrdered)
	s.idCounter++
	ruleOrdered.ID = s.idCounter
	ruleOrdered.Links.Self.Href = fmt.Sprintf("http://%s%s/%d", r.Host, r.URL.String(), ruleOrdered.ID)
	s.ruleordered[strconv.Itoa(ruleOrdered.ID)] = &ruleOrdered
	w.Header().Set("Location", ruleOrdered.Links.Self.Href)
	w.WriteHeader(http.StatusCreated)
}

func (s *fakeGalebServer) findRulesOrderedByRule(w http.ResponseWriter, r *http.Request) {
	ruleId := mux.Vars(r)["id"]
	var items []interface{}
	for _, item := range s.ruleordered {
		ruleOrdered := item.(*galebClient.RuleOrdered)
		if strings.HasSuffix(ruleOrdered.Rule, "/"+ruleId) {
			items = append(items, item)
		}
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(makeSearchRsp("ruleordered", items...))
}

func (s *fakeGalebServer) createVirtualhost(w http.ResponseWriter, r *http.Request) {
	var virtualhost galebClient.VirtualHost
	virtualhost.Status = map[string]string{"1": "OK"}
	json.NewDecoder(r.Body).Decode(&virtualhost)
	if len(s.findItemByName("virtualhost", virtualhost.Name)) > 0 {
		w.WriteHeader(http.StatusConflict)
		return
	}
	s.idCounter++
	virtualhost.ID = s.idCounter
	virtualhost.Links.Self.Href = fmt.Sprintf("http://%s%s/%d", r.Host, r.URL.String(), virtualhost.ID)
	if virtualhost.VirtualHostGroup == "" {
		s.idCounter++
		virtualhost.VirtualHostGroup = fmt.Sprintf("http://%s/virtualhostgroup/%d", r.Host, s.idCounter)
	}
	s.virtualhosts[strconv.Itoa(virtualhost.ID)] = &virtualhost
	w.Header().Set("Location", virtualhost.Links.Self.Href)
	w.WriteHeader(http.StatusCreated)
}

func (s *fakeGalebServer) updateVirtualHost(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var vh galebClient.VirtualHost
	json.NewDecoder(r.Body).Decode(&vh)
	existingVH, ok := s.virtualhosts[id].(*galebClient.VirtualHost)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	existingVH.VirtualHostGroup = vh.VirtualHostGroup
	w.WriteHeader(http.StatusNoContent)
}

func init() {
	suite := &routertest.RouterSuite{
		SetUpSuiteFunc: func(c *check.C) {
			config.Set("routers:galeb:username", "myusername")
			config.Set("routers:galeb:password", "mypassword")
			config.Set("routers:galeb:domain", "galeb.com")
			config.Set("routers:galeb:use-token", true)
			config.Set("routers:galeb:type", "galebv2")
			config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
			config.Set("database:name", "router_galebv2_tests")
		},
	}
	var server *httptest.Server
	var fakeServer *fakeGalebServer
	suite.SetUpTestFunc = func(c *check.C) {
		clientCache.cache = nil
		var err error
		fakeServer, err = NewFakeGalebServer()
		c.Assert(err, check.IsNil)
		server = httptest.NewServer(fakeServer)
		config.Set("routers:galeb:api-url", server.URL+"/api")
		gRouter, err := createRouter("galeb", "routers:galeb")
		c.Assert(err, check.IsNil)
		suite.Router = gRouter
		conn, err := db.Conn()
		c.Assert(err, check.IsNil)
		defer conn.Close()
		dbtest.ClearAllCollections(conn.Collection("router_galeb_tests").Database)
	}
	suite.TearDownTestFunc = func(c *check.C) {
		server.Close()
		c.Check(fakeServer.targets, check.DeepEquals, map[string]interface{}{})
		c.Check(fakeServer.pools, check.DeepEquals, map[string]interface{}{})
		c.Check(fakeServer.virtualhosts, check.DeepEquals, map[string]interface{}{})
		c.Check(fakeServer.rules, check.DeepEquals, map[string]interface{}{})
		c.Check(fakeServer.ruleordered, check.DeepEquals, map[string]interface{}{})
	}
	check.Suite(suite)
}

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	config.Set("routers:galeb:username", "myusername")
	config.Set("routers:galeb:password", "mypassword")
	config.Set("routers:galeb:domain", "galeb.com")
	config.Set("routers:galeb:use-token", true)
	config.Set("routers:galeb:type", "galebv2")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "router_galebv2_tests")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Collection("router_galeb_tests").Database)
	clientCache.cache = nil
}

func (s *S) TestCreateRouterConcurrent(c *check.C) {
	config.Set("routers:r1:domain", "galeb1.com")
	config.Set("routers:r2:domain", "galeb2.com")
	config.Set("routers:r1:api-url", "galeb1.com")
	config.Set("routers:r2:api-url", "galeb2.com")
	defer config.Unset("routers:r1:domain")
	defer config.Unset("routers:r2:domain")
	defer config.Unset("routers:r1:api-url")
	defer config.Unset("routers:r2:api-url")
	nConcurrent := 50
	prefixes := []string{"routers:r1", "routers:r2"}
	var routers []*galebRouter
	var mu sync.Mutex
	wg := sync.WaitGroup{}
	for i := 0; i < nConcurrent; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := createRouter("rx", prefixes[i%len(prefixes)])
			c.Assert(err, check.IsNil)
			mu.Lock()
			routers = append(routers, r.(*galebRouter))
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	c.Assert(routers, check.HasLen, nConcurrent)
	clients := map[string]int{}
	for _, r := range routers {
		clients[fmt.Sprintf("%p", r.client)]++
	}
	c.Assert(clients, check.HasLen, 2)
	for _, v := range clients {
		c.Assert(v, check.Equals, nConcurrent/2)
	}
}

func (s *S) TestAddBackendPartialFailure(c *check.C) {
	fakeServer, err := NewFakeGalebServer()
	c.Assert(err, check.IsNil)
	server := httptest.NewServer(fakeServer)
	defer server.Close()
	config.Set("routers:galeb:api-url", server.URL+"/api")
	gRouter, err := createRouter("galeb", "routers:galeb")
	c.Assert(err, check.IsNil)
	fakeServer.prepareError("POST", "/api/rule", "error on AddRuleToPool")
	err = gRouter.AddBackend(routertest.FakeApp{Name: "backend1"})
	c.Assert(err, check.ErrorMatches, "(?s)POST /rule: invalid response code: 500: error on AddRuleToPool.*")
	c.Check(fakeServer.targets, check.DeepEquals, map[string]interface{}{})
	c.Check(fakeServer.pools, check.DeepEquals, map[string]interface{}{})
	c.Check(fakeServer.virtualhosts, check.DeepEquals, map[string]interface{}{})
	c.Check(fakeServer.rules, check.DeepEquals, map[string]interface{}{})
	c.Check(fakeServer.ruleordered, check.DeepEquals, map[string]interface{}{})
}

func (s *S) TestAddBackendPartialFailureExisting(c *check.C) {
	fakeServer, err := NewFakeGalebServer()
	c.Assert(err, check.IsNil)
	server := httptest.NewServer(fakeServer)
	defer server.Close()
	config.Set("routers:galeb:api-url", server.URL+"/api")
	gRouter, err := createRouter("galeb", "routers:galeb")
	c.Assert(err, check.IsNil)
	err = gRouter.AddBackend(routertest.FakeApp{Name: "backend1"})
	c.Assert(err, check.IsNil)
	fakeServer.prepareError("POST", "/api/rule", "error on AddRuleToPool")
	err = gRouter.AddBackend(routertest.FakeApp{Name: "backend1"})
	c.Assert(err, check.ErrorMatches, "(?s)POST /rule: invalid response code: 500: error on AddRuleToPool.*")
	c.Check(fakeServer.pools, check.Not(check.DeepEquals), map[string]interface{}{})
	c.Check(fakeServer.virtualhosts, check.Not(check.DeepEquals), map[string]interface{}{})
	c.Check(fakeServer.rules, check.Not(check.DeepEquals), map[string]interface{}{})
	c.Check(fakeServer.ruleordered, check.Not(check.DeepEquals), map[string]interface{}{})
}

func (s *S) TestAddBackendPartialFailureInFirstResource(c *check.C) {
	fakeServer, err := NewFakeGalebServer()
	c.Assert(err, check.IsNil)
	server := httptest.NewServer(fakeServer)
	defer server.Close()
	config.Set("routers:galeb:api-url", server.URL+"/api")
	gRouter, err := createRouter("galeb", "routers:galeb")
	c.Assert(err, check.IsNil)
	fakeServer.prepareError("POST", "/api/pool", "error in pool create")
	err = gRouter.AddBackend(routertest.FakeApp{Name: "backend1"})
	c.Assert(err, check.ErrorMatches, "(?s)POST /pool: invalid response code: 500: error in pool create.*")
	c.Check(fakeServer.targets, check.DeepEquals, map[string]interface{}{})
	c.Check(fakeServer.pools, check.DeepEquals, map[string]interface{}{})
	c.Check(fakeServer.virtualhosts, check.DeepEquals, map[string]interface{}{})
	c.Check(fakeServer.rules, check.DeepEquals, map[string]interface{}{})
	c.Check(fakeServer.ruleordered, check.DeepEquals, map[string]interface{}{})
}

func (s *S) TestAddBackendPartialFailureInFirstResourceExisting(c *check.C) {
	fakeServer, err := NewFakeGalebServer()
	c.Assert(err, check.IsNil)
	server := httptest.NewServer(fakeServer)
	defer server.Close()
	config.Set("routers:galeb:api-url", server.URL+"/api")
	gRouter, err := createRouter("galeb", "routers:galeb")
	c.Assert(err, check.IsNil)
	err = gRouter.AddBackend(routertest.FakeApp{Name: "backend1"})
	c.Assert(err, check.IsNil)
	fakeServer.prepareError("POST", "/api/pool", "error in pool create")
	err = gRouter.AddBackend(routertest.FakeApp{Name: "backend1"})
	c.Assert(err, check.ErrorMatches, "(?s)POST /pool: invalid response code: 500: error in pool create.*")
	c.Check(fakeServer.pools, check.Not(check.DeepEquals), map[string]interface{}{})
	c.Check(fakeServer.virtualhosts, check.Not(check.DeepEquals), map[string]interface{}{})
	c.Check(fakeServer.rules, check.Not(check.DeepEquals), map[string]interface{}{})
	c.Check(fakeServer.ruleordered, check.Not(check.DeepEquals), map[string]interface{}{})
}
