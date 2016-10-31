// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild

import (
	"net/url"

	"github.com/tsuru/tsuru/router"
)

type RebuildRoutesResult struct {
	Added   []string
	Removed []string
}

type RebuildApp interface {
	GetRouterOpts() map[string]string
	GetName() string
	GetCname() []string
	Router() (router.Router, error)
	RoutableAddresses() ([]url.URL, error)
	UpdateAddr() error
	InternalLock(string) (bool, error)
	Unlock()
}

func RebuildRoutes(app RebuildApp) (*RebuildRoutesResult, error) {
	r, err := app.Router()
	if err != nil {
		return nil, err
	}
	if optsRouter, ok := r.(router.OptsRouter); ok {
		err = optsRouter.AddBackendOpts(app.GetName(), app.GetRouterOpts())
	} else {
		err = r.AddBackend(app.GetName())
	}
	if err != nil && err != router.ErrBackendExists {
		return nil, err
	}
	err = app.UpdateAddr()
	if err != nil {
		return nil, err
	}
	if cnameRouter, ok := r.(router.CNameRouter); ok {
		for _, cname := range app.GetCname() {
			err = cnameRouter.SetCName(cname, app.GetName())
			if err != nil && err != router.ErrCNameExists {
				return nil, err
			}
		}
	}
	oldRoutes, err := r.Routes(app.GetName())
	if err != nil {
		return nil, err
	}
	expectedMap := make(map[string]*url.URL)
	addresses, err := app.RoutableAddresses()
	if err != nil {
		return nil, err
	}
	for i, addr := range addresses {
		expectedMap[addr.Host] = &addresses[i]
	}
	var toRemove []*url.URL
	for _, url := range oldRoutes {
		if _, isPresent := expectedMap[url.Host]; isPresent {
			delete(expectedMap, url.Host)
		} else {
			toRemove = append(toRemove, url)
		}
	}
	var result RebuildRoutesResult
	for _, toAddUrl := range expectedMap {
		err := r.AddRoute(app.GetName(), toAddUrl)
		if err != nil {
			return nil, err
		}
		result.Added = append(result.Added, toAddUrl.String())
	}
	for _, toRemoveUrl := range toRemove {
		err := r.RemoveRoute(app.GetName(), toRemoveUrl)
		if err != nil {
			return nil, err
		}
		result.Removed = append(result.Removed, toRemoveUrl.String())
	}
	return &result, nil
}
