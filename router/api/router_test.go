// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"testing"

	"net/url"

	"sort"

	"github.com/gorilla/mux"
	"github.com/tsuru/config"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/router"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct {
	apiRouter  *fakeRouterAPI
	testRouter *apiRouter
}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	s.apiRouter = newFakeRouter(c)
	s.apiRouter.certificates = make(map[string]certData)
	s.testRouter = &apiRouter{
		endpoint:   s.apiRouter.endpoint,
		client:     tsuruNet.Dial5Full60ClientNoKeepAlive,
		routerName: "apirouter",
	}
	config.Set("routers:apirouter:api-url", s.apiRouter.endpoint)
	s.apiRouter.backends = map[string]*backend{
		"mybackend": {addr: "mybackend.cloud.com", addresses: []string{"http://127.0.0.1:32876", "http://127.0.0.1:32678"}},
	}
}

func (s *S) TearDownTest(c *check.C) {
	config.Unset("routers:apirouter")
	s.apiRouter.stop()
}

func (s *S) TestAddr(c *check.C) {
	addr, err := s.testRouter.Addr("mybackend")
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.DeepEquals, "mybackend.cloud.com")
}

func (s *S) TestAddrNotFound(c *check.C) {
	addr, err := s.testRouter.Addr("invalid")
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
	c.Assert(addr, check.DeepEquals, "")
}

func (s *S) TestAddBackend(c *check.C) {
	err := s.testRouter.AddBackend("new-backend")
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["new-backend"], check.NotNil)
}

func (s *S) TestAddBackendExists(c *check.C) {
	err := s.testRouter.AddBackend("mybackend")
	c.Assert(err, check.DeepEquals, router.ErrBackendExists)
}

func (s *S) TestRemoveBackend(c *check.C) {
	err := s.testRouter.RemoveBackend("mybackend")
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"], check.IsNil)
}

