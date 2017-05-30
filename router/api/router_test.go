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
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/router"
	check "gopkg.in/check.v1"
)

type backend struct {
	addr      string
	addresses []string
	swapWith  string
	cnameOnly bool
}

type fakeRouterAPI struct {
	listener net.Listener
	backends map[string]*backend
	endpoint string
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
	listener, err := net.Listen("tcp", "")
	c.Assert(err, check.IsNil)
	api.listener = listener
	api.endpoint = fmt.Sprintf("http://%s", listener.Addr().String())
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
			w.WriteHeader(http.StatusForbidden)
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
	r.ParseForm()
	target := r.FormValue("target")
	cnameOnly := r.FormValue("cnameOnly")
	if backend, ok := f.backends[name]; ok {
		backend.swapWith = target
		backend.cnameOnly, _ = strconv.ParseBool(cnameOnly)
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

func (s *S) SetUpSuite(c *check.C) {
	s.apiRouter = newFakeRouter(c)
	s.testRouter = &apiRouter{
		endpoint:   s.apiRouter.endpoint,
		client:     tsuruNet.Dial5Full60ClientNoKeepAlive,
		routerName: "apirouter",
	}
}

func (s *S) SetUpTest(c *check.C) {
	s.apiRouter.backends = map[string]*backend{
		"mybackend": &backend{addr: "mybackend.cloud.com", addresses: []string{"http://127.0.0.1:32876", "http://127.0.0.1:32678"}},
	}
}

func (s *S) TearDownSuite(c *check.C) {
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
