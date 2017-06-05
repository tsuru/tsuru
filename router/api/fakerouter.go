// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/tsuru/tsuru/router"
	check "gopkg.in/check.v1"
)

type backend struct {
	addr        string
	addresses   []string
	cnames      []string
	swapWith    string
	cnameOnly   bool
	healthcheck router.HealthcheckData
}

type fakeRouterAPI struct {
	listener     net.Listener
	backends     map[string]*backend
	certificates map[string]certData
	endpoint     string
	router       *mux.Router
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
	r.HandleFunc("/backend/{name}/healthcheck", api.setHealthcheck).Methods(http.MethodPut)
	r.HandleFunc("/certificate/{cname}", api.getCertificate).Methods(http.MethodGet)
	r.HandleFunc("/certificate/{cname}", api.addCertificate).Methods(http.MethodPut)
	r.HandleFunc("/certificate/{cname}", api.removeCertificate).Methods(http.MethodDelete)
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
	cname := vars["cname"]
	if cert, ok := f.certificates[cname]; ok {
		json.NewEncoder(w).Encode(&cert.Certificate)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(router.ErrCertificateNotFound.Error()))
	return

}

func (f *fakeRouterAPI) addCertificate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cname := vars["cname"]
	var cert certData
	json.NewDecoder(r.Body).Decode(&cert)
	f.certificates[cname] = cert
	w.WriteHeader(http.StatusOK)
}

func (f *fakeRouterAPI) removeCertificate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cname := vars["cname"]
	if _, ok := f.certificates[cname]; !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(router.ErrCertificateNotFound.Error()))
		return
	}
	delete(f.certificates, cname)
}

func (f *fakeRouterAPI) setHealthcheck(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	if b, ok := f.backends[name]; ok {
		hc := router.HealthcheckData{}
		json.NewDecoder(r.Body).Decode(&hc)
		b.healthcheck = hc
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (f *fakeRouterAPI) stop() {
	f.listener.Close()
}
