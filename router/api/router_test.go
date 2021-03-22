// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	provTypes "github.com/tsuru/tsuru/types/provision"
	routerTypes "github.com/tsuru/tsuru/types/router"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct {
	apiRouter   *fakeRouterAPI
	testRouter  *apiRouter
	mockService *servicemock.MockService
}

var _ = check.Suite(&S{})

func init() {
	suite := &routertest.RouterSuite{}
	var r *fakeRouterAPI
	suite.SetUpTestFunc = func(c *check.C) {
		r = newFakeRouter(c)
		r.router.HandleFunc("/support/{feature}", func(http.ResponseWriter, *http.Request) {
		})
		config.Set("routers:apirouter:api-url", r.endpoint)
		config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
		config.Set("database:name", "router_api_tests")
		r.backends = make(map[string]*backend)
		apiRouter, err := createRouter("api", router.ConfigGetterFromPrefix("routers:apirouter"))
		c.Assert(err, check.IsNil)
		suite.Router = apiRouter
	}
	suite.TearDownTestFunc = func(c *check.C) {
		r.stop()
		config.Unset("routers:apirouter")
		conn, err := db.Conn()
		c.Assert(err, check.IsNil)
		defer conn.Close()
		dbtest.ClearAllCollections(conn.Collection("router_api_tests").Database)
	}
	check.Suite(suite)
}

func (s *S) SetUpTest(c *check.C) {
	s.apiRouter = newFakeRouter(c)
	s.apiRouter.certificates = make(map[string]certData)
	s.testRouter = &apiRouter{
		endpoint:   s.apiRouter.endpoint,
		client:     tsuruNet.Dial15Full60ClientNoKeepAlive,
		routerName: "apirouter",
	}
	s.testRouter.supIface = s.testRouter
	config.Set("routers:apirouter:api-url", s.apiRouter.endpoint)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "router_api_tests")
	s.apiRouter.backends = make(map[string]*backend)
	s.testRouter.AddBackend(context.TODO(), routertest.FakeApp{Name: "mybackend"})
	s.apiRouter.backends = map[string]*backend{
		"mybackend": {addr: "mybackend.cloud.com", addresses: []string{"http://127.0.0.1:32876", "http://127.0.0.1:32678"}},
	}
	s.mockService = &servicemock.MockService{}
	servicemock.SetMockService(s.mockService)
}

func (s *S) TearDownTest(c *check.C) {
	config.Unset("routers:apirouter")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Collection("router_api_tests").Database)
	s.apiRouter.stop()
}

func (s *S) TestAddr(c *check.C) {
	addr, err := s.testRouter.Addr(context.TODO(), routertest.FakeApp{Name: "mybackend"})
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.DeepEquals, "mybackend.cloud.com")
}

func (s *S) TestAddrNotFound(c *check.C) {
	addr, err := s.testRouter.Addr(context.TODO(), routertest.FakeApp{Name: "invalid"})
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
	c.Assert(addr, check.DeepEquals, "")
}

func (s *S) TestAddBackend(c *check.C) {
	err := s.testRouter.AddBackend(context.TODO(), routertest.FakeApp{Name: "new-backend"})
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["new-backend"], check.NotNil)
}

func (s *S) TestAddBackendOpts(c *check.C) {
	app := routertest.FakeApp{
		Name:      "new-backend",
		Pool:      "mypool",
		TeamOwner: "owner",
		Teams:     []string{"team1", "team2"},
	}
	err := s.testRouter.AddBackendOpts(context.TODO(), app, map[string]string{"opt1": "val1"})
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["new-backend"].opts, check.DeepEquals, map[string]interface{}{
		"opt1":                   "val1",
		"tsuru.io/app-pool":      "mypool",
		"tsuru.io/app-teamowner": "owner",
		"tsuru.io/app-teams":     []interface{}{"team1", "team2"},
	})
}

