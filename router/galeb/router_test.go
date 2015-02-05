// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package galeb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/apitest"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/router"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct {
	conn    *db.Storage
	server  *httptest.Server
	handler apitest.MultiTestHandler
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("galeb:username", "myusername")
	config.Set("galeb:password", "mypassword")
	config.Set("galeb:domain", "galeb.com")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_galeb_tests_s")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) SetUpTest(c *gocheck.C) {
	s.handler = apitest.MultiTestHandler{}
	s.server = httptest.NewServer(&s.handler)
	config.Set("galeb:api-url", s.server.URL+"/api")
	dbtest.ClearAllCollections(s.conn.Collection("router_galeb_tests").Database)
}

func (s *S) TearDownTest(c *gocheck.C) {
	s.server.Close()
}

func (s *S) TestAddBackend(c *gocheck.C) {
	s.handler.ConditionalContent = map[string]interface{}{
		"/api/backendpool/": `{"_links":{"self":"pool1"}}`,
		"/api/rule/":        `{"_links":{"self":"rule1"}}`,
		"/api/virtualhost/": `{"_links":{"self":"vh1"}}`,
	}
	s.handler.RspCode = http.StatusCreated
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.AddBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/backendpool/", "/api/rule/", "/api/virtualhost/"})
	data, err := getGalebData("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(*data, gocheck.DeepEquals, galebData{
		Name:          "myapp",
		BackendPoolId: "pool1",
		RootRuleId:    "rule1",
		VirtualHostId: "vh1",
		CNames:        []galebCNameData{},
		Reals:         []galebRealData{},
	})
	result := map[string]string{}
	err = json.Unmarshal(s.handler.Body[0], &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, map[string]string{
		"name": "tsuru-backendpool-myapp", "environment": "", "farmtype": "", "plan": "", "project": "", "loadbalancepolicy": "",
	})
	result = map[string]string{}
	err = json.Unmarshal(s.handler.Body[1], &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, map[string]string{
		"name": "tsuru-rootrule-myapp", "match": "/", "backendpool": "pool1", "ruletype": "", "project": "",
	})
	result = map[string]string{}
	err = json.Unmarshal(s.handler.Body[2], &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, map[string]string{
		"name": "myapp.galeb.com", "farmtype": "", "plan": "", "environment": "", "project": "", "rule_default": "rule1",
	})
	backendName, err := router.Retrieve("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(backendName, gocheck.Equals, "myapp")
}

func (s *S) TestRemoveBackend(c *gocheck.C) {
	s.handler.RspCode = http.StatusNoContent
	err := router.Store("myapp", "myapp", routerName)
	c.Assert(err, gocheck.IsNil)
	data := galebData{
		Name:          "myapp",
		BackendPoolId: s.server.URL + "/api/backend1",
		RootRuleId:    s.server.URL + "/api/rule1",
		VirtualHostId: s.server.URL + "/api/vh1",
		CNames: []galebCNameData{
			{CName: "my.1.cname", VirtualHostId: s.server.URL + "/api/vh2"},
			{CName: "my.2.cname", VirtualHostId: s.server.URL + "/api/vh3"},
		},
	}
	err = data.save()
	c.Assert(err, gocheck.IsNil)
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.RemoveBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{
		"/api/vh1", "/api/vh2", "/api/vh3", "/api/rule1", "/api/backend1",
	})
	_, err = router.Retrieve("myapp")
	c.Assert(err, gocheck.ErrorMatches, "not found")
	_, err = getGalebData("myapp")
	c.Assert(err, gocheck.ErrorMatches, "not found")
}

func (s *S) TestAddRoute(c *gocheck.C) {
	err := router.Store("myapp", "myapp", routerName)
	c.Assert(err, gocheck.IsNil)
	data := galebData{
		Name:          "myapp",
		BackendPoolId: "mybackendpoolid",
	}
	err = data.save()
	c.Assert(err, gocheck.IsNil)
	s.handler.ConditionalContent = map[string]interface{}{
		"/api/backend/": `{"_links":{"self":"backend1"}}`,
	}
	s.handler.RspCode = http.StatusCreated
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.AddRoute("myapp", "10.9.2.1:44001")
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/backend/"})
	dbData, err := getGalebData("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbData.Reals, gocheck.DeepEquals, []galebRealData{
		{Real: "10.9.2.1:44001", BackendId: "backend1"},
	})
	result := map[string]interface{}{}
	err = json.Unmarshal(s.handler.Body[0], &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, map[string]interface{}{
		"ip": "10.9.2.1", "port": float64(44001), "backendpool": "mybackendpoolid",
	})
}

