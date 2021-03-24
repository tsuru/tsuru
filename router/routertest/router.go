// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routertest

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/router"
	appTypes "github.com/tsuru/tsuru/types/app"
	routerTypes "github.com/tsuru/tsuru/types/router"
)

var FakeRouter = newFakeRouter()

var HCRouter = hcRouter{fakeRouter: newFakeRouter()}

var OptsRouter = optsRouter{
	fakeRouter: newFakeRouter(),
	Opts:       make(map[string]map[string]string),
}

var InfoRouter = infoRouter{
	fakeRouter: newFakeRouter(),
	Info:       make(map[string]string),
}

var StatusRouter = statusRouter{
	fakeRouter: newFakeRouter(),
	Status:     router.BackendStatusReady,
}

var TLSRouter = tlsRouter{
	fakeRouter: newFakeRouter(),
	Certs:      make(map[string]string),
	Keys:       make(map[string]string),
}

var PrefixRouter = prefixRouter{
	fakeRouter:   newFakeRouter(),
	prefixRoutes: make(map[string][]appTypes.RoutableAddresses),
}

var FakeRouterV2 = fakeRouterV2{
	fakeRouter: newFakeRouter(),
}

var ErrForcedFailure = errors.New("Forced failure")

func init() {
	router.Register("fake", createRouter)
	router.Register("fake-v2", createRouterV2)
	router.Register("fake-hc", createHCRouter)
	router.Register("fake-tls", createTLSRouter)
	router.Register("fake-opts", createOptsRouter)
	router.Register("fake-info", createInfoRouter)
	router.Register("fake-status", createStatusRouter)
	router.Register("fake-prefix", createPrefixRouter)
}

func createRouter(name string, config router.ConfigGetter) (router.Router, error) {
	return &FakeRouter, nil
}

func createRouterV2(name string, config router.ConfigGetter) (router.Router, error) {
	return &FakeRouterV2, nil
}

func createHCRouter(name string, config router.ConfigGetter) (router.Router, error) {
	return &HCRouter, nil
}

func createTLSRouter(name string, config router.ConfigGetter) (router.Router, error) {
	return &TLSRouter, nil
}

func createOptsRouter(name string, config router.ConfigGetter) (router.Router, error) {
	return &OptsRouter, nil
}

func createInfoRouter(name string, config router.ConfigGetter) (router.Router, error) {
	return &InfoRouter, nil
}

func createStatusRouter(name string, config router.ConfigGetter) (router.Router, error) {
	return &StatusRouter, nil
}

func createPrefixRouter(name string, config router.ConfigGetter) (router.Router, error) {
	return &PrefixRouter, nil
}

func newFakeRouter() fakeRouter {
	return fakeRouter{cnames: make(map[string]string), backends: make(map[string][]string), failuresByIp: make(map[string]bool), healthcheck: make(map[string]routerTypes.HealthcheckData), mutex: &sync.Mutex{}}
}

type fakeRouter struct {
	backends     map[string][]string
	backendAddrs map[string]string
	cnames       map[string]string
	failuresByIp map[string]bool
	healthcheck  map[string]routerTypes.HealthcheckData
	mutex        *sync.Mutex
}

var (
	_ router.Router      = &fakeRouter{}
	_ router.CNameRouter = &fakeRouter{}
)

func (r *fakeRouter) GetName() string {
	return "fake"
}

func (r *fakeRouter) GetType() string {
	return "fake"
}

func (r *fakeRouter) FailForIp(ip string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	u, err := url.Parse(ip)
	if err == nil && u.Host != "" {
		ip = u.Host
	}
	r.failuresByIp[ip] = true
}

func (r *fakeRouter) RemoveFailForIp(ip string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	u, err := url.Parse(ip)
	if err == nil && u.Host != "" {
		ip = u.Host
	}
	delete(r.failuresByIp, ip)
}

