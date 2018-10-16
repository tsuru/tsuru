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

func diffRoutes(old []*url.URL, new []url.URL) (toAdd []*url.URL, toRemove []*url.URL) {
	expectedMap := make(map[string]*url.URL)
	for i, addr := range new {
		expectedMap[addr.Host] = &new[i]
	}
	for _, url := range old {
		if _, isPresent := expectedMap[url.Host]; isPresent {
			delete(expectedMap, url.Host)
		} else {
			toRemove = append(toRemove, url)
		}
	}
	for _, toAddURL := range expectedMap {
		toAdd = append(toAdd, toAddURL)
	}
	return toAdd, toRemove
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
		if asyncR == nil {
			err = r.AddBackend(app)
		} else {
			err = asyncR.AddBackendAsync(app)
		}
	}
	if err != nil && err != router.ErrBackendExists {
		return nil, err
	}
	if cnameRouter, ok := r.(router.CNameRouter); ok {
		var oldCnames []*url.URL
		oldCnames, err = cnameRouter.CNames(app.GetName())
		if err != nil {
			return nil, err
		}
		appCnames := app.GetCname()
		cnameAddrs := make([]url.URL, len(appCnames))
		for i, cname := range appCnames {
			cnameAddrs[i] = url.URL{Host: cname}
		}
		_, toRemove := diffRoutes(oldCnames, cnameAddrs)
		for _, cname := range appCnames {
			if asyncR == nil {
				err = cnameRouter.SetCName(cname, app.GetName())
			} else {
				err = asyncR.SetCNameAsync(cname, app.GetName())
			}
			if err != nil && err != router.ErrCNameExists {
				return nil, err
			}
		}
		for _, toRemoveCname := range toRemove {
			err = cnameRouter.UnsetCName(toRemoveCname.Host, app.GetName())
			if err != nil && err != router.ErrCNameNotFound {
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
	addresses, err := app.RoutableAddresses()
	if err != nil {
		return nil, err
	}
	log.Debugf("[rebuild-routes] addresses for app %q: %v", app.GetName(), addresses)
	toAdd, toRemove := diffRoutes(oldRoutes, addresses)
	var result RebuildRoutesResult
	for _, toAddURL := range toAdd {
		result.Added = append(result.Added, toAddURL.String())
	}
	for _, toRemoveURL := range toRemove {
		result.Removed = append(result.Removed, toRemoveURL.String())
	}
	if dry {
		log.Debugf("[rebuild-routes] nothing to do. DRY mode for app: %q", app.GetName())
		return &result, nil
	}
	if asyncR == nil {
		err = r.AddRoutes(app.GetName(), toAdd)
	} else {
		err = asyncR.AddRoutesAsync(app.GetName(), toAdd)
	}
	if err != nil {
		return nil, err
	}
	if asyncR == nil {
		err = r.RemoveRoutes(app.GetName(), toRemove)
	} else {
		err = asyncR.RemoveRoutesAsync(app.GetName(), toRemove)
	}
	if err != nil {
		return nil, err
	}
	log.Debugf("[rebuild-routes] routes added for app %q: %s", app.GetName(), strings.Join(result.Added, ", "))
	log.Debugf("[rebuild-routes] routes removed for app %q: %s", app.GetName(), strings.Join(result.Removed, ", "))
	return &result, nil
}