func (s *S) TestAddRouteParsesURL(c *gocheck.C) {
	err := router.Store("myapp", "myapp", routerName)
	c.Assert(err, gocheck.IsNil)
	data := galebData{
		Name:          "myapp",
		BackendPoolId: "mybackendpoolid",
	}
	err = data.save()
	c.Assert(err, gocheck.IsNil)
	s.handler.ConditionalContent = map[string]interface{}{
		"/api/backend/": `{"_links":{"self":"backend1"}}`,
	}
	s.handler.RspCode = http.StatusCreated
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.AddRoute("myapp", "http://10.9.9.9:11001/")
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/backend/"})
	dbData, err := getGalebData("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbData.Reals, gocheck.DeepEquals, []galebRealData{
		{Real: "10.9.9.9:11001", BackendId: "backend1"},
	})
	result := map[string]interface{}{}
	err = json.Unmarshal(s.handler.Body[0], &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, map[string]interface{}{
		"ip": "10.9.9.9", "port": float64(11001), "backendpool": "mybackendpoolid",
	})
}

func (s *S) TestRemoveRoute(c *gocheck.C) {
	err := router.Store("myapp", "myapp", routerName)
	c.Assert(err, gocheck.IsNil)
	data := galebData{
		Name: "myapp",
		Reals: []galebRealData{
			{Real: "10.1.1.10", BackendId: s.server.URL + "/api/backend1"},
		},
	}
	err = data.save()
	c.Assert(err, gocheck.IsNil)
	s.handler.RspCode = http.StatusNoContent
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.RemoveRoute("myapp", "10.1.1.10")
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/backend1"})
	dbData, err := getGalebData("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbData.Reals, gocheck.DeepEquals, []galebRealData{})
}

func (s *S) TestRemoveRouteParsesURL(c *gocheck.C) {
	err := router.Store("myapp", "myapp", routerName)
	c.Assert(err, gocheck.IsNil)
	data := galebData{
		Name: "myapp",
		Reals: []galebRealData{
			{Real: "10.1.1.10:1010", BackendId: s.server.URL + "/api/backend1"},
		},
	}
	err = data.save()
	c.Assert(err, gocheck.IsNil)
	s.handler.RspCode = http.StatusNoContent
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.RemoveRoute("myapp", "https://10.1.1.10:1010/")
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/backend1"})
	dbData, err := getGalebData("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbData.Reals, gocheck.DeepEquals, []galebRealData{})
}

func (s *S) TestSetCName(c *gocheck.C) {
	err := router.Store("myapp", "myapp", routerName)
	c.Assert(err, gocheck.IsNil)
	data := galebData{
		Name:       "myapp",
		RootRuleId: "myrootrule",
	}
	err = data.save()
	c.Assert(err, gocheck.IsNil)
	s.handler.ConditionalContent = map[string]interface{}{
		"/api/virtualhost/": `{"_links":{"self":"vhX"}}`,
	}
	s.handler.RspCode = http.StatusCreated
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.SetCName("my.new.cname", "myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/virtualhost/"})
	dbData, err := getGalebData("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbData.CNames, gocheck.DeepEquals, []galebCNameData{
		{CName: "my.new.cname", VirtualHostId: "vhX"},
	})
	result := map[string]interface{}{}
	err = json.Unmarshal(s.handler.Body[0], &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, map[string]interface{}{
		"name": "my.new.cname", "farmtype": "", "plan": "", "environment": "", "project": "", "rule_default": "myrootrule",
	})
}

