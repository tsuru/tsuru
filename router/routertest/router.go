// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routertest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/router"
	routerTypes "github.com/tsuru/tsuru/types/router"
)

var FakeRouter = newFakeRouter()

var TLSRouter = tlsRouter{
	fakeRouter: newFakeRouter(),
	Certs:      make(map[string]string),
	Keys:       make(map[string]string),
}

var ErrForcedFailure = errors.New("Forced failure")

func init() {
	router.Register("fake", createRouter)
	router.Register("fake-tls", createTLSRouter)
}

func createRouter(name string, config router.ConfigGetter) (router.Router, error) {
	return &FakeRouter, nil
}

func createTLSRouter(name string, config router.ConfigGetter) (router.Router, error) {
	return &TLSRouter, nil
}

func newFakeRouter() fakeRouter {
	return fakeRouter{
		cnames:      make(map[string]string),
		certIssuers: make(map[string]string),
		BackendOpts: make(map[string]router.EnsureBackendOpts),
		Info:        make(map[string]string),
		Status: router.RouterBackendStatus{
			Status: router.BackendStatusReady,
		},
		mutex:          &sync.Mutex{},
		FailuresByHost: make(map[string]bool),
	}
}

type fakeRouter struct {
	FailuresByHost map[string]bool
	BackendOpts    map[string]router.EnsureBackendOpts
	cnames         map[string]string
	certIssuers    map[string]string
	Info           map[string]string
	Status         router.RouterBackendStatus
	mutex          *sync.Mutex
}

var (
	_ router.Router = &fakeRouter{}
)

func (r *fakeRouter) GetName() string {
	return "fake"
}

func (r *fakeRouter) GetType() string {
	return "fake"
}

func (r *fakeRouter) GetHealthcheck(name string) routerTypes.HealthcheckData {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.BackendOpts[name].Healthcheck
}

func (r *fakeRouter) HasBackend(name string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	_, ok := r.BackendOpts[name]
	return ok
}

func (r *fakeRouter) HasCName(name string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	_, ok := r.cnames[name]
	return ok
}

func (r *fakeRouter) HasCNameFor(name, cname string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	stored, ok := r.cnames[cname]
	return ok && stored == name
}

func (r *fakeRouter) HasCertIssuerForCName(name, cname, issuer string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	stored, ok := r.certIssuers[cname+":"+issuer]
	return ok && stored == name
}

func (r *fakeRouter) RemoveBackend(ctx context.Context, app router.App) error {
	backendName := app.GetName()

	if !r.HasBackend(backendName) {
		return router.ErrBackendNotFound
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for cname, backend := range r.cnames {
		if backend == backendName {
			delete(r.cnames, cname)
		}
	}
	delete(r.BackendOpts, backendName)
	return nil
}

func (r *fakeRouter) GetInfo(ctx context.Context) (map[string]string, error) {
	return r.Info, nil
}

func (r *fakeRouter) GetBackendStatus(ctx context.Context, app router.App) (router.RouterBackendStatus, error) {
	backendName := app.GetName()

	if r.FailuresByHost[r.GetName()+":"+backendName] || r.FailuresByHost[backendName] {
		return router.RouterBackendStatus{
			Status: router.BackendStatusNotReady,
			Detail: "Forced failure",
		}, ErrForcedFailure
	}
	return r.Status, nil
}

func (r *fakeRouter) Reset() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.BackendOpts = make(map[string]router.EnsureBackendOpts)
	r.cnames = make(map[string]string)
	r.Info = make(map[string]string)
	r.Status = router.RouterBackendStatus{
		Status: router.BackendStatusReady,
	}
	r.FailuresByHost = map[string]bool{}
}

func (r *fakeRouter) EnsureBackend(ctx context.Context, app router.App, opts router.EnsureBackendOpts) error {
	name := app.GetName()
	if name == "myapp-with-error" {
		return errors.New("Ensure backend error")
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	for _, cname := range opts.CNames {
		if r.cnames[cname] != "" && r.cnames[cname] != name {
			return ErrForcedFailure
		}

		if r.FailuresByHost[cname] {
			return ErrForcedFailure
		}
	}

	r.BackendOpts[name] = opts

	currentAppCNames := map[string]bool{}

	for _, cname := range opts.CNames {
		r.cnames[cname] = name
		currentAppCNames[cname] = true
	}

	for cname, app := range r.cnames {
		if app != name {
			continue
		}

		if !currentAppCNames[cname] {
			delete(r.cnames, cname)
		}

	}

	currentAppCertIssuers := map[string]bool{}

	for cname, issuer := range opts.CertIssuers {
		r.certIssuers[cname+":"+issuer] = name
		currentAppCertIssuers[cname+":"+issuer] = true
	}

	for key, app := range r.certIssuers {
		if app != name {
			continue
		}

		if !currentAppCertIssuers[key] {
			delete(r.certIssuers, key)
		}
	}

	return nil
}

func (r *fakeRouter) Addresses(ctx context.Context, app router.App) ([]string, error) {
	backendName := app.GetName()
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.FailuresByHost[r.GetName()+":"+backendName] || r.FailuresByHost[backendName] {
		return nil, ErrForcedFailure
	}

	opts := r.BackendOpts[backendName]

	addrs := map[string]bool{
		fmt.Sprintf("%s.fakerouter.com", backendName): true,
	}
	for _, data := range opts.Prefixes {
		name := backendName
		if data.Prefix != "" {
			name = data.Prefix + "." + backendName
		}
		addrs[fmt.Sprintf("%s.fakerouter.com", name)] = true
	}

	sortedAddrs := []string{}
	for addr := range addrs {
		sortedAddrs = append(sortedAddrs, addr)
	}
	sort.Strings(sortedAddrs)

	return sortedAddrs, nil
}

type tlsRouter struct {
	fakeRouter
	Certs map[string]string
	Keys  map[string]string
}

var _ router.TLSRouter = &tlsRouter{}

func (r *tlsRouter) AddCertificate(ctx context.Context, app router.App, cname, certificate, key string) error {
	r.Certs[cname] = certificate
	r.Keys[cname] = key
	return nil
}

func (r *tlsRouter) RemoveCertificate(ctx context.Context, app router.App, cname string) error {
	delete(r.Certs, cname)
	delete(r.Keys, cname)
	return nil
}

func (r *tlsRouter) GetCertificate(ctx context.Context, app router.App, cname string) (string, error) {
	if app.GetName()+".faketlsrouter.com" == cname {
		return "<mock cert>", nil
	}
	data, ok := r.Certs[cname]
	if !ok {
		return "", router.ErrCertificateNotFound
	}
	return data, nil
}

func (r *tlsRouter) Addresses(ctx context.Context, app router.App) ([]string, error) {
	addrs, err := r.fakeRouter.Addresses(ctx, app)
	if err != nil {
		return nil, err
	}
	for i := range addrs {
		addrs[i] = "https://" + strings.Replace(addrs[i], ".fakerouter.com", ".faketlsrouter.com", -1)
	}

	return addrs, nil
}

func (r *tlsRouter) Reset() {
	r.fakeRouter.Reset()
	r.Certs = make(map[string]string)
	r.Keys = make(map[string]string)
}
