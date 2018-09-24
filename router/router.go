// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package router provides interfaces that need to be satisfied in order to
// implement a new router on tsuru.
package router

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
)

type routerFactory func(routerName, configPrefix string) (Router, error)

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

func Type(name string) (string, string, error) {
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
func Get(name string) (Router, error) {
	routerType, prefix, err := Type(name)
	if err != nil {
		return nil, &ErrRouterNotFound{Name: name}
	}
	factory, ok := routers[routerType]
	if !ok {
		return nil, errors.Errorf("unknown router: %q.", routerType)
	}
	r, err := factory(name, prefix)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Default returns the default router
func Default() (string, error) {
	plans, err := List()
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

	AddBackend(app App) error
	RemoveBackend(name string) error
	AddRoutes(name string, address []*url.URL) error
	RemoveRoutes(name string, addresses []*url.URL) error
	Addr(name string) (string, error)

	// Swap change the router between two backends.
	Swap(backend1, backend2 string, cnameOnly bool) error

	// Routes returns a list of routes of a backend.
	Routes(name string) ([]*url.URL, error)
}

type CNameRouter interface {
	SetCName(cname, name string) error
	UnsetCName(cname, name string) error
	CNames(name string) ([]*url.URL, error)
}

type CNameMoveRouter interface {
	MoveCName(cname, orgBackend, dstBackend string) error
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
	AddBackendOpts(app App, opts map[string]string) error
	UpdateBackendOpts(app App, opts map[string]string) error
}

// TLSRouter is a router that supports adding and removing
// certificates for a given cname
type TLSRouter interface {
	AddCertificate(app App, cname, certificate, key string) error
	RemoveCertificate(app App, cname string) error
	GetCertificate(app App, cname string) (string, error)
}

type InfoRouter interface {
	GetInfo() (map[string]string, error)
}

type AsyncRouter interface {
	AddBackendAsync(app App) error
	SetCNameAsync(cname, name string) error
	AddRoutesAsync(name string, address []*url.URL) error
	RemoveRoutesAsync(name string, addresses []*url.URL) error
}

type BackendStatus string

var (
	BackendStatusReady    = BackendStatus("ready")
	BackendStatusNotReady = BackendStatus("not ready")
)

type StatusRouter interface {
	GetBackendStatus(name string) (status BackendStatus, detail string, err error)
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
	swapCnameRouter, _ := r.(CNameMoveRouter)
	for _, cname := range cnames1 {
		if swapCnameRouter == nil {
			err = cnameRouter.UnsetCName(cname.Host, backend1)
			if err != nil {
				return err
			}
			err = cnameRouter.SetCName(cname.Host, backend2)
		} else {
			err = swapCnameRouter.MoveCName(cname.Host, backend1, backend2)
		}
		if err != nil {
			return err
		}
	}
	for _, cname := range cnames2 {
		if swapCnameRouter == nil {
			err = cnameRouter.UnsetCName(cname.Host, backend2)
			if err != nil {
				return err
			}
			err = cnameRouter.SetCName(cname.Host, backend1)
		} else {
			err = swapCnameRouter.MoveCName(cname.Host, backend2, backend1)
		}
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
	if data1.Kind != data2.Kind {
		return errors.Errorf("swap is only allowed between routers of the same kind. %q uses %q, %q uses %q",
			backend1, data1.Kind, backend2, data2.Kind)
	}
	if cnameOnly {
		return swapCnames(r, backend1, backend2)
	}
	return swapBackends(r, backend1, backend2)
}

type PlanRouter struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Info    map[string]string `json:"info"`
	Default bool              `json:"default"`
}

func ListWithInfo() ([]PlanRouter, error) {
	routers, err := List()
	if err != nil {
		return nil, err
	}
	for i, planRouter := range routers {
		r, err := Get(planRouter.Name)
		if err != nil {
			return nil, err
		}
		if infoR, ok := r.(InfoRouter); ok {
			routers[i].Info, err = infoR.GetInfo()
			if err != nil {
				return nil, err
			}
		}
	}
	return routers, nil
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
	dockerRouter, _ := config.GetString("docker:router")
	sort.Strings(keys)
	for _, value := range keys {
		var routerType string
		var defaultFlag bool
		routerProperties, _ := routers[value].(map[interface{}]interface{})
		if routerProperties != nil {
			routerType, _ = routerProperties["type"].(string)
			defaultFlag, _ = routerProperties["default"].(bool)
		}
		if routerType == "" {
			routerType = value
		}
		if !defaultFlag {
			defaultFlag = value == dockerRouter
		}
		routersList = append(routersList, PlanRouter{
			Name:    value,
			Type:    routerType,
			Default: defaultFlag,
		})
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
