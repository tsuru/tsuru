// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"errors"
	"github.com/globocom/tsuru/router"
)

func init() {
	router.Register("fake", &FakeRouter{})
}

type FakeRouter struct {
	routes map[string]string
}

func (r *FakeRouter) AddRoute(name, ip string) error {
	if r.routes == nil {
		r.routes = make(map[string]string)
	}
	r.routes[name] = ip
	return nil
}

func (r *FakeRouter) RemoveRoute(name string) error {
	if r.routes != nil {
		delete(r.routes, name)
	}
	return nil
}

func (r *FakeRouter) HasRoute(name string) bool {
	_, ok := r.routes[name]
	return ok
}

func (r *FakeRouter) Addr(name string) (string, error) {
	if v, ok := r.routes[name]; ok {
		return v, nil
	}
	return "", errors.New("Route not found")
}
