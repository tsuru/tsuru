// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package router provides interfaces that need to be satisfied in order to
// implement a new router on tsuru.
package router

import "fmt"

// Router is the basic interface of this package. It provides methods for
// managing backends and routes. Each backend can have multiple routes.
type Router interface {
	AddBackend(name string) error
	RemoveBackend(name string) error
	AddRoute(name, address string) error
	RemoveRoute(name, address string) error
	SetCName(cname, name string) error
	UnsetCName(cname string) error
	Addr(name string) (string, error)
}

var routers = make(map[string]Router)

// Register registers a new router.
func Register(name string, r Router) {
	routers[name] = r
}

// Get gets the named router from the registry.
func Get(name string) (Router, error) {
	r, ok := routers[name]
	if !ok {
		return nil, fmt.Errorf("Unknown router: %q.", name)
	}
	return r, nil
}
