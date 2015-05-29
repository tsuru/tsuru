// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vulcand

import (
	"github.com/tsuru/tsuru/router"
)

const routerName = "vulcand"

func init() {
	router.Register(routerName, createRouter)
}

type vulcandRouter struct{}

func createRouter(prefix string) (router.Router, error) {
	return &vulcandRouter{}, nil
}

func (r *vulcandRouter) AddBackend(name string) error {
	return nil
}

func (r *vulcandRouter) RemoveBackend(name string) error {
	return nil
}

func (r *vulcandRouter) AddRoute(name, address string) error {
	return nil
}

func (r *vulcandRouter) RemoveRoute(name, address string) error {
	return nil
}

func (r *vulcandRouter) SetCName(cname, name string) error {
	return nil
}

func (r *vulcandRouter) UnsetCName(cname, name string) error {
	return nil
}

func (r *vulcandRouter) Addr(name string) (string, error) {
	return "", nil
}

func (r *vulcandRouter) Swap(string, string) error {
	return nil
}

func (r *vulcandRouter) Routes(name string) ([]string, error) {
	return []string{}, nil
}

func (r *vulcandRouter) StartupMessage() (string, error) {
	return "", nil
}
