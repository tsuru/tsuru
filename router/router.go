// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package router provides interfaces that need to be satisfied in order to
// implement a new router on tsuru.
package router

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	internalConfig "github.com/tsuru/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/router"
)

type routerFactory func(routerName string, config ConfigGetter) (Router, error)

var (
	ErrBackendExists         = errors.New("Backend already exists")
	ErrBackendNotFound       = errors.New("Backend not found")
	ErrBackendSwapped        = errors.New("Backend is swapped cannot remove")
	ErrRouteNotFound         = errors.New("Route not found")
	ErrCNameExists           = errors.New("CName already exists")
	ErrCNameNotFound         = errors.New("CName not found")
	ErrCNameNotAllowed       = errors.New("CName as router subdomain not allowed")
	ErrCertificateNotFound   = errors.New("Certificate not found")
	ErrDefaultRouterNotFound = errors.New("No default router found")

	ErrSwapAmongDifferentClusters = errors.New("Could not swap apps among different clusters")
)

type ErrRouterNotFound struct {
	Name string
}

func (e *ErrRouterNotFound) Error() string {
	return fmt.Sprintf("router %q not found", e.Name)
}

const HttpScheme = "http"

var routers = make(map[string]routerFactory)

// Register registers a new router.
func Register(name string, r routerFactory) {
	routers[name] = r
}

func Unregister(name string) {
	delete(routers, name)
}

func configType(name string) (string, string, error) {
	prefix := "routers:" + name
	routerType, err := config.GetString(prefix + ":type")
	if err != nil {
		msg := fmt.Sprintf("config key '%s:type' not found", prefix)
		if name != "hipache" {
			return "", "", errors.New(msg)
		}
		log.Errorf("WARNING: %s, fallback to top level '%s:*' router config", msg, name)
		return name, name, nil
	}
	return routerType, prefix, nil
}

// Get gets the named router from the registry.
func Get(ctx context.Context, name string) (Router, error) {
	r, _, err := GetWithPlanRouter(ctx, name)
	return r, err
}

func GetWithPlanRouter(ctx context.Context, name string) (Router, router.PlanRouter, error) {
	var planRouter router.PlanRouter

	dr, err := servicemanager.DynamicRouter.Get(ctx, name)
	if err != nil && err != router.ErrDynamicRouterNotFound {
		return nil, planRouter, err
	}
	var routerType string
	var config ConfigGetter
	if dr != nil {
		routerType = dr.Type
		config = configGetterFromData(dr.Config)
		planRouter = dr.ToPlanRouter()
	} else {
		var prefix string
		routerType, prefix, err = configType(name)
		if err != nil {
			return nil, planRouter, &ErrRouterNotFound{Name: name}
		}
		config = ConfigGetterFromPrefix(prefix)
		planRouter = legacyConfigToPlanRouter(name)
	}
	factory, ok := routers[routerType]
	if !ok {
		return nil, planRouter, errors.Errorf("unknown router: %q.", routerType)
	}
	r, err := factory(name, config)
	if err != nil {
		return nil, planRouter, err
	}
	return r, planRouter, nil
}

// Default returns the default router
func Default(ctx context.Context) (string, error) {
	plans, err := List(ctx)
	if err != nil {
		return "", err
	}
	if len(plans) == 0 {
		return "", ErrDefaultRouterNotFound
	}
	if len(plans) == 1 {
		return plans[0].Name, nil
	}
	for _, p := range plans {
		if p.Default {
			return p.Name, nil
		}
	}
	return "", ErrDefaultRouterNotFound
}

// App is the interface implemented by routable applications.
type App interface {
	GetName() string
	GetPool() string
	GetTeamOwner() string
	GetTeamsName() []string
}