func (s *S) TestAddBackendOptsMultiCluster(c *check.C) {
	s.mockService.ResetCluster()
	s.mockService.Pool.OnFindByName = func(name string) (*provTypes.Pool, error) {
		return &provTypes.Pool{
			Name:        "my-pool",
			Provisioner: "kubernetes",
		}, nil
	}
	s.mockService.Cluster.OnFindByPool = func(provisioner, pool string) (*provTypes.Cluster, error) {
		return &provTypes.Cluster{
			Name:        "cluster-my-pool",
			Provisioner: "kubernetes",
			Addresses: []string{
				"https://kubernetes.example.com",
			},
		}, nil
	}
	s.apiRouter.interceptor = func(r *http.Request) {
		c.Assert(r.Header.Get("X-Tsuru-Pool-Name"), check.Equals, "my-pool")
		c.Assert(r.Header.Get("X-Tsuru-Pool-Provisioner"), check.Equals, "kubernetes")
		c.Assert(r.Header.Get("X-Tsuru-Cluster-Name"), check.Equals, "cluster-my-pool")
		c.Assert(r.Header.Get("X-Tsuru-Cluster-Provisioner"), check.Equals, "kubernetes")
		c.Assert(r.Header.Get("X-Tsuru-Cluster-Addresses"), check.Equals, "https://kubernetes.example.com")
	}
	defer func() {
		s.apiRouter.interceptor = nil
	}()

	s.testRouter.multiCluster = true
	app := routertest.FakeApp{
		Name:      "new-backend",
		Pool:      "my-pool",
		TeamOwner: "owner",
		Teams:     []string{"team1", "team2"},
	}
	err := s.testRouter.AddBackendOpts(context.TODO(), app, map[string]string{"opt1": "val1"})
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["new-backend"].opts, check.DeepEquals, map[string]interface{}{
		"opt1":                   "val1",
		"tsuru.io/app-pool":      "my-pool",
		"tsuru.io/app-teamowner": "owner",
		"tsuru.io/app-teams":     []interface{}{"team1", "team2"},
	})
}

func (s *S) TestUpdateBackendOpts(c *check.C) {
	app := routertest.FakeApp{
		Name:      "new-backend",
		Pool:      "pool",
		TeamOwner: "owner",
		Teams:     []string{"team1", "team2"},
	}
	err := s.testRouter.AddBackendOpts(context.TODO(), app, map[string]string{"opt1": "val1"})
	c.Assert(err, check.IsNil)
	app.Pool = "newpool"
	app.Teams = []string{"team1"}
	err = s.testRouter.UpdateBackendOpts(context.TODO(), app, map[string]string{"opt1": "val2"})
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["new-backend"].opts, check.DeepEquals, map[string]interface{}{
		"opt1":                   "val2",
		"tsuru.io/app-pool":      "newpool",
		"tsuru.io/app-teamowner": "owner",
		"tsuru.io/app-teams":     []interface{}{"team1"},
	})
}

func (s *S) TestAddBackendExists(c *check.C) {
	err := s.testRouter.AddBackend(context.TODO(), routertest.FakeApp{Name: "mybackend"})
	c.Assert(err, check.DeepEquals, router.ErrBackendExists)
}

func (s *S) TestRemoveBackend(c *check.C) {
	err := s.testRouter.RemoveBackend(context.TODO(), routertest.FakeApp{Name: "mybackend"})
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"], check.IsNil)
}

