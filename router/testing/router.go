// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"errors"
	"github.com/globocom/tsuru/router"
	"sync"
)

func init() {
	router.Register("fake", &FakeRouter{})
}

type FakeRouter struct {
	routes   map[string]string
	backends []string
	mutex    sync.Mutex
}

func (r *FakeRouter) AddBackend(name string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.backends = append(r.backends, name)
	return nil
}

func (r *FakeRouter) RemoveBackend(name string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for i, b := range r.backends {
		if name == b {
			r.backends[i], r.backends = r.backends[len(r.backends)-1], r.backends[:len(r.backends)-1]
			break
		}
	}
	return nil
}

func (r *FakeRouter) HasBackend(name string) bool {
	for _, b := range r.backends {
		if name == b {
			return true
		}
	}
	return false
}

func (r *FakeRouter) AddRoute(name, ip string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.routes == nil {
		r.routes = make(map[string]string)
	}
	r.routes[name] = ip
	return nil
}

func (r *FakeRouter) RemoveRoute(name, ip string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.routes != nil {
		delete(r.routes, name)
	}
	return nil
}

func (r *FakeRouter) HasRoute(name string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	_, ok := r.routes[name]
	return ok
}

func (r *FakeRouter) Addr(name string) (string, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if v, ok := r.routes[name]; ok {
		return v, nil
	}
	return "", errors.New("Route not found")
}