// Router is the basic interface of this package. It provides methods for
// managing backends and routes. Each backend can have multiple routes.
type Router interface {
	GetName() string
	GetType() string

	AddBackend(ctx context.Context, app App) error
	RemoveBackend(ctx context.Context, app App) error
	AddRoutes(ctx context.Context, app App, address []*url.URL) error
	RemoveRoutes(ctx context.Context, app App, addresses []*url.URL) error
	Addr(ctx context.Context, app App) (string, error)

	// Swap change the router between two backends.
	Swap(ctx context.Context, app1 App, app2 App, cnameOnly bool) error

	// Routes returns a list of routes of a backend.
	Routes(ctx context.Context, app App) ([]*url.URL, error)
}

type CNameRouter interface {
	SetCName(ctx context.Context, cname string, app App) error
	UnsetCName(ctx context.Context, cname string, app App) error
	CNames(ctx context.Context, app App) ([]*url.URL, error)
}

type CNameMoveRouter interface {
	MoveCName(ctx context.Context, cname string, orgBackend, dstBackend App) error
}

type MessageRouter interface {
	StartupMessage() (string, error)
}

type CustomHealthcheckRouter interface {
	SetHealthcheck(ctx context.Context, app App, data router.HealthcheckData) error
}

type HealthChecker interface {
	HealthCheck(ctx context.Context) error
}

type OptsRouter interface {
	AddBackendOpts(ctx context.Context, app App, opts map[string]string) error
	UpdateBackendOpts(ctx context.Context, app App, opts map[string]string) error
}

// TLSRouter is a router that supports adding and removing
// certificates for a given cname
type TLSRouter interface {
	AddCertificate(ctx context.Context, app App, cname, certificate, key string) error
	RemoveCertificate(ctx context.Context, app App, cname string) error
	GetCertificate(ctx context.Context, app App, cname string) (string, error)
}

type InfoRouter interface {
	GetInfo(ctx context.Context) (map[string]string, error)
}

type AsyncRouter interface {
	AddBackendAsync(ctx context.Context, app App) error
	SetCNameAsync(ctx context.Context, cname string, app App) error
	AddRoutesAsync(ctx context.Context, app App, address []*url.URL) error
	RemoveRoutesAsync(ctx context.Context, app App, addresses []*url.URL) error
}

type PrefixRouter interface {
	RoutesPrefix(ctx context.Context, app App) ([]appTypes.RoutableAddresses, error)
	Addresses(ctx context.Context, app App) ([]string, error)
	AddRoutesPrefix(ctx context.Context, app App, addresses appTypes.RoutableAddresses, sync bool) error
	RemoveRoutesPrefix(ctx context.Context, app App, addresses appTypes.RoutableAddresses, sync bool) error
}

type BackendStatus string

var (
	BackendStatusReady    = BackendStatus("ready")
	BackendStatusNotReady = BackendStatus("not ready")
)

type RouterBackendStatus struct {
	Status BackendStatus `json:"status"`
	Detail string        `json:"detail"`
	Checks []URLCheck    `json:"checks,omitempty"`
}

type URLCheck struct {
	Address string `json:"address"`
	Status  int    `json:"status"`
	Error   string `json:"error"`
}

type StatusRouter interface {
	GetBackendStatus(ctx context.Context, app App, path string) (status RouterBackendStatus, err error)
}

type RouterError struct {
	Op  string
	Err error
}

func (e *RouterError) Error() string {
	return fmt.Sprintf("[router %s] %s", e.Op, e.Err)
}

func collection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	coll := conn.Collection("routers")
	err = coll.EnsureIndex(mgo.Index{Key: []string{"app"}, Unique: true})
	if err != nil {
		return nil, errors.Wrapf(err, "Could not create index on db.routers. Please run `tsurud migrate` before starting the api server to fix this issue.")
	}
	return coll, nil
}

type routerAppEntry struct {
	ID     bson.ObjectId `bson:"_id,omitempty"`
	App    string        `bson:"app"`
	Router string        `bson:"router"`
	Kind   string        `bson:"kind"`
}

// Store stores the app name related with the
// router name.
func Store(appName, routerName, kind string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	data := routerAppEntry{
		App:    appName,
		Router: routerName,
		Kind:   kind,
	}
	_, err = coll.Upsert(bson.M{"app": appName}, data)
	return err
}