func (r *fakeRouter) GetHealthcheck(name string) routerTypes.HealthcheckData {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.healthcheck[name]
}

func (r *fakeRouter) HasBackend(name string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	_, ok := r.backends[name]
	return ok
}

func (r *fakeRouter) CNames(ctx context.Context, app router.App) ([]*url.URL, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	result := []*url.URL{}
	for cname, backendName := range r.cnames {
		if backendName == app.GetName() {
			result = append(result, &url.URL{Host: cname})
		}
	}
	return result, nil
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

func (r *fakeRouter) HasRoute(name, address string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	routes, ok := r.backends[name]
	if !ok {
		routes, ok = r.backends[r.cnames[name]]
		if !ok {
			return false
		}
	}
	u, err := url.Parse(address)
	if err == nil && u.Host != "" {
		address = u.Host
	}
	for _, route := range routes {
		if route == address {
			return true
		}
	}
	return false
}

func (r *fakeRouter) AddBackend(ctx context.Context, app router.App) error {
	name := app.GetName()
	if r.HasBackend(name) {
		return router.ErrBackendExists
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.backends[name] = nil
	return router.Store(name, name, "fake")
}

func (r *fakeRouter) RemoveBackend(ctx context.Context, app router.App) error {
	r.mutex.Lock()
	name := app.GetName()
	if r.failuresByIp[name] {
		r.mutex.Unlock()
		return ErrForcedFailure
	}
	r.mutex.Unlock()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if backendName != name {
		return router.ErrBackendSwapped
	}
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
	delete(r.backends, backendName)
	return nil
}

func (r *fakeRouter) AddRoutes(ctx context.Context, app router.App, addresses []*url.URL) error {
	name := app.GetName()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if !r.HasBackend(backendName) {
		return router.ErrBackendNotFound
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, addr := range addresses {
		if r.failuresByIp[addr.Host] {
			return ErrForcedFailure
		}
	}
	routes := r.backends[backendName]
addresses:
	for _, addr := range addresses {
		for i := range routes {
			if routes[i] == addr.Host {
				continue addresses
			}
		}
		routes = append(routes, addr.Host)
	}
	r.backends[backendName] = routes
	return nil
}

func (r *fakeRouter) RemoveRoutes(ctx context.Context, app router.App, addresses []*url.URL) error {
	name := app.GetName()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if !r.HasBackend(backendName) {
		return router.ErrBackendNotFound
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, addr := range addresses {
		if r.failuresByIp[addr.Host] {
			return ErrForcedFailure
		}
	}
	routes := r.backends[backendName]
	for _, addr := range addresses {
		for i := range routes {
			if routes[i] == addr.Host {
				routes = append(routes[:i], routes[i+1:]...)
				break
			}
		}
	}
	r.backends[backendName] = routes
	return nil
}

func (r *fakeRouter) SetCName(ctx context.Context, cname string, app router.App) error {
	name := app.GetName()
	r.mutex.Lock()
	if r.failuresByIp[cname] {
		r.mutex.Unlock()
		return ErrForcedFailure
	}
	r.mutex.Unlock()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if !r.HasBackend(backendName) {
		return nil
	}
	if !router.ValidCName(cname, "fakerouter.com") {
		return router.ErrCNameNotAllowed
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if _, ok := r.cnames[cname]; ok {
		return router.ErrCNameExists
	}
	r.cnames[cname] = backendName
	return nil
}

func (r *fakeRouter) UnsetCName(ctx context.Context, cname string, app router.App) error {
	name := app.GetName()
	r.mutex.Lock()
	if r.failuresByIp[cname] {
		r.mutex.Unlock()
		return ErrForcedFailure
	}
	r.mutex.Unlock()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if !r.HasBackend(backendName) {
		return nil
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if _, ok := r.cnames[cname]; !ok {
		return router.ErrCNameNotFound
	}
	delete(r.cnames, cname)
	return nil
}

func (r *fakeRouter) SetBackendAddr(ctx context.Context, name, addr string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.backendAddrs == nil {
		r.backendAddrs = make(map[string]string)
	}
	r.backendAddrs[name] = addr
}

func (r *fakeRouter) Addr(ctx context.Context, app router.App) (string, error) {
	name := app.GetName()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.failuresByIp[r.GetName()+name] || r.failuresByIp[name] {
		return "", ErrForcedFailure
	}
	backendName, err := router.Retrieve(name)
	if err != nil {
		return "", err
	}
	if _, ok := r.backends[backendName]; ok {
		if r.backendAddrs != nil && r.backendAddrs[backendName] != "" {
			return r.backendAddrs[backendName], nil
		}
		return fmt.Sprintf("%s.fakerouter.com", backendName), nil
	}
	return "", router.ErrBackendNotFound
}

func (r *fakeRouter) Reset() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.backendAddrs = nil
	r.backends = make(map[string][]string)
	r.failuresByIp = make(map[string]bool)
	r.cnames = make(map[string]string)
	r.healthcheck = make(map[string]routerTypes.HealthcheckData)
}

func (r *fakeRouter) Routes(ctx context.Context, app router.App) ([]*url.URL, error) {
	name := app.GetName()
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	routes := r.backends[backendName]
	result := make([]*url.URL, len(routes))
	for i, route := range routes {
		result[i] = &url.URL{Scheme: router.HttpScheme, Host: route}
	}
	return result, nil
}

func (r *fakeRouter) Swap(ctx context.Context, app1, app2 router.App, cnameOnly bool) error {
	return router.Swap(ctx, r, app1, app2, cnameOnly)
}

type fakeRouterV2 struct {
	fakeRouter
}

var (
	_ router.RouterV2 = &fakeRouterV2{}
)

func (r *fakeRouterV2) EnsureBackend(ctx context.Context, app router.App, opts router.EnsureBackendOpts) error {
	name := app.GetName()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.backends[name] = nil

	return nil
}

type hcRouter struct {
	fakeRouter
	err error
}

var (
	_ router.CustomHealthcheckRouter = &hcRouter{}
	_ router.HealthChecker           = &hcRouter{}
)

func (r *hcRouter) SetErr(err error) {
	r.err = err
}

func (r *hcRouter) HealthCheck(context.Context) error {
	return r.err
}

func (r *hcRouter) Addr(ctx context.Context, app router.App) (string, error) {
	addr, err := r.fakeRouter.Addr(ctx, app)
	if err != nil {
		return "", err
	}
	return strings.Replace(addr, ".fakerouter.com", ".fakehcrouter.com", -1), nil
}

func (r *fakeRouter) SetHealthcheck(ctx context.Context, app router.App, data routerTypes.HealthcheckData) error {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.healthcheck[backendName] = data
	return nil
}

func (r *fakeRouter) RemoveHealthcheck(ctx context.Context, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.healthcheck, backendName)
	return nil
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
	data, ok := r.Certs[cname]
	if !ok {
		return "", router.ErrCertificateNotFound
	}
	return data, nil
}

func (r *tlsRouter) Addr(ctx context.Context, app router.App) (string, error) {
	addr, err := r.fakeRouter.Addr(ctx, app)
	if err != nil {
		return "", err
	}
	return strings.Replace(addr, ".fakerouter.com", ".faketlsrouter.com", -1), nil
}

type optsRouter struct {
	fakeRouter
	Opts map[string]map[string]string
}

var _ router.OptsRouter = &optsRouter{}

func (r *optsRouter) AddBackendOpts(ctx context.Context, app router.App, opts map[string]string) error {
	r.Opts[app.GetName()] = opts
	return r.fakeRouter.AddBackend(ctx, app)
}

func (r *optsRouter) UpdateBackendOpts(ctx context.Context, app router.App, opts map[string]string) error {
	r.Opts[app.GetName()] = opts
	return nil
}

type infoRouter struct {
	fakeRouter
	Info map[string]string
}

var _ router.InfoRouter = &infoRouter{}

func (r *infoRouter) GetInfo(ctx context.Context) (map[string]string, error) {
	return r.Info, nil
}

func (r *infoRouter) Reset() {
	r.fakeRouter.Reset()
	r.Info = make(map[string]string)
}

type statusRouter struct {
	fakeRouter
	Status       router.BackendStatus
	StatusDetail string
}

var _ router.StatusRouter = &statusRouter{}

func (r *statusRouter) GetBackendStatus(ctx context.Context, app router.App) (router.BackendStatus, string, error) {
	return r.Status, r.StatusDetail, nil
}

func (r *statusRouter) Reset() {
	r.fakeRouter.Reset()
	r.Status = router.BackendStatusReady
	r.StatusDetail = ""
}

type prefixRouter struct {
	fakeRouter
	prefixRoutes map[string][]appTypes.RoutableAddresses
}

var _ router.PrefixRouter = &prefixRouter{}

func (r *prefixRouter) RoutesPrefix(ctx context.Context, app router.App) ([]appTypes.RoutableAddresses, error) {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.prefixRoutes[backendName], nil
}

func (r *prefixRouter) Addresses(ctx context.Context, app router.App) ([]string, error) {
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	prefixes := r.prefixRoutes[backendName]
	var addrs []string
	for _, data := range prefixes {
		name := backendName
		if data.Prefix != "" {
			name = data.Prefix + "." + backendName
		}
		addrs = append(addrs, fmt.Sprintf("%s.fakerouter.com", name))
	}
	return addrs, nil
}

func (r *prefixRouter) AddRoutesPrefix(ctx context.Context, app router.App, addresses appTypes.RoutableAddresses, sync bool) error {
	name := app.GetName()
	if addresses.Prefix == "" {
		err := r.AddRoutes(ctx, app, addresses.Addresses)
		if err != nil {
			return err
		}
	}
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	routableAddrs := r.prefixRoutes[backendName]
	idx := -1
	for i, r := range routableAddrs {
		if addresses.Prefix == r.Prefix {
			idx = i
			break
		}
	}
	if idx == -1 {
		routableAddrs = append(routableAddrs, appTypes.RoutableAddresses{Prefix: addresses.Prefix})
		idx = len(routableAddrs) - 1
	}
	routableAddrs[idx].ExtraData = addresses.ExtraData

	for _, addr := range addresses.Addresses {
		if containsHost(routableAddrs[idx].Addresses, addr) {
			continue
		}
		routableAddrs[idx].Addresses = append(routableAddrs[idx].Addresses, addr)
	}
	r.prefixRoutes[backendName] = routableAddrs
	return nil
}

func (r *prefixRouter) RemoveRoutesPrefix(ctx context.Context, app router.App, addresses appTypes.RoutableAddresses, sync bool) error {
	name := app.GetName()
	if addresses.Prefix == "" {
		err := r.RemoveRoutes(ctx, app, addresses.Addresses)
		if err != nil {
			return err
		}
	}
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	routableAddrs := r.prefixRoutes[backendName]
	for i, r := range routableAddrs {
		if addresses.Prefix != r.Prefix {
			continue
		}
		for _, addr := range addresses.Addresses {
			for j := range r.Addresses {
				if r.Addresses[j].Host == addr.Host {
					r.Addresses = append(r.Addresses[:j], r.Addresses[j+1:]...)
					break
				}
			}
		}
		routableAddrs[i] = r
	}
	r.prefixRoutes[backendName] = routableAddrs
	return nil
}

func containsHost(addrs []*url.URL, addr *url.URL) bool {
	for i := range addrs {
		if addrs[i].Host == addr.Host {
			return true
		}
	}
	return false
}
