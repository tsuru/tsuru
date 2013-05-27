// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"errors"
	"github.com/globocom/tsuru/router"
	"sync"
)

var FakeRouter = fakeRouter{backends: make(map[string][]string)}

var ErrBackendNotFound = errors.New("Backend not found")

func init() {
	router.Register("fake", &FakeRouter)
}

type fakeRouter struct {
	backends map[string][]string
	mutex    sync.Mutex
}

func (r *fakeRouter) HasBackend(name string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	_, ok := r.backends[name]
	return ok
}

func (r *fakeRouter) HasRoute(name, address string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	routes, ok := r.backends[name]
	if !ok {
		return false
	}
	for _, route := range routes {
		if route == address {
			return true
		}
	}
	return false
}

func (r *fakeRouter) AddBackend(name string) error {
	if r.HasBackend(name) {
		return errors.New("Backend already exists")
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.backends[name] = nil
	return nil
}

func (r *fakeRouter) RemoveBackend(name string) error {
	if !r.HasBackend(name) {
		return ErrBackendNotFound
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.backends, name)
	return nil
}

func (r *fakeRouter) AddRoute(name, ip string) error {
	if !r.HasBackend(name) {
		return ErrBackendNotFound
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	routes := r.backends[name]
	routes = append(routes, ip)
	r.backends[name] = routes
	return nil
}

func (r *fakeRouter) RemoveRoute(name, ip string) error {
	if !r.HasBackend(name) {
		return ErrBackendNotFound
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	index := -1
	routes := r.backends[name]
	for i := range routes {
		if routes[i] == ip {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("Route not found")
	}
	routes[index] = routes[len(routes)-1]
	r.backends[name] = routes[:len(routes)-1]
	return nil
}

func (fakeRouter) AddCName(cname, name string) error {
	return nil
}

func (fakeRouter) RemoveCName(cname, address string) error {
	return nil
}

func (r *fakeRouter) Addr(name string) (string, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if v, ok := r.backends[name]; ok {
		return v[0], nil
	}
	return "", ErrBackendNotFound
}

func (r *fakeRouter) Reset() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.backends = make(map[string][]string)
}
