// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild

import (
	"net/url"
	"strings"

	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type RebuildRoutesResult struct {
	Added   []string
	Removed []string
}

type RebuildApp interface {
	router.App
	GetCname() []string
	GetRouters() []appTypes.AppRouter
	GetHealthcheckData() (router.HealthcheckData, error)
	RoutableAddresses() ([]url.URL, error)
	InternalLock(string) (bool, error)
	Unlock()
}

func RebuildRoutes(app RebuildApp, dry bool) (map[string]RebuildRoutesResult, error) {
	return rebuildRoutes(app, dry, true)
}

func rebuildRoutesAsync(app RebuildApp, dry bool) (map[string]RebuildRoutesResult, error) {
	return rebuildRoutes(app, dry, false)
}

func rebuildRoutes(app RebuildApp, dry, wait bool) (map[string]RebuildRoutesResult, error) {
	result := make(map[string]RebuildRoutesResult)
	for _, appRouter := range app.GetRouters() {
		resultInRouter, err := rebuildRoutesInRouter(app, dry, appRouter, wait)
		if err != nil {
			return nil, err
		}
		result[appRouter.Name] = *resultInRouter
	}
	return result, nil
}

func rebuildRoutesInRouter(app RebuildApp, dry bool, appRouter appTypes.AppRouter, wait bool) (*RebuildRoutesResult, error) {
	log.Debugf("[rebuild-routes] rebuilding routes for app %q", app.GetName())
	r, err := router.Get(appRouter.Name)
	if err != nil {
		return nil, err
	}
	var asyncR router.AsyncRouter
	if !wait {
		asyncR, _ = r.(router.AsyncRouter)
	}
	if optsRouter, ok := r.(router.OptsRouter); ok {
		err = optsRouter.AddBackendOpts(app, appRouter.Opts)
	} else {
		err = r.AddBackend(app)
	}
	if err != nil && err != router.ErrBackendExists {
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
	if hcRouter, ok := r.(router.CustomHealthcheckRouter); ok {
		hcData, errHc := app.GetHealthcheckData()
		if errHc != nil {
			return nil, errHc
		}
		errHc = hcRouter.SetHealthcheck(app.GetName(), hcData)
		if errHc != nil {
			return nil, errHc
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
	if asyncR != nil {
		err = asyncR.AddRoutesAsync(app.GetName(), toAdd)
	} else {
		err = r.AddRoutes(app.GetName(), toAdd)
	}
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