func (s *S) TestRemoveBackendNotFound(c *check.C) {
	err := s.testRouter.RemoveBackend(context.TODO(), routertest.FakeApp{Name: "invalid"})
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestRemoveBackendSwapped(c *check.C) {
	app1 := routertest.FakeApp{Name: "mybackend1"}
	app2 := routertest.FakeApp{Name: "mybackend2"}
	err := s.testRouter.AddBackend(context.TODO(), app1)
	c.Assert(err, check.IsNil)
	err = s.testRouter.AddBackend(context.TODO(), app2)
	c.Assert(err, check.IsNil)
	err = s.testRouter.Swap(context.TODO(), app1, app2, false)
	c.Assert(err, check.IsNil)
	err = s.testRouter.RemoveBackend(context.TODO(), app1)
	c.Assert(err, check.DeepEquals, router.ErrBackendSwapped)
}

func (s *S) TestAddRoutes(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:1234")
	c.Assert(err, check.IsNil)
	err = s.testRouter.AddRoutes(context.TODO(), routertest.FakeApp{Name: "mybackend"}, []*url.URL{addr})
	c.Assert(err, check.IsNil)
	sort.Strings(s.apiRouter.backends["mybackend"].addresses)
	c.Assert(s.apiRouter.backends["mybackend"].addresses, check.DeepEquals,
		[]string{"http://127.0.0.1:1234", "http://127.0.0.1:32678", "http://127.0.0.1:32876"})
}

func (s *S) TestAddRoutesBackendNotFound(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:1234")
	c.Assert(err, check.IsNil)
	err = s.testRouter.AddRoutes(context.TODO(), routertest.FakeApp{Name: "invalid"}, []*url.URL{addr})
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestRemoveRoute(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:32678")
	c.Assert(err, check.IsNil)
	err = s.testRouter.RemoveRoutes(context.TODO(), routertest.FakeApp{Name: "mybackend"}, []*url.URL{addr})
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"].addresses, check.DeepEquals,
		[]string{"http://127.0.0.1:32876"})
}

func (s *S) TestRemoveRoutes(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:32678")
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://127.0.0.1:32876")
	c.Assert(err, check.IsNil)
	err = s.testRouter.RemoveRoutes(context.TODO(), routertest.FakeApp{Name: "mybackend"}, []*url.URL{addr, addr2})
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"].addresses, check.DeepEquals, []string(nil))
}

func (s *S) TestRemoveRoutesBackendNotFound(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:1234")
	c.Assert(err, check.IsNil)
	err = s.testRouter.RemoveRoutes(context.TODO(), routertest.FakeApp{Name: "invalid"}, []*url.URL{addr})
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestGetRoutes(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:32876")
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://127.0.0.1:32678")
	c.Assert(err, check.IsNil)
	addrs, err := s.testRouter.Routes(context.TODO(), routertest.FakeApp{Name: "mybackend"})
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []*url.URL{addr, addr2})
}

func (s *S) TestGetRoutesBackendNotFound(c *check.C) {
	addrs, err := s.testRouter.Routes(context.TODO(), routertest.FakeApp{Name: "invalid"})
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
	c.Assert(len(addrs), check.Equals, 0)
}

func (s *S) TestSwap(c *check.C) {
	err := s.testRouter.AddBackend(context.TODO(), routertest.FakeApp{Name: "backend2"})
	c.Assert(err, check.IsNil)
	err = s.testRouter.Swap(context.TODO(), routertest.FakeApp{Name: "mybackend"}, routertest.FakeApp{Name: "backend2"}, true)
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"].cnameOnly, check.Equals, true)
	c.Assert(s.apiRouter.backends["mybackend"].swapWith, check.Equals, "backend2")
	err = s.testRouter.Swap(context.TODO(), routertest.FakeApp{Name: "mybackend"}, routertest.FakeApp{Name: "backend2"}, true)
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"].swapWith, check.Equals, "")
}

func (s *S) TestSwapMultiCluster(c *check.C) {
	s.testRouter.multiCluster = true
	defer func() {
		s.testRouter.multiCluster = false
	}()
	s.mockService.Pool.OnFindByName = func(name string) (*provTypes.Pool, error) {
		return &provTypes.Pool{
			Name:        name,
			Provisioner: "kubernetes",
		}, nil
	}
	s.mockService.Cluster.OnFindByPool = func(provisioner, pool string) (*provTypes.Cluster, error) {
		return &provTypes.Cluster{
			Name:        "cluster-" + pool,
			Provisioner: "kubernetes",
			Addresses: []string{
				"https://kubernetes.example.com",
			},
		}, nil
	}
	s.apiRouter.interceptor = func(r *http.Request) {
		if !strings.HasSuffix(r.URL.String(), "/swap") {
			return
		}
		c.Assert(r.Header.Get("X-Tsuru-Pool-Name"), check.Equals, "videos")
		c.Assert(r.Header.Get("X-Tsuru-Pool-Provisioner"), check.Equals, "kubernetes")
		c.Assert(r.Header.Get("X-Tsuru-Cluster-Name"), check.Equals, "cluster-videos")
		c.Assert(r.Header.Get("X-Tsuru-Cluster-Provisioner"), check.Equals, "kubernetes")
		c.Assert(r.Header.Get("X-Tsuru-Cluster-Addresses"), check.Equals, "https://kubernetes.example.com")
	}
	app1 := routertest.FakeApp{Name: "app1", Pool: "sports"}
	app2 := routertest.FakeApp{Name: "app2", Pool: "videos"}
	app3 := routertest.FakeApp{Name: "app3", Pool: "videos"}
	err := s.testRouter.AddBackend(context.TODO(), app1)
	c.Assert(err, check.IsNil)
	err = s.testRouter.AddBackend(context.TODO(), app2)
	c.Assert(err, check.IsNil)
	err = s.testRouter.AddBackend(context.TODO(), app3)
	c.Assert(err, check.IsNil)

	err = s.testRouter.Swap(context.TODO(), app1, app2, true)
	c.Assert(err, check.Equals, router.ErrSwapAmongDifferentClusters)

	err = s.testRouter.Swap(context.TODO(), app2, app3, true)
	c.Assert(err, check.IsNil)
	c.Check(s.apiRouter.backends["app2"].swapWith, check.Equals, "app3")
	c.Check(s.apiRouter.backends["app3"].swapWith, check.Equals, "app2")
}

func (s *S) TestSwapNotFound(c *check.C) {
	err := s.testRouter.Swap(context.TODO(), routertest.FakeApp{Name: "invalid"}, routertest.FakeApp{Name: "backend2"}, false)
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestSetCName(c *check.C) {
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.SetCName(context.TODO(), "cname.com", routertest.FakeApp{Name: "mybackend"})
	c.Assert(err, check.IsNil)
}

func (s *S) TestSetCNameBackendNotFound(c *check.C) {
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.SetCName(context.TODO(), "cname.com", routertest.FakeApp{Name: "invalid"})
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestSetCNameCNameAlreadyExists(c *check.C) {
	s.apiRouter.backends["mybackend"].cnames = []string{"cname.com"}
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.SetCName(context.TODO(), "cname.com", routertest.FakeApp{Name: "mybackend"})
	c.Assert(err, check.DeepEquals, router.ErrCNameExists)
}

func (s *S) TestUnsetCName(c *check.C) {
	s.apiRouter.backends["mybackend"].cnames = []string{"cname.com"}
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.UnsetCName(context.TODO(), "cname.com", routertest.FakeApp{Name: "mybackend"})
	c.Assert(err, check.IsNil)
}

func (s *S) TestUnsetCNameBackendNotFound(c *check.C) {
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.UnsetCName(context.TODO(), "cname.com", routertest.FakeApp{Name: "invalid"})
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestUnsetCNameCNameNotFound(c *check.C) {
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.UnsetCName(context.TODO(), "cname.com", routertest.FakeApp{Name: "mybackend"})
	c.Assert(err, check.DeepEquals, router.ErrCNameNotFound)
}

func (s *S) TestCNames(c *check.C) {
	s.apiRouter.backends["mybackend"].cnames = []string{"cname.com", "cname2.com"}
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	cnames, err := cnameRouter.CNames(context.TODO(), routertest.FakeApp{Name: "mybackend"})
	c.Assert(err, check.IsNil)
	c.Assert(len(cnames), check.Equals, 2)
}

func (s *S) TestCNamesBackendNotFound(c *check.C) {
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	cnames, err := cnameRouter.CNames(context.TODO(), routertest.FakeApp{Name: "invalid"})
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
	c.Assert(len(cnames), check.Equals, 0)
}

func (s *S) TestAddCertificate(c *check.C) {
	tlsRouter := &apiRouterWithTLSSupport{s.testRouter}
	err := tlsRouter.AddCertificate(context.TODO(), routertest.FakeApp{Name: "myapp"}, "cname.com", "cert", "key")
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.certificates["myapp/cname.com"], check.DeepEquals, certData{Certificate: "cert", Key: "key"})
}

func (s *S) TestRemoveCertificate(c *check.C) {
	tlsRouter := &apiRouterWithTLSSupport{s.testRouter}
	err := tlsRouter.AddCertificate(context.TODO(), routertest.FakeApp{Name: "myapp"}, "cname.com", "cert", "key")
	c.Assert(err, check.IsNil)
	err = tlsRouter.RemoveCertificate(context.TODO(), routertest.FakeApp{Name: "myapp"}, "cname.com")
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemoveCertificateNotFound(c *check.C) {
	tlsRouter := &apiRouterWithTLSSupport{s.testRouter}
	err := tlsRouter.RemoveCertificate(context.TODO(), routertest.FakeApp{Name: "myapp"}, "cname.com")
	c.Assert(err, check.DeepEquals, router.ErrCertificateNotFound)
}

func (s *S) TestGetCertificate(c *check.C) {
	tlsRouter := &apiRouterWithTLSSupport{s.testRouter}
	err := tlsRouter.AddCertificate(context.TODO(), routertest.FakeApp{Name: "myapp"}, "cname.com", "cert", "key")
	c.Assert(err, check.IsNil)
	cert, err := tlsRouter.GetCertificate(context.TODO(), routertest.FakeApp{Name: "myapp"}, "cname.com")
	c.Assert(err, check.IsNil)
	c.Assert(cert, check.DeepEquals, "cert")
}

func (s *S) TestGetCertificateNotFound(c *check.C) {
	tlsRouter := &apiRouterWithTLSSupport{s.testRouter}
	cert, err := tlsRouter.GetCertificate(context.TODO(), routertest.FakeApp{Name: "myapp"}, "cname.com")
	c.Assert(err, check.DeepEquals, router.ErrCertificateNotFound)
	c.Assert(cert, check.DeepEquals, "")
}

func (s *S) TestSetHealthcheck(c *check.C) {
	hcRouter := &apiRouterWithHealthcheckSupport{s.testRouter}
	hc := routerTypes.HealthcheckData{Path: "/", Status: 200}
	err := hcRouter.SetHealthcheck(context.TODO(), routertest.FakeApp{Name: "mybackend"}, hc)
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"].healthcheck, check.DeepEquals, hc)
}

func (s *S) TestHealcheckBackendNotFound(c *check.C) {
	hcRouter := &apiRouterWithHealthcheckSupport{s.testRouter}
	hc := routerTypes.HealthcheckData{Path: "/", Status: 200}
	err := hcRouter.SetHealthcheck(context.TODO(), routertest.FakeApp{Name: "invalid"}, hc)
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

// Router V2 exclusive APIs
func (s *S) TestEnsureBackend(c *check.C) {
	routerV2 := &apiRouterV2{s.testRouter}
	app := routertest.FakeApp{Name: "myapp", Pool: "mypool", Teams: []string{"team01", "team02"}, TeamOwner: "team03"}
	err := routerV2.EnsureBackend(context.TODO(), app, router.EnsureBackendOpts{
		Opts: map[string]interface{}{
			"myinfo.io/test": "test",
		},
		Prefixes: []router.BackendPrefix{
			{
				Prefix: "",
				Target: map[string]string{
					// for kubernetes provisioner example
					"service":   "myapp-web",
					"namespace": "tsuru-myapp",
				},
			},
			{
				Prefix: "subscriber",
				Target: map[string]string{
					// for kubernetes provisioner example
					"service":   "myapp-subscriber",
					"namespace": "tsuru-myapp",
				},
			},
		},
	})
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["myapp"].opts, check.DeepEquals, map[string]interface{}{
		"myinfo.io/test":         "test",
		"tsuru.io/app-pool":      "mypool",
		"tsuru.io/app-teamowner": "team03",
		"tsuru.io/app-teams":     []interface{}{"team01", "team02"},
	})
	c.Assert(s.apiRouter.backends["myapp"].prefixAddrs, check.DeepEquals, map[string]routesReq{
		"": {
			Prefix:    "",
			ExtraData: map[string]string{"namespace": "tsuru-myapp", "service": "myapp-web"},
		},
		"subscriber": {
			Prefix:    "subscriber",
			ExtraData: map[string]string{"namespace": "tsuru-myapp", "service": "myapp-subscriber"},
		},
	})
}

func (s *S) TestCreateRouterSupport(c *check.C) {
	tt := []struct {
		features    map[string]bool
		expectCname bool
		expectTLS   bool
		expectHC    bool
	}{
		{nil, false, false, false},
		{features: map[string]bool{"cname": true}, expectCname: true},
		{features: map[string]bool{"tls": true}, expectTLS: true},
		{features: map[string]bool{"healthcheck": true}, expectHC: true},
		{features: map[string]bool{"cname": true, "tls": true}, expectCname: true, expectTLS: true},
		{features: map[string]bool{"cname": true, "tls": true, "healthcheck": true}, expectCname: true, expectTLS: true, expectHC: true},
		{features: map[string]bool{"cname": true, "healthcheck": true}, expectCname: true, expectHC: true},
		{features: map[string]bool{"tls": true, "healthcheck": true}, expectTLS: true, expectHC: true},
	}
	var i int
	s.apiRouter.router.HandleFunc("/support/{name}", func(w http.ResponseWriter, r *http.Request) {
		f := tt[i]
		v := mux.Vars(r)
		if f.features == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if b := f.features[v["name"]]; b {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	for i = range tt {
		comment := check.Commentf("case %d: %v", i, tt[i])
		r, err := createRouter("myrouter", router.ConfigGetterFromPrefix("routers:apirouter"))
		c.Assert(err, check.IsNil, comment)
		_, ok := r.(router.CNameRouter)
		c.Assert(ok, check.Equals, tt[i].expectCname, comment)
		_, ok = r.(router.TLSRouter)
		c.Assert(ok, check.Equals, tt[i].expectTLS, comment)
		_, ok = r.(router.CustomHealthcheckRouter)
		c.Assert(ok, check.Equals, tt[i].expectHC, comment)
	}
}

func (s *S) TestCreateCustomHeaders(c *check.C) {
	s.apiRouter.router.HandleFunc("/custom", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-CUSTOM") != "HI" || r.Header.Get("X-CUSTOM-ENV") != "XYZ" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	})
	os.Setenv("ROUTER_ENV_HEADER_OPT", "XYZ")
	config.Set("routers:apirouter:headers", map[interface{}]interface{}{"X-CUSTOM": "HI", "X-CUSTOM-ENV": "$ROUTER_ENV_HEADER_OPT"})
	defer config.Unset("router:apirouter:headers")
	defer os.Unsetenv("ROUTER_ENV_HEADER_OPT")
	r, err := createRouter("apirouter", router.ConfigGetterFromPrefix("routers:apirouter"))
	c.Assert(err, check.IsNil)
	_, code, err := r.(*struct {
		router.Router
		router.OptsRouter
	}).Router.(*apiRouter).do(context.TODO(), http.MethodGet, "/custom", nil, nil)
	c.Assert(code, check.DeepEquals, http.StatusOK)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateDuplicatedCustomHeaders(c *check.C) {
	s.apiRouter.router.HandleFunc("/custom", func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("X-CUSTOM-ENV")
		sort.Strings(values)

		if !c.Check(values, check.DeepEquals, []string{"ABC", "XYZ"}) {
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	os.Setenv("ROUTER_ENV_HEADER_OPT", "XYZ")
	config.Set("routers:apirouter:headers", map[interface{}]interface{}{
		"X-CUSTOM-ENV": []interface{}{
			"$ROUTER_ENV_HEADER_OPT",
			"ABC",
		},
	})
	defer config.Unset("router:apirouter:headers")
	defer os.Unsetenv("ROUTER_ENV_HEADER_OPT")
	r, err := createRouter("apirouter", router.ConfigGetterFromPrefix("routers:apirouter"))
	c.Assert(err, check.IsNil)
	_, code, err := r.(*struct {
		router.Router
		router.OptsRouter
	}).Router.(*apiRouter).do(context.TODO(), http.MethodGet, "/custom", nil, nil)
	c.Assert(code, check.DeepEquals, http.StatusOK)
	c.Assert(err, check.IsNil)
}

func newFakeRouter(c *check.C) *fakeRouterAPI {
	api := &fakeRouterAPI{}
	r := mux.NewRouter()
	r.HandleFunc("/backend/{name}", api.getBackend).Methods(http.MethodGet)
	r.HandleFunc("/backend/{name}", api.addBackend).Methods(http.MethodPost)
	r.HandleFunc("/backend/{name}", api.updateBackend).Methods(http.MethodPut)
	r.HandleFunc("/backend/{name}", api.removeBackend).Methods(http.MethodDelete)
	r.HandleFunc("/backend/{name}/routes", api.getRoutes).Methods(http.MethodGet)
	r.HandleFunc("/backend/{name}/routes", api.addRoutes).Methods(http.MethodPost)
	r.HandleFunc("/backend/{name}/routes/remove", api.removeRoutes).Methods(http.MethodPost)
	r.HandleFunc("/backend/{name}/swap", api.swap).Methods(http.MethodPost)
	r.HandleFunc("/backend/{name}/cname", api.getCnames).Methods(http.MethodGet)
	r.HandleFunc("/backend/{name}/cname/{cname}", api.setCname).Methods(http.MethodPost)
	r.HandleFunc("/backend/{name}/cname/{cname}", api.unsetCname).Methods(http.MethodDelete)
	r.HandleFunc("/backend/{name}/healthcheck", api.setHealthcheck).Methods(http.MethodPut)
	r.HandleFunc("/backend/{name}/certificate/{cname}", api.getCertificate).Methods(http.MethodGet)
	r.HandleFunc("/backend/{name}/certificate/{cname}", api.addCertificate).Methods(http.MethodPut)
	r.HandleFunc("/backend/{name}/certificate/{cname}", api.removeCertificate).Methods(http.MethodDelete)
	r.HandleFunc("/backend/{name}/status", api.getStatusBackend).Methods(http.MethodGet)
	r.HandleFunc("/info", api.getInfo).Methods(http.MethodGet)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, check.IsNil)
	api.listener = listener
	api.endpoint = fmt.Sprintf("http://%s", listener.Addr().String())
	api.router = r
	go http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if api.interceptor != nil {
			api.interceptor(req)
		}
		r.ServeHTTP(w, req)
	}))
	return api
}

type backend struct {
	addr        string
	addresses   []string
	cnames      []string
	swapWith    string
	cnameOnly   bool
	healthcheck routerTypes.HealthcheckData
	opts        map[string]interface{}
	prefixAddrs map[string]routesReq
}

type fakeRouterAPI struct {
	listener     net.Listener
	backends     map[string]*backend
	certificates map[string]certData
	endpoint     string
	router       *mux.Router
	interceptor  func(r *http.Request)
}

func (f *fakeRouterAPI) getInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"just": "proper"}`))
}

func (f *fakeRouterAPI) getStatusBackend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "ready", "detail": "anaander"}`))
}

func (f *fakeRouterAPI) getBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	backend, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	allAddrs := []string{backend.addr}
	for prefix := range backend.prefixAddrs {
		if prefix == "" {
			continue
		}
		allAddrs = append(allAddrs, prefix+"."+name+".apirouter.com")
	}
	resp := &backendResp{Address: backend.addr, Addresses: allAddrs}
	json.NewEncoder(w).Encode(resp)
}

func (f *fakeRouterAPI) addBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	_, ok := f.backends[name]
	if ok {
		w.WriteHeader(http.StatusConflict)
		return
	}
	var req map[string]interface{}
	json.NewDecoder(r.Body).Decode(&req)
	f.backends[name] = &backend{opts: req, addr: name + ".apirouter.com"}
}

func (f *fakeRouterAPI) ensureBackendV2(w http.ResponseWriter, r *http.Request) {
	o := &router.EnsureBackendOpts{}
	err := json.NewDecoder(r.Body).Decode(o)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vars := mux.Vars(r)
	name := vars["name"]

	f.backends[name] = &backend{
		opts:        o.Opts,
		prefixAddrs: map[string]routesReq{},
		addr:        name + ".apirouter.com",
	}

	for _, prefix := range o.Prefixes {
		f.backends[name].prefixAddrs[prefix.Prefix] = routesReq{
			Prefix:    prefix.Prefix,
			ExtraData: prefix.Target,
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeRouterAPI) updateBackend(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	contentTypeParts := strings.Split(contentType, ";")

	if len(contentTypeParts) > 1 {
		if strings.TrimSpace(contentTypeParts[1]) == "router=v2" {
			f.ensureBackendV2(w, r)
			return
		}
	}

	vars := mux.Vars(r)
	name := vars["name"]
	backend, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var req map[string]interface{}
	json.NewDecoder(r.Body).Decode(&req)
	backend.opts = req
}

func (f *fakeRouterAPI) removeBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	backend, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if backend.swapWith != "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(router.ErrBackendSwapped.Error()))
		return
	}
	delete(f.backends, name)
}

func (f *fakeRouterAPI) getRoutes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	backend, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	resp := &routesPrefixReq{}
	resp.Addresses = backend.addresses
	for _, prefixData := range backend.prefixAddrs {
		resp.AddressesWithPrefix = append(resp.AddressesWithPrefix, prefixData)
	}
	json.NewEncoder(w).Encode(resp)
}

