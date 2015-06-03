// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vulcand

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/router"

	vulcandAPI "github.com/mailgun/vulcand/api"
	vulcandReg "github.com/mailgun/vulcand/plugin/registry"
)

const routerName = "vulcand"

func init() {
	router.Register(routerName, createRouter)
}

type vulcandRouter struct {
	client *vulcandAPI.Client
	prefix string
	domain string
}

func createRouter(prefix string) (router.Router, error) {
	vURL, err := config.GetString(prefix + ":api-url")
	if err != nil {
		return nil, err
	}

	domain, err := config.GetString(prefix + ":domain")
	if err != nil {
		return nil, err
	}

	client := vulcandAPI.NewClient(vURL, vulcandReg.GetRegistry())
	vRouter := &vulcandRouter{
		client: client,
		prefix: prefix,
		domain: domain,
	}

	return vRouter, nil
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
