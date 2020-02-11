// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild

import (
	"net/url"
	"sort"
	"strings"

	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router"
	appTypes "github.com/tsuru/tsuru/types/app"
	routerTypes "github.com/tsuru/tsuru/types/router"
)

type RebuildRoutesResult struct {
	PrefixResults []RebuildPrefixResult
}

type RebuildPrefixResult struct {
	Prefix  string
	Added   []string
	Removed []string
}

type RebuildApp interface {
	router.App
	GetCname() []string
	GetRouters() []appTypes.AppRouter
	GetHealthcheckData() (routerTypes.HealthcheckData, error)
	RoutableAddresses() ([]appTypes.RoutableAddresses, error)
}

func RebuildRoutes(app RebuildApp, dry bool) (map[string]RebuildRoutesResult, error) {
	return rebuildRoutes(app, dry, true)
}

func rebuildRoutesAsync(app RebuildApp, dry bool) (map[string]RebuildRoutesResult, error) {
	return rebuildRoutes(app, dry, false)
}

func rebuildRoutes(app RebuildApp, dry, wait bool) (map[string]RebuildRoutesResult, error) {
	result := make(map[string]RebuildRoutesResult)
	multi := errors.NewMultiError()
	for _, appRouter := range app.GetRouters() {
		resultInRouter, err := rebuildRoutesInRouter(app, dry, appRouter, wait)
		if err == nil {
			result[appRouter.Name] = *resultInRouter
		} else {
			multi.Add(err)
		}
	}
	return result, multi.ToError()
}

func resultHasChanges(result map[string]RebuildRoutesResult) bool {
	for _, routerResult := range result {
		for _, prefixResult := range routerResult.PrefixResults {
			if len(prefixResult.Added) > 0 || len(prefixResult.Removed) > 0 {
				return true
			}
		}
	}
	return false
}

func diffRoutes(old []*url.URL, new []*url.URL) (toAdd []*url.URL, toRemove []*url.URL) {
	expectedMap := make(map[string]*url.URL)
	for i, addr := range new {
		expectedMap[addr.Host] = new[i]
	}
	for _, url := range old {
		if _, isPresent := expectedMap[url.Host]; !isPresent {
			toRemove = append(toRemove, url)
		}
		delete(expectedMap, url.Host)
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
		cnameAddrs := make([]*url.URL, len(appCnames))
		for i, cname := range appCnames {
			cnameAddrs[i] = &url.URL{Host: cname}
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

	prefixRouter, isPrefixRouter := r.(router.PrefixRouter)
	var oldRoutes []appTypes.RoutableAddresses
	if isPrefixRouter {
		oldRoutes, err = prefixRouter.RoutesPrefix(app.GetName())
		if err != nil {
			return nil, err
		}
	} else {
		var simpleOldRoutes []*url.URL
		simpleOldRoutes, err = r.Routes(app.GetName())
		if err != nil {
			return nil, err
		}
		oldRoutes = []appTypes.RoutableAddresses{{Addresses: simpleOldRoutes}}
	}
	oldPrefixMap := make(map[string]appTypes.RoutableAddresses)
	for _, addrs := range oldRoutes {
		oldPrefixMap[addrs.Prefix] = addrs
	}

	log.Debugf("[rebuild-routes] old routes for app %q: %v", app.GetName(), oldRoutes)
	routableAddresses, err := app.RoutableAddresses()
	if err != nil {
		return nil, err
	}
	log.Debugf("[rebuild-routes] addresses for app %q: %v", app.GetName(), routableAddresses)
	var result RebuildRoutesResult
	for _, addresses := range routableAddresses {
		if addresses.Prefix != "" && !isPrefixRouter {
			continue
		}

		prefixResult := RebuildPrefixResult{
			Prefix: addresses.Prefix,
		}
		oldRoutesForPrefix := oldPrefixMap[addresses.Prefix]
		toAdd, toRemove := diffRoutes(oldRoutesForPrefix.Addresses, addresses.Addresses)
		for _, toAddURL := range toAdd {
			prefixResult.Added = append(prefixResult.Added, toAddURL.String())
		}
		for _, toRemoveURL := range toRemove {
			prefixResult.Removed = append(prefixResult.Removed, toRemoveURL.String())
		}
		sort.Strings(prefixResult.Added)
		sort.Strings(prefixResult.Removed)
		result.PrefixResults = append(result.PrefixResults, prefixResult)
		if dry {
			log.Debugf("[rebuild-routes] nothing to do. DRY mode for app: %q", app.GetName())
			return &result, nil
		}

		if isPrefixRouter {
			addresses.Addresses = toAdd
			err = prefixRouter.AddRoutesPrefix(app.GetName(), addresses, wait)
		} else if asyncR == nil {
			err = r.AddRoutes(app.GetName(), toAdd)
		} else {
			err = asyncR.AddRoutesAsync(app.GetName(), toAdd)
		}
		if err != nil {
			return nil, err
		}
		if isPrefixRouter {
			oldRoutesForPrefix.Addresses = toRemove
			err = prefixRouter.AddRoutesPrefix(app.GetName(), oldRoutesForPrefix, wait)
		} else if asyncR == nil {
			err = r.RemoveRoutes(app.GetName(), toRemove)
		} else {
			err = asyncR.RemoveRoutesAsync(app.GetName(), toRemove)
		}
		if err != nil {
			return nil, err
		}
		log.Debugf("[rebuild-routes] routes added for app %q, prefix %q: %s", app.GetName(), addresses.Prefix, strings.Join(prefixResult.Added, ", "))
		log.Debugf("[rebuild-routes] routes removed for app %q, prefix %q: %s", app.GetName(), addresses.Prefix, strings.Join(prefixResult.Removed, ", "))
	}
	sort.Slice(result.PrefixResults, func(i, j int) bool {
		return result.PrefixResults[i].Prefix < result.PrefixResults[j].Prefix
	})
	return &result, nil
}