func (f *fakeRouterAPI) addRoutes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	backend, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	req := &routesReq{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	rMap := make(map[string]struct{})
	addressToKey := func(a string) string {
		u, _ := url.Parse(a)
		return u.Host + ":" + u.Port()
	}
	if backend.prefixAddrs == nil {
		backend.prefixAddrs = make(map[string]routesReq)
	}
	var prefixData *routesReq
	for prefixName, item := range backend.prefixAddrs {
		if req.Prefix == prefixName {
			prefixData = &item
			break
		}
	}
	if prefixData == nil {
		prefixData = &routesReq{Prefix: req.Prefix}
		if req.Prefix == "" {
			prefixData.Addresses = backend.addresses
		}
	}
	if req.ExtraData != nil {
		prefixData.ExtraData = req.ExtraData
	}
	for _, a := range prefixData.Addresses {
		rMap[addressToKey(a)] = struct{}{}
	}
	for i, a := range req.Addresses {
		if _, ok := rMap[addressToKey(a)]; ok {
			continue
		}
		rMap[addressToKey(a)] = struct{}{}
		prefixData.Addresses = append(prefixData.Addresses, req.Addresses[i])
		if req.Prefix == "" {
			backend.addresses = append(backend.addresses, req.Addresses[i])
		}
	}
	backend.prefixAddrs[req.Prefix] = *prefixData
}

