// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"

	"strconv"

	"net/url"

	"sort"

	"github.com/gorilla/mux"
	"github.com/tsuru/config"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/router"
	check "gopkg.in/check.v1"
)

type backend struct {
	addr         string
	addresses    []string
	cnames       []string
	certificates map[string]certData
	swapWith     string
	cnameOnly    bool
}

type fakeRouterAPI struct {
	listener net.Listener
	backends map[string]*backend
	endpoint string
	router   *mux.Router
}

func newFakeRouter(c *check.C) *fakeRouterAPI {
	api := &fakeRouterAPI{}
	r := mux.NewRouter()
	r.HandleFunc("/backend/{name}", api.getBackend).Methods(http.MethodGet)
	r.HandleFunc("/backend/{name}", api.addBackend).Methods(http.MethodPost)
	r.HandleFunc("/backend/{name}", api.removeBackend).Methods(http.MethodDelete)
	r.HandleFunc("/backend/{name}/routes", api.getRoutes).Methods(http.MethodGet)
	r.HandleFunc("/backend/{name}/routes", api.setRoutes).Methods(http.MethodPut)
	r.HandleFunc("/backend/{name}/swap", api.swap).Methods(http.MethodPost)
	r.HandleFunc("/backend/{name}/cname", api.getCnames).Methods(http.MethodGet)
	r.HandleFunc("/backend/{name}/cname/{cname}", api.setCname).Methods(http.MethodPost)
	r.HandleFunc("/backend/{name}/cname/{cname}", api.unsetCname).Methods(http.MethodDelete)
	r.HandleFunc("/backend/{name}/certificate/{cname}", api.getCertificate).Methods(http.MethodGet)
	r.HandleFunc("/backend/{name}/certificate/{cname}", api.addCertificate).Methods(http.MethodPost)
	r.HandleFunc("/backend/{name}/certificate/{cname}", api.removeCertificate).Methods(http.MethodDelete)
	listener, err := net.Listen("tcp", "")
	c.Assert(err, check.IsNil)
	api.listener = listener
	api.endpoint = fmt.Sprintf("http://%s", listener.Addr().String())
	api.router = r
	go http.Serve(listener, r)
	return api
}

func (f *fakeRouterAPI) getBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	if backend, ok := f.backends[name]; ok {
		resp := &backendResp{Address: backend.addr}
		json.NewEncoder(w).Encode(resp)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) addBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	if _, ok := f.backends[name]; !ok {
		f.backends[name] = &backend{}
		return
	}
	w.WriteHeader(http.StatusConflict)
}

func (f *fakeRouterAPI) removeBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	if backend, ok := f.backends[name]; ok {
		if backend.swapWith != "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(router.ErrBackendSwapped.Error()))
			return
		}
		delete(f.backends, name)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) getRoutes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	if backend, ok := f.backends[name]; ok {
		resp := &routesReq{}
		resp.Addresses = backend.addresses
		json.NewEncoder(w).Encode(resp)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) setRoutes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	if backend, ok := f.backends[name]; ok {
		req := &routesReq{}
		err := json.NewDecoder(r.Body).Decode(req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		backend.addresses = req.Addresses
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) swap(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	target := r.FormValue("target")
	cnameOnly := r.FormValue("cnameOnly")
	if backend, ok := f.backends[name]; ok {
		backend.swapWith = target
		backend.cnameOnly, _ = strconv.ParseBool(cnameOnly)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) setCname(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	cname := vars["cname"]
	if backend, ok := f.backends[name]; ok {
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
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) unsetCname(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	cname := vars["cname"]
	if backend, ok := f.backends[name]; ok {
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
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) getCnames(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	if backend, ok := f.backends[name]; ok {
		resp := cnamesResp{Cnames: backend.cnames}
		json.NewEncoder(w).Encode(&resp)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) getCertificate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	cname := vars["cname"]
	if backend, ok := f.backends[name]; ok {
		if cert, ok := backend.certificates[cname]; ok {
			json.NewEncoder(w).Encode(&cert.Certificate)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(router.ErrCertificateNotFound.Error()))
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) addCertificate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	cname := vars["cname"]
	cert := r.FormValue("certificate")
	key := r.FormValue("key")
	if backend, ok := f.backends[name]; ok {
		certData := backend.certificates[cname]
		certData.Certificate = cert
		certData.Key = key
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) removeCertificate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	cname := vars["cname"]
	if backend, ok := f.backends[name]; ok {
		if _, ok := backend.certificates[cname]; !ok {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(router.ErrCertificateNotFound.Error()))
			return
		}
		delete(backend.certificates, cname)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) stop() {
	f.listener.Close()
}

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
	s.testRouter = &apiRouter{
		endpoint:   s.apiRouter.endpoint,
		client:     tsuruNet.Dial5Full60ClientNoKeepAlive,
		routerName: "apirouter",
	}
	config.Set("routers:apirouter:endpoint", s.apiRouter.endpoint)
	s.apiRouter.backends = map[string]*backend{
		"mybackend": &backend{addr: "mybackend.cloud.com", addresses: []string{"http://127.0.0.1:32876", "http://127.0.0.1:32678"}},
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
