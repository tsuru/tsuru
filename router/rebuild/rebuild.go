// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild

import (
	"net/url"
	"strings"

	"github.com/tsuru/tsuru/log"
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
	GetRouter() (router.Router, error)
	RoutableAddresses() ([]url.URL, error)
	UpdateAddr() error
	InternalLock(string) (bool, error)
	Unlock()
}

func RebuildRoutes(app RebuildApp, dry bool) (*RebuildRoutesResult, error) {
	log.Debugf("[rebuild-routes] rebuilding routes for app %q", app.GetName())
	r, err := app.GetRouter()
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
	log.Debugf("[rebuild-routes] old routes for app %q: %v", app.GetName(), oldRoutes)
	expectedMap := make(map[string]*url.URL)
	addresses, err := app.RoutableAddresses()
	if err != nil {
		return nil, err
	}
	log.Debugf("[rebuild-routes] addresses for app %q: %v", app.GetName(), addresses)
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
	var toAdd []*url.URL
	for _, toAddURL := range expectedMap {
		toAdd = append(toAdd, toAddURL)
		result.Added = append(result.Added, toAddURL.String())
	}
	for _, toRemoveURL := range toRemove {
		result.Removed = append(result.Removed, toRemoveURL.String())
	}
	if dry {
		log.Debugf("[rebuild-routes] nothing to do. DRY mode for app: %q", app.GetName())
		return &result, nil
	}
	err = r.AddRoutes(app.GetName(), toAdd)
	if err != nil {
		return nil, err
	}
	err = r.RemoveRoutes(app.GetName(), toRemove)
	if err != nil {
		return nil, err
	}
	log.Debugf("[rebuild-routes] routes added for app %q: %s", app.GetName(), strings.Join(result.Added, ", "))
	log.Debugf("[rebuild-routes] routes removed for app %q: %s", app.GetName(), strings.Join(result.Removed, ", "))
	return &result, nil
}
