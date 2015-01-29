// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package router provides interfaces that need to be satisfied in order to
// implement a new router on tsuru.
package router

import (
	"errors"
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2/bson"
)

type routerFactory func(string) (Router, error)

var ErrRouteNotFound = errors.New("Route not found")

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
		log.Errorf("WARNING: config key '%s:type' not found, fallback to top level '%s:*' router config", prefix, name)
		routerType = name
		prefix = name
	}
	factory, ok := routers[routerType]
	if !ok {
		return nil, fmt.Errorf("Unknown router: %q.", routerType)
	}
	r, err := factory(prefix)
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
	AddRoute(name, address string) error
	RemoveRoute(name, address string) error
	SetCName(cname, name string) error
	UnsetCName(cname, name string) error
	Addr(name string) (string, error)

	// Swap change the router between two backends.
	Swap(string, string) error

	// Routes returns a list of routes of a backend.
	Routes(name string) ([]string, error)
}

type MessageRouter interface {
	StartupMessage() (string, error)
}

type HealthcheckRouter interface {
	Healthcheck() error
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
		return "", err
	}
	return data["router"], nil
}

func Remove(appName string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	return coll.Remove(bson.M{"app": appName})
}

func swapBackendName(backend1, backend2 string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
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

func Swap(r Router, backend1, backend2 string) error {
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
	routes1, err := r.Routes(backend1)
	if err != nil {
		return err
	}
	routes2, err := r.Routes(backend2)
	if err != nil {
		return err
	}
	for _, route := range routes1 {
		err = r.AddRoute(backend2, route)
		if err != nil {
			return err
		}
		err = r.RemoveRoute(backend1, route)
		if err != nil {
			return err
		}
	}
	for _, route := range routes2 {
		err = r.AddRoute(backend1, route)
		if err != nil {
			return err
		}
		err = r.RemoveRoute(backend2, route)
		if err != nil {
			return err
		}
	}
	return swapBackendName(backend1, backend2)
}