func (f *fakeRouterAPI) removeRoutes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	backend, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	req := &routesReq{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	addressToKey := func(a string) string {
		u, _ := url.Parse(a)
		return u.Host + ":" + u.Port()
	}

	if backend.prefixAddrs == nil {
		backend.prefixAddrs = make(map[string]routesReq)
	}
	var prefixData *routesReq
	for prefixName, item := range backend.prefixAddrs {
		if req.Prefix == prefixName {
			prefixData = &item
			break
		}
	}
	if prefixData == nil {
		prefixData = &routesReq{Prefix: req.Prefix}
		if req.Prefix == "" {
			prefixData.Addresses = backend.addresses
		}
	}

	if req.ExtraData != nil {
		prefixData.ExtraData = req.ExtraData
	}
	addrMap := make(map[string]string)
	for _, b := range prefixData.Addresses {
		addrMap[addressToKey(b)] = b
	}
	for _, b := range req.Addresses {
		delete(addrMap, addressToKey(b))
	}
	prefixData.Addresses = nil
	if req.Prefix == "" {
		backend.addresses = nil
	}
	for _, b := range addrMap {
		prefixData.Addresses = append(prefixData.Addresses, b)
		if req.Prefix == "" {
			backend.addresses = append(backend.addresses, b)
		}
	}
	backend.prefixAddrs[req.Prefix] = *prefixData
}