func retrieveRouterData(appName string) (routerAppEntry, error) {
	var data routerAppEntry
	coll, err := collection()
	if err != nil {
		return data, err
	}
	defer coll.Close()
	err = coll.Find(bson.M{"app": appName}).One(&data)
	// Avoid need for data migrations, before kind existed we only supported
	// hipache as a router so we set is as default here.
	if data.Kind == "" {
		data.Kind = "hipache"
	}
	return data, err
}

func Retrieve(appName string) (string, error) {
	data, err := retrieveRouterData(appName)
	if err != nil {
		if err == mgo.ErrNotFound {
			return "", ErrBackendNotFound
		}
		return "", err
	}
	return data.Router, nil
}

func Remove(appName string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.Remove(bson.M{"app": appName})
}

func swapBackendName(backend1, backend2 string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	router1, err := Retrieve(backend1)
	if err != nil {
		return err
	}
	router2, err := Retrieve(backend2)
	if err != nil {
		return err
	}
	update := bson.M{"$set": bson.M{"router": router2}}
	err = coll.Update(bson.M{"app": backend1}, update)
	if err != nil {
		return err
	}
	update = bson.M{"$set": bson.M{"router": router1}}
	return coll.Update(bson.M{"app": backend2}, update)
}

func swapCnames(ctx context.Context, r Router, backend1, backend2 App) error {
	cnameRouter, ok := r.(CNameRouter)
	if !ok {
		return nil
	}
	cnames1, err := cnameRouter.CNames(ctx, backend1)
	if err != nil {
		return err
	}
	cnames2, err := cnameRouter.CNames(ctx, backend2)
	if err != nil {
		return err
	}
	swapCnameRouter, _ := r.(CNameMoveRouter)
	for _, cname := range cnames1 {
		if swapCnameRouter == nil {
			err = cnameRouter.UnsetCName(ctx, cname.Host, backend1)
			if err != nil {
				return err
			}
			err = cnameRouter.SetCName(ctx, cname.Host, backend2)
		} else {
			err = swapCnameRouter.MoveCName(ctx, cname.Host, backend1, backend2)
		}
		if err != nil {
			return err
		}
	}
	for _, cname := range cnames2 {
		if swapCnameRouter == nil {
			err = cnameRouter.UnsetCName(ctx, cname.Host, backend2)
			if err != nil {
				return err
			}
			err = cnameRouter.SetCName(ctx, cname.Host, backend1)
		} else {
			err = swapCnameRouter.MoveCName(ctx, cname.Host, backend2, backend1)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func swapBackends(ctx context.Context, r Router, backend1, backend2 App) error {
	if _, isRouterV2 := r.(RouterV2); isRouterV2 {
		return swapBackendName(backend1.GetName(), backend2.GetName())
	}
	routes1, err := r.Routes(ctx, backend1)
	if err != nil {
		return err
	}
	routes2, err := r.Routes(ctx, backend2)
	if err != nil {
		return err
	}
	err = r.AddRoutes(ctx, backend1, routes2)
	if err != nil {
		return err
	}
	err = r.AddRoutes(ctx, backend2, routes1)
	if err != nil {
		return err
	}
	err = r.RemoveRoutes(ctx, backend1, routes1)
	if err != nil {
		return err
	}
	err = r.RemoveRoutes(ctx, backend2, routes2)
	if err != nil {
		return err
	}
	return swapBackendName(backend1.GetName(), backend2.GetName())

}

func Swap(ctx context.Context, r Router, backend1, backend2 App, cnameOnly bool) error {
	data1, err := retrieveRouterData(backend1.GetName())
	if err != nil {
		return err
	}
	data2, err := retrieveRouterData(backend2.GetName())
	if err != nil {
		return err
	}
	if data1.Kind != data2.Kind {
		return errors.Errorf("swap is only allowed between routers of the same kind. %q uses %q, %q uses %q",
			backend1.GetName(), data1.Kind, backend2.GetName(), data2.Kind)
	}
	if cnameOnly {
		return swapCnames(ctx, r, backend1, backend2)
	}
	return swapBackends(ctx, r, backend1, backend2)
}

func ListWithInfo(ctx context.Context) ([]router.PlanRouter, error) {
	routers, err := List(ctx)
	if err != nil {
		return nil, err
	}
	wg := sync.WaitGroup{}
	for i := range routers {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			info, infoErr := fetchRouterInfo(ctx, routers[i].Name)
			if infoErr != nil {
				routers[i].Info = map[string]string{"error": infoErr.Error()}
			} else {
				routers[i].Info = info
			}
		}()
	}
	wg.Wait()
	return routers, nil
}

func fetchRouterInfo(ctx context.Context, name string) (map[string]string, error) {
	r, err := Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if infoR, ok := r.(InfoRouter); ok {
		return infoR.GetInfo(ctx)
	}
	return nil, nil
}

func List(ctx context.Context) ([]router.PlanRouter, error) {
	allRouters, err := listConfigRouters()
	if err != nil {
		return nil, err
	}
	dynamicRouters, err := servicemanager.DynamicRouter.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range dynamicRouters {
		allRouters = append(allRouters, r.ToPlanRouter())
	}
	sort.Slice(allRouters, func(i, j int) bool {
		return allRouters[i].Name < allRouters[j].Name
	})
	return allRouters, nil
}

func listConfigRouters() ([]router.PlanRouter, error) {
	routerConfig, err := config.Get("routers")
	var routers map[interface{}]interface{}
	if err == nil {
		routers, _ = routerConfig.(map[interface{}]interface{})
	}
	routersList := make([]router.PlanRouter, 0, len(routers))
	var keys []string
	for key := range routers {
		keys = append(keys, key.(string))
	}
	topLevelHipacheConfig, _ := config.Get("hipache")
	if topLevelHipacheConfig != nil {
		keys = append(keys, "hipache")
	}
	dockerRouter, _ := config.GetString("docker:router")
	sort.Strings(keys)
	for _, value := range keys {
		planRouter := legacyConfigToPlanRouter(value)
		if planRouter.Name == dockerRouter {
			planRouter.Default = true
		}
		routersList = append(routersList, planRouter)
	}
	return routersList, nil
}

func legacyConfigToPlanRouter(name string) router.PlanRouter {
	routerConfig, err := config.Get("routers")
	var routers map[interface{}]interface{}
	if err == nil {
		routers, _ = routerConfig.(map[interface{}]interface{})
	}
	var routerType string
	var defaultFlag bool
	var readinessGates []string
	routerProperties, _ := routers[name].(map[interface{}]interface{})
	if routerProperties != nil {
		routerType, _ = routerProperties["type"].(string)
		defaultFlag, _ = routerProperties["default"].(bool)
		readinessGatesRaw, ok := routerProperties["readinessGates"].([]interface{})
		if ok {
			for _, readinessGate := range readinessGatesRaw {
				readinessGates = append(readinessGates, fmt.Sprint(readinessGate))
			}
		}
	}
	if routerType == "" {
		routerType = name
	}
	var config map[string]interface{}
	if routerProperties != nil {
		configRaw := internalConfig.ConvertEntries(routerProperties)
		config, _ = configRaw.(map[string]interface{})
		delete(config, "type")
		delete(config, "default")
		delete(config, "readinessGates")
		if len(config) == 0 {
			config = nil
		}
	}
	return router.PlanRouter{
		Name:           name,
		Type:           routerType,
		Config:         config,
		ReadinessGates: readinessGates,
		Default:        defaultFlag,
	}
}

// validCName returns true if the cname is not a subdomain of
// the router current domain, false otherwise.
func ValidCName(cname, domain string) bool {
	return !strings.HasSuffix(cname, domain)
}

func IsSwapped(name string) (bool, string, error) {
	backendName, err := Retrieve(name)
	if err != nil && err == ErrBackendNotFound {
		// NOTE: apps created with "none" router don't have entry on routers collection.
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return name != backendName, backendName, nil
}
