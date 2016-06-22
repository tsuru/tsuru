// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package router provides interfaces that need to be satisfied in order to
// implement a new router on tsuru.
package router

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type routerFactory func(routerName, configPrefix string) (Router, error)

var (
	ErrBackendExists   = errors.New("Backend already exists")
	ErrBackendNotFound = errors.New("Backend not found")
	ErrBackendSwapped  = errors.New("Backend is swapped cannot remove")
	ErrRouteExists     = errors.New("Route already exists")
	ErrRouteNotFound   = errors.New("Route not found")
	ErrCNameExists     = errors.New("CName already exists")
	ErrCNameNotFound   = errors.New("CName not found")
	ErrCNameNotAllowed = errors.New("CName as router subdomain not allowed")
)

var routers = make(map[string]routerFactory)

// Register registers a new router.
func Register(name string, r routerFactory) {
	routers[name] = r
}

// Get gets the named router from the registry.
func Get(name string) (Router, error) {
	prefix := "routers:" + name
	routerType, err := config.GetString(prefix + ":type")
	if err != nil {
		msg := fmt.Sprintf("config key '%s:type' not found", prefix)
		if name != "hipache" {
			return nil, errors.New(msg)
		}
		log.Errorf("WARNING: %s, fallback to top level '%s:*' router config", msg, name)
		routerType = name
		prefix = name
	}
	factory, ok := routers[routerType]
	if !ok {
		return nil, fmt.Errorf("unknown router: %q.", routerType)
	}
	r, err := factory(name, prefix)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Router is the basic interface of this package. It provides methods for
// managing backends and routes. Each backend can have multiple routes.
type Router interface {
	AddBackend(name string) error
	RemoveBackend(name string) error
	AddRoute(name string, address *url.URL) error
	AddRoutes(name string, address []*url.URL) error
	RemoveRoute(name string, address *url.URL) error
	RemoveRoutes(name string, addresses []*url.URL) error
	Addr(name string) (string, error)

	// Swap change the router between two backends.
	Swap(backend1, backend2 string, cnameOnly bool) error

	// Routes returns a list of routes of a backend.
	Routes(name string) ([]*url.URL, error)
}

type CNameRouter interface {
	Router
	SetCName(cname, name string) error
	UnsetCName(cname, name string) error
	CNames(name string) ([]*url.URL, error)
}

type MessageRouter interface {
	StartupMessage() (string, error)
}

type CustomHealthcheckRouter interface {
	SetHealthcheck(name string, data HealthcheckData) error
}

type HealthChecker interface {
	HealthCheck() error
}

type OptsRouter interface {
	AddBackendOpts(name string, opts map[string]string) error
}

type HealthcheckData struct {
	Path   string
	Status int
	Body   string
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
	return conn.Collection("routers"), nil
}

// Store stores the app name related with the
// router name.
func Store(appName, routerName, kind string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	data := map[string]string{
		"app":    appName,
		"router": routerName,
		"kind":   kind,
	}
	return coll.Insert(&data)
}

func retrieveRouterData(appName string) (map[string]string, error) {
	data := map[string]string{}
	coll, err := collection()
	if err != nil {
		return data, err
	}
	defer coll.Close()
	err = coll.Find(bson.M{"app": appName}).One(&data)
	// Avoid need for data migrations, before kind existed we only supported
	// hipache as a router so we set is as default here.
	if data["kind"] == "" {
		data["kind"] = "hipache"
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
	return data["router"], nil
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

func swapCnames(r Router, backend1, backend2 string) error {
	cnameRouter, ok := r.(CNameRouter)
	if !ok {
		return nil
	}
	cnames1, err := cnameRouter.CNames(backend1)
	if err != nil {
		return err
	}
	cnames2, err := cnameRouter.CNames(backend2)
	if err != nil {
		return err
	}
	for _, cname := range cnames1 {
		err = cnameRouter.UnsetCName(cname.String(), backend1)
		if err != nil {
			return err
		}
		err = cnameRouter.SetCName(cname.String(), backend2)
		if err != nil {
			return err
		}
	}
	for _, cname := range cnames2 {
		err = cnameRouter.UnsetCName(cname.String(), backend2)
		if err != nil {
			return err
		}
		err = cnameRouter.SetCName(cname.String(), backend1)
		if err != nil {
			return err
		}
	}
	return nil
}

func swapBackends(r Router, backend1, backend2 string) error {
	routes1, err := r.Routes(backend1)
	if err != nil {
		return err
	}
	routes2, err := r.Routes(backend2)
	if err != nil {
		return err
	}
	err = r.AddRoutes(backend1, routes2)
	if err != nil {
		return err
	}
	err = r.AddRoutes(backend2, routes1)
	if err != nil {
		return err
	}
	err = r.RemoveRoutes(backend1, routes1)
	if err != nil {
		return err
	}
	err = r.RemoveRoutes(backend2, routes2)
	if err != nil {
		return err
	}
	return swapBackendName(backend1, backend2)

}

func Swap(r Router, backend1, backend2 string, cnameOnly bool) error {
	data1, err := retrieveRouterData(backend1)
	if err != nil {
		return err
	}
	data2, err := retrieveRouterData(backend2)
	if err != nil {
		return err
	}
	if data1["kind"] != data2["kind"] {
		return fmt.Errorf("swap is only allowed between routers of the same kind. %q uses %q, %q uses %q",
			backend1, data1["kind"], backend2, data2["kind"])
	}
	if cnameOnly {
		return swapCnames(r, backend1, backend2)
	}
	return swapBackends(r, backend1, backend2)
}

type PlanRouter struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func List() ([]PlanRouter, error) {
	routerConfig, err := config.Get("routers")
	var routers map[interface{}]interface{}
	if err == nil {
		routers, _ = routerConfig.(map[interface{}]interface{})
	}
	routersList := make([]PlanRouter, 0, len(routers))
	var keys []string
	for key := range routers {
		keys = append(keys, key.(string))
	}
	topLevelHipacheConfig, _ := config.Get("hipache")
	if topLevelHipacheConfig != nil {
		keys = append(keys, "hipache")
	}
	sort.Strings(keys)
	for _, value := range keys {
		var routerType string
		routerProperties, _ := routers[value].(map[interface{}]interface{})
		if routerProperties != nil {
			routerType, _ = routerProperties["type"].(string)
		}
		if routerType == "" {
			routerType = value
		}
		routersList = append(routersList, PlanRouter{Name: value, Type: routerType})
	}
	return routersList, nil
}

// validCName returns true if the cname is not a subdomain of
// the router current domain, false otherwise.
func ValidCName(cname, domain string) bool {
	return !strings.HasSuffix(cname, domain)
}

func IsSwapped(name string) (bool, string, error) {
	backendName, err := Retrieve(name)
	if err != nil {
		return false, "", err
	}
	return name != backendName, backendName, nil
}