func (f *fakeRouterAPI) swap(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	req := swapReq{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	backend, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	target, ok := f.backends[req.Target]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if backend.swapWith == req.Target {
		backend.swapWith = ""
		target.swapWith = ""
	} else {
		backend.swapWith = req.Target
		target.swapWith = name
	}
	backend.cnameOnly = req.CnameOnly
	target.cnameOnly = backend.cnameOnly
}

func (f *fakeRouterAPI) setCname(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	cname := vars["cname"]
	backend, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if strings.HasSuffix(cname, fmt.Sprintf(".%s.apirouter.com", name)) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var hasCname bool
	for _, c := range backend.cnames {
		if c == cname {
			hasCname = true
			break
		}
	}
	if hasCname {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(router.ErrCNameExists.Error()))
		return
	}
	backend.cnames = append(backend.cnames, cname)
}

func (f *fakeRouterAPI) unsetCname(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	cname := vars["cname"]
	backend, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var newCnames []string
	var found bool
	for _, c := range backend.cnames {
		if c == cname {
			found = true
			continue
		}
		newCnames = append(newCnames, c)
	}
	if !found {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(router.ErrCNameNotFound.Error()))
		return
	}
	backend.cnames = newCnames
}

func (f *fakeRouterAPI) getCnames(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	backend, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	resp := cnamesResp{Cnames: backend.cnames}
	json.NewEncoder(w).Encode(&resp)
}

func (f *fakeRouterAPI) getCertificate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cname := vars["cname"]
	name := vars["name"]
	cert, ok := f.certificates[name+"/"+cname]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(router.ErrCertificateNotFound.Error()))
		return
	}
	json.NewEncoder(w).Encode(&cert)
}

func (f *fakeRouterAPI) addCertificate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cname := vars["cname"]
	name := vars["name"]
	var cert certData
	json.NewDecoder(r.Body).Decode(&cert)
	f.certificates[name+"/"+cname] = cert
	w.WriteHeader(http.StatusOK)
}

func (f *fakeRouterAPI) removeCertificate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cname := vars["cname"]
	name := vars["name"]
	if _, ok := f.certificates[name+"/"+cname]; !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(router.ErrCertificateNotFound.Error()))
		return
	}
	delete(f.certificates, cname)
}

func (f *fakeRouterAPI) setHealthcheck(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	b, ok := f.backends[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	hc := routerTypes.HealthcheckData{}
	json.NewDecoder(r.Body).Decode(&hc)
	b.healthcheck = hc
}

func (f *fakeRouterAPI) stop() {
	f.listener.Close()
}