func (s *S) TestRemoveBackendNotFound(c *check.C) {
	err := s.testRouter.RemoveBackend("invalid")
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestRemoveBackendSwapped(c *check.C) {
	err := s.testRouter.Swap("mybackend", "backend2", false)
	c.Assert(err, check.IsNil)
	err = s.testRouter.RemoveBackend("mybackend")
	c.Assert(err, check.DeepEquals, router.ErrBackendSwapped)
}

func (s *S) TestAddRoute(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:1234")
	c.Assert(err, check.IsNil)
	err = s.testRouter.AddRoute("mybackend", addr)
	c.Assert(err, check.IsNil)
	sort.Strings(s.apiRouter.backends["mybackend"].addresses)
	c.Assert(s.apiRouter.backends["mybackend"].addresses, check.DeepEquals,
		[]string{"http://127.0.0.1:1234", "http://127.0.0.1:32678", "http://127.0.0.1:32876"})
}

func (s *S) TestAddRoutes(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:1234")
	c.Assert(err, check.IsNil)
	err = s.testRouter.AddRoutes("mybackend", []*url.URL{addr})
	c.Assert(err, check.IsNil)
	sort.Strings(s.apiRouter.backends["mybackend"].addresses)
	c.Assert(s.apiRouter.backends["mybackend"].addresses, check.DeepEquals,
		[]string{"http://127.0.0.1:1234", "http://127.0.0.1:32678", "http://127.0.0.1:32876"})
}

func (s *S) TestAddRoutesBackendNotFound(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:1234")
	c.Assert(err, check.IsNil)
	err = s.testRouter.AddRoutes("invalid", []*url.URL{addr})
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestRemoveRoute(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:32678")
	c.Assert(err, check.IsNil)
	err = s.testRouter.RemoveRoutes("mybackend", []*url.URL{addr})
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"].addresses, check.DeepEquals,
		[]string{"http://127.0.0.1:32876"})
}

func (s *S) TestRemoveRoutes(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:32678")
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://127.0.0.1:32876")
	c.Assert(err, check.IsNil)
	err = s.testRouter.RemoveRoutes("mybackend", []*url.URL{addr, addr2})
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"].addresses, check.DeepEquals, []string(nil))
}

func (s *S) TestRemoveRoutesBackendNotFound(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:1234")
	c.Assert(err, check.IsNil)
	err = s.testRouter.RemoveRoutes("invalid", []*url.URL{addr})
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestGetRoutes(c *check.C) {
	addr, err := url.Parse("http://127.0.0.1:32876")
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://127.0.0.1:32678")
	c.Assert(err, check.IsNil)
	addrs, err := s.testRouter.Routes("mybackend")
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []*url.URL{addr, addr2})
}

func (s *S) TestGetRoutesBackendNotFound(c *check.C) {
	addrs, err := s.testRouter.Routes("invalid")
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
	c.Assert(len(addrs), check.Equals, 0)
}

func (s *S) TestSwap(c *check.C) {
	err := s.testRouter.Swap("mybackend", "backend2", false)
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"].cnameOnly, check.Equals, false)
	c.Assert(s.apiRouter.backends["mybackend"].swapWith, check.Equals, "backend2")
	err = s.testRouter.Swap("mybackend", "backend2", true)
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"].cnameOnly, check.Equals, true)
	c.Assert(s.apiRouter.backends["mybackend"].swapWith, check.Equals, "backend2")
}

func (s *S) TestSwapNotFound(c *check.C) {
	err := s.testRouter.Swap("invalid", "backend2", false)
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestSetCName(c *check.C) {
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.SetCName("cname.com", "mybackend")
	c.Assert(err, check.IsNil)
}

func (s *S) TestSetCNameBackendNotFound(c *check.C) {
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.SetCName("cname.com", "invalid")
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestSetCNameCNameAlreadyExists(c *check.C) {
	s.apiRouter.backends["mybackend"].cnames = []string{"cname.com"}
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.SetCName("cname.com", "mybackend")
	c.Assert(err, check.DeepEquals, router.ErrCNameExists)
}

func (s *S) TestUnsetCName(c *check.C) {
	s.apiRouter.backends["mybackend"].cnames = []string{"cname.com"}
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.UnsetCName("cname.com", "mybackend")
	c.Assert(err, check.IsNil)
}

func (s *S) TestUnsetCNameBackendNotFound(c *check.C) {
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.UnsetCName("cname.com", "invalid")
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
}

func (s *S) TestUnsetCNameCNameNotFound(c *check.C) {
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	err := cnameRouter.UnsetCName("cname.com", "mybackend")
	c.Assert(err, check.DeepEquals, router.ErrCNameNotFound)
}

func (s *S) TestCNames(c *check.C) {
	s.apiRouter.backends["mybackend"].cnames = []string{"cname.com", "cname2.com"}
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	cnames, err := cnameRouter.CNames("mybackend")
	c.Assert(err, check.IsNil)
	c.Assert(len(cnames), check.Equals, 2)
}

func (s *S) TestCNamesBackendNotFound(c *check.C) {
	cnameRouter := &apiRouterWithCnameSupport{s.testRouter}
	cnames, err := cnameRouter.CNames("invalid")
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
	c.Assert(len(cnames), check.Equals, 0)
}

func (s *S) TestAddCertificate(c *check.C) {
	tlsRouter := &apiRouterWithTLSSupport{s.testRouter}
	err := tlsRouter.AddCertificate("cname.com", "cert", "key")
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.certificates["cname.com"], check.DeepEquals, certData{Certificate: "cert", Key: "key"})
}

func (s *S) TestRemoveCertificate(c *check.C) {
	tlsRouter := &apiRouterWithTLSSupport{s.testRouter}
	err := tlsRouter.AddCertificate("cname.com", "cert", "key")
	c.Assert(err, check.IsNil)
	err = tlsRouter.RemoveCertificate("cname.com")
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemoveCertificateNotFound(c *check.C) {
	tlsRouter := &apiRouterWithTLSSupport{s.testRouter}
	err := tlsRouter.RemoveCertificate("cname.com")
	c.Assert(err, check.DeepEquals, router.ErrCertificateNotFound)
}

func (s *S) TestGetCertificate(c *check.C) {
	tlsRouter := &apiRouterWithTLSSupport{s.testRouter}
	err := tlsRouter.AddCertificate("cname.com", "cert", "key")
	c.Assert(err, check.IsNil)
	cert, err := tlsRouter.GetCertificate("cname.com")
	c.Assert(err, check.IsNil)
	c.Assert(cert, check.DeepEquals, "cert")
}

func (s *S) TestGetCertificateNotFound(c *check.C) {
	tlsRouter := &apiRouterWithTLSSupport{s.testRouter}
	cert, err := tlsRouter.GetCertificate("cname.com")
	c.Assert(err, check.DeepEquals, router.ErrCertificateNotFound)
	c.Assert(cert, check.DeepEquals, "")
}

func (s *S) TestSetHealthcheck(c *check.C) {
	hcRouter := &apiRouterWithHealthcheckSupport{s.testRouter}
	hc := router.HealthcheckData{Path: "/", Status: 200}
	err := hcRouter.SetHealthcheck("mybackend", hc)
	c.Assert(err, check.IsNil)
	c.Assert(s.apiRouter.backends["mybackend"].healthcheck, check.DeepEquals, hc)
}

func (s *S) TestHealcheckBackendNotFound(c *check.C) {
	hcRouter := &apiRouterWithHealthcheckSupport{s.testRouter}
	hc := router.HealthcheckData{Path: "/", Status: 200}
	err := hcRouter.SetHealthcheck("invalid", hc)
	c.Assert(err, check.DeepEquals, router.ErrBackendNotFound)
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
		if b := f.features[v["name"]]; b == true {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	for i = range tt {
		comment := check.Commentf("case %d: %v", i, tt[i])
		r, err := createRouter("myrouter", "routers:apirouter")
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
		if r.Header.Get("X-CUSTOM") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	})
	config.Set("routers:apirouter:headers", map[string]string{"X-CUSTOM": "HI"})
	defer config.Unset("router:apirouter:headers")
	r, err := createRouter("apirouter", "routers:apirouter")
	c.Assert(err, check.IsNil)
	_, code, err := r.(*apiRouter).do(http.MethodGet, "/custom", nil)
	c.Assert(code, check.DeepEquals, http.StatusOK)
	c.Assert(err, check.IsNil)
}
