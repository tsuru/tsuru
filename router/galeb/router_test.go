// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package galeb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/router"
	ttesting "github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct {
	conn    *db.Storage
	server  *httptest.Server
	handler ttesting.MultiTestHandler
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("galeb:username", "myusername")
	config.Set("galeb:password", "mypassword")
	config.Set("galeb:domain", "galeb.com")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) SetUpTest(c *gocheck.C) {
	s.handler = ttesting.MultiTestHandler{}
	s.server = httptest.NewServer(&s.handler)
	config.Set("galeb:api-url", s.server.URL+"/api")
	ttesting.ClearAllCollections(s.conn.Collection("router_galeb_tests").Database)
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
	gRouter := galebRouter{}
	err := gRouter.AddBackend("myapp")
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
	gRouter := galebRouter{}
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
	gRouter := galebRouter{}
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
	gRouter := galebRouter{}
	err = gRouter.RemoveRoute("myapp", "10.1.1.10")
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
	gRouter := galebRouter{}
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
	gRouter := galebRouter{}
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
	gRouter := galebRouter{}
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
	gRouter := galebRouter{}
	err := gRouter.AddBackend(backend1)
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
	c.Assert(data1.Reals, gocheck.DeepEquals, []galebRealData{{Real: "http://10.10.10.10", BackendId: "/backendX"}})
	data2, err := getGalebData(backend2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(data2.Reals, gocheck.DeepEquals, []galebRealData{{Real: "http://127.0.0.1", BackendId: "/backendX"}})
}

func (s *S) TestAddr(c *gocheck.C) {
	err := router.Store("myapp", "myapp", routerName)
	c.Assert(err, gocheck.IsNil)
	data := galebData{
		Name: "myapp",
	}
	err = data.save()
	c.Assert(err, gocheck.IsNil)
	gRouter := galebRouter{}
	addr, err := gRouter.Addr("myapp")
	c.Assert(addr, gocheck.Equals, "myapp.galeb.com")
}