func (s *S) TestUnsetCName(c *gocheck.C) {
	err := router.Store("myapp", "myapp", routerName)
	c.Assert(err, gocheck.IsNil)
	data := galebData{
		Name: "myapp",
		CNames: []galebCNameData{
			{CName: "my.new.cname", VirtualHostId: s.server.URL + "/api/vh999"},
		},
	}
	err = data.save()
	c.Assert(err, gocheck.IsNil)
	s.handler.RspCode = http.StatusNoContent
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.UnsetCName("my.new.cname", "myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.handler.Url, gocheck.DeepEquals, []string{"/api/vh999"})
	dbData, err := getGalebData("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbData.CNames, gocheck.DeepEquals, []galebCNameData{})
}

func (s *S) TestRoutes(c *gocheck.C) {
	err := router.Store("myapp", "myapp", routerName)
	c.Assert(err, gocheck.IsNil)
	data := galebData{
		Name: "myapp",
		Reals: []galebRealData{
			{Real: "10.1.1.10", BackendId: s.server.URL + "/api/backend1"},
			{Real: "10.1.1.11", BackendId: s.server.URL + "/api/backend2"},
		},
	}
	err = data.save()
	c.Assert(err, gocheck.IsNil)
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	routes, err := gRouter.Routes("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(routes, gocheck.DeepEquals, []string{"10.1.1.10", "10.1.1.11"})
}

func (s *S) TestSwap(c *gocheck.C) {
	s.handler.RspCode = http.StatusNoContent
	s.handler.ConditionalContent = map[string]interface{}{
		"/api/backendpool/": []string{"201", `{"_links":{"self":"/pool1"}}`},
		"/api/rule/":        []string{"201", `{"_links":{"self":"/rule1"}}`},
		"/api/virtualhost/": []string{"201", `{"_links":{"self":"/vh1"}}`},
		"/api/backend/":     []string{"201", `{"_links":{"self":"/backendX"}}`},
	}
	backend1 := "b1"
	backend2 := "b2"
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.AddBackend(backend1)
	c.Assert(err, gocheck.IsNil)
	err = gRouter.AddRoute(backend1, "http://127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.AddBackend(backend2)
	c.Assert(err, gocheck.IsNil)
	err = gRouter.AddRoute(backend2, "http://10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = gRouter.Swap(backend1, backend2)
	c.Assert(err, gocheck.IsNil)
	data1, err := getGalebData(backend1)
	c.Assert(err, gocheck.IsNil)
	c.Assert(data1.Reals, gocheck.DeepEquals, []galebRealData{{Real: "10.10.10.10", BackendId: "/backendX"}})
	data2, err := getGalebData(backend2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(data2.Reals, gocheck.DeepEquals, []galebRealData{{Real: "127.0.0.1", BackendId: "/backendX"}})
}

func (s *S) TestAddr(c *gocheck.C) {
	err := router.Store("myapp", "myapp", routerName)
	c.Assert(err, gocheck.IsNil)
	data := galebData{
		Name: "myapp",
	}
	err = data.save()
	c.Assert(err, gocheck.IsNil)
	gRouter, err := createRouter("galeb")
	c.Assert(err, gocheck.IsNil)
	addr, err := gRouter.Addr("myapp")
	c.Assert(addr, gocheck.Equals, "myapp.galeb.com")
}

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	r, err := router.Get("galeb")
	c.Assert(err, gocheck.IsNil)
	_, ok := r.(*galebRouter)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestShouldBeRegisteredAllowingPrefixes(c *gocheck.C) {
	config.Set("routers:inst1:api-url", "url1")
	config.Set("routers:inst1:username", "username1")
	config.Set("routers:inst1:password", "pass1")
	config.Set("routers:inst1:domain", "domain1")
	config.Set("routers:inst2:api-url", "url2")
	config.Set("routers:inst2:username", "username2")
	config.Set("routers:inst2:password", "pass2")
	config.Set("routers:inst2:domain", "domain2")
	config.Set("routers:inst1:type", "galeb")
	config.Set("routers:inst2:type", "galeb")
	defer config.Unset("routers:inst1:type")
	defer config.Unset("routers:inst2:type")
	defer config.Unset("routers:inst1:api-url")
	defer config.Unset("routers:inst1:username")
	defer config.Unset("routers:inst1:password")
	defer config.Unset("routers:inst1:domain")
	defer config.Unset("routers:inst2:api-url")
	defer config.Unset("routers:inst2:username")
	defer config.Unset("routers:inst2:password")
	defer config.Unset("routers:inst2:domain")
	got1, err := router.Get("inst1")
	c.Assert(err, gocheck.IsNil)
	got2, err := router.Get("inst2")
	c.Assert(err, gocheck.IsNil)
	r1, ok := got1.(*galebRouter)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(r1.prefix, gocheck.Equals, "routers:inst1")
	c.Assert(r1.client.ApiUrl, gocheck.Equals, "url1")
	c.Assert(r1.client.Username, gocheck.Equals, "username1")
	c.Assert(r1.client.Password, gocheck.Equals, "pass1")
	c.Assert(r1.domain, gocheck.Equals, "domain1")
	r2, ok := got2.(*galebRouter)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(r2.prefix, gocheck.Equals, "routers:inst2")
	c.Assert(r2.client.ApiUrl, gocheck.Equals, "url2")
	c.Assert(r2.client.Username, gocheck.Equals, "username2")
	c.Assert(r2.client.Password, gocheck.Equals, "pass2")
	c.Assert(r2.domain, gocheck.Equals, "domain2")
}
