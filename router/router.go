// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package router provides interfaces that need to be satisfied in order to
// implement a new router on tsuru.
package router

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	internalConfig "github.com/tsuru/tsuru/config"
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
		return "", "", errors.New(msg)
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

// Router is the basic interface of this package. It provides methods for
// managing backends and routes. Each backend can have multiple routes.
type Router interface {
	GetName() string
	GetType() string

	EnsureBackend(ctx context.Context, app *appTypes.App, o EnsureBackendOpts) error
	RemoveBackend(ctx context.Context, app *appTypes.App) error

	Addresses(ctx context.Context, app *appTypes.App) ([]string, error)

	GetInfo(ctx context.Context) (map[string]string, error)
	GetBackendStatus(ctx context.Context, app *appTypes.App) (status RouterBackendStatus, err error)
}

type BackendPrefix struct {
	Prefix string            `json:"prefix"`
	Target map[string]string `json:"target"` // in kubernetes cluster be like {serviceName: "", namespace: ""}
}

type EnsureBackendOpts struct {
	Opts        map[string]interface{} `json:"opts"`
	CNames      []string               `json:"cnames"`
	Team        string                 `json:"team,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	CertIssuers map[string]string      `json:"certIssuers,omitempty"`
	Prefixes    []BackendPrefix        `json:"prefixes"`
	Healthcheck router.HealthcheckData `json:"healthcheck"`
}

// TLSRouter is a router that supports adding and removing
// certificates for a given cname
type TLSRouter interface {
	AddCertificate(ctx context.Context, app *appTypes.App, cname, certificate, key string) error
	RemoveCertificate(ctx context.Context, app *appTypes.App, cname string) error
	GetCertificate(ctx context.Context, app *appTypes.App, cname string) (string, error)
}

type BackendStatus string

var (
	BackendStatusReady    = BackendStatus("ready")
	BackendStatusNotReady = BackendStatus("not ready")
)

type RouterBackendStatus struct {
	Status BackendStatus `json:"status"`
	Detail string        `json:"detail"`
}

type RouterError struct {
	Op  string
	Err error
}

func (e *RouterError) Error() string {
	return fmt.Sprintf("[router %s] %s", e.Op, e.Err)
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
	return r.GetInfo(ctx)
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
	sort.Strings(keys)
	for _, value := range keys {
		planRouter := legacyConfigToPlanRouter(value)
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
