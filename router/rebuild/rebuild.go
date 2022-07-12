// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/set"
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
	RoutableAddresses(context.Context) ([]appTypes.RoutableAddresses, error)
}

type RebuildRoutesOpts struct {
	App               RebuildApp
	Writer            io.Writer
	Dry               bool
	Wait              bool
	PreserveOldCNames bool
}

func RebuildRoutes(ctx context.Context, opts RebuildRoutesOpts) (map[string]RebuildRoutesResult, error) {
	result := make(map[string]RebuildRoutesResult)
	multi := errors.NewMultiError()
	writer := opts.Writer

	if writer == nil {
		opts.Writer = ioutil.Discard
	}

	for _, appRouter := range opts.App.GetRouters() {
		resultInRouter, err := RebuildRoutesInRouter(ctx, appRouter, opts)
		if err == nil {
			result[appRouter.Name] = *resultInRouter
		} else {
			multi.Add(err)
		}
	}
	return result, multi.ToError()
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

func RebuildRoutesInRouter(ctx context.Context, appRouter appTypes.AppRouter, o RebuildRoutesOpts) (*RebuildRoutesResult, error) {
	log.Debugf("[rebuild-routes] rebuilding routes for app %q", o.App.GetName())
	if o.Writer == nil {
		o.Writer = ioutil.Discard
	}
	fmt.Fprintf(o.Writer, "\n---- Updating router [%s] ----\n", appRouter.Name)
	r, err := router.Get(ctx, appRouter.Name)
	if err != nil {
		return nil, err
	}

	if routerV2, isRouterV2 := r.(router.RouterV2); isRouterV2 {
		routes, routesErr := o.App.RoutableAddresses(ctx)
		if routesErr != nil {
			return nil, routesErr
		}
		hcData, errHc := o.App.GetHealthcheckData()
		if errHc != nil {
			return nil, errHc
		}
		opts := router.EnsureBackendOpts{
			Opts:        map[string]interface{}{},
			Prefixes:    []router.BackendPrefix{},
			CNames:      o.App.GetCname(),
			Healthcheck: hcData,

			PreserveOldCNames: o.PreserveOldCNames,
		}
		for key, opt := range appRouter.Opts {
			opts.Opts[key] = opt
		}
		var resultRouterV2 RebuildRoutesResult
		for _, route := range routes {
			opts.Prefixes = append(opts.Prefixes, router.BackendPrefix{
				Prefix: route.Prefix,
				Target: route.ExtraData,
			})
			resultRouterV2.PrefixResults = append(resultRouterV2.PrefixResults, RebuildPrefixResult{
				Prefix: route.Prefix,
			})
		}
		err = routerV2.EnsureBackend(ctx, o.App, opts)
		if err != nil {
			return nil, err
		}
		return &resultRouterV2, nil
	}

	var asyncR router.AsyncRouter
	if !o.Wait {
		asyncR, _ = r.(router.AsyncRouter)
	}

	if optsRouter, ok := r.(router.OptsRouter); ok {
		err = optsRouter.AddBackendOpts(ctx, o.App, appRouter.Opts)
	} else {
		if asyncR == nil {
			err = r.AddBackend(ctx, o.App)
		} else {
			err = asyncR.AddBackendAsync(ctx, o.App)
		}
	}
	if err != nil && err != router.ErrBackendExists {
		return nil, err
	}
	if cnameRouter, ok := r.(router.CNameRouter); ok {
		var oldCnames []*url.URL
		oldCnames, err = cnameRouter.CNames(ctx, o.App)
		if err != nil {
			return nil, err
		}
		appCnames := o.App.GetCname()
		cnameAddrs := make([]*url.URL, len(appCnames))
		for i, cname := range appCnames {
			cnameAddrs[i] = &url.URL{Host: cname}
		}
		_, toRemove := diffRoutes(oldCnames, cnameAddrs)
		for _, cname := range appCnames {
			fmt.Fprintf(o.Writer, " ---> Adding cname: %s\n", cname)
			if asyncR == nil {
				err = cnameRouter.SetCName(ctx, cname, o.App)
			} else {
				err = asyncR.SetCNameAsync(ctx, cname, o.App)
			}
			if err != nil && err != router.ErrCNameExists {
				return nil, err
			}
		}
		for _, toRemoveCname := range toRemove {
			fmt.Fprintf(o.Writer, " ---> Removing cname: %s\n", toRemoveCname.Host)
			err = cnameRouter.UnsetCName(ctx, toRemoveCname.Host, o.App)
			if err != nil && err != router.ErrCNameNotFound {
				return nil, err
			}
		}
	}
	if hcRouter, ok := r.(router.CustomHealthcheckRouter); ok {
		hcData, errHc := o.App.GetHealthcheckData()
		if errHc != nil {
			return nil, errHc
		}
		fmt.Fprintf(o.Writer, " ---> Setting healthcheck: %s\n", hcData.String())
		errHc = hcRouter.SetHealthcheck(ctx, o.App, hcData)
		if errHc != nil {
			return nil, errHc
		}
	}

	prefixRouter, isPrefixRouter := r.(router.PrefixRouter)
	var oldRoutes []appTypes.RoutableAddresses
	if isPrefixRouter {
		oldRoutes, err = prefixRouter.RoutesPrefix(ctx, o.App)
		if err != nil {
			return nil, err
		}
	} else {
		var simpleOldRoutes []*url.URL
		simpleOldRoutes, err = r.Routes(ctx, o.App)
		if err != nil {
			return nil, err
		}
		oldRoutes = []appTypes.RoutableAddresses{{Addresses: simpleOldRoutes}}
	}
	log.Debugf("[rebuild-routes] old routes for app %q: %+v", o.App.GetName(), oldRoutes)

	allPrefixes := set.Set{}

	oldPrefixMap := make(map[string]appTypes.RoutableAddresses)
	for _, addrs := range oldRoutes {
		oldPrefixMap[addrs.Prefix] = addrs
		allPrefixes.Add(addrs.Prefix)
	}

	newRoutes, err := o.App.RoutableAddresses(ctx)
	if err != nil {
		return nil, err
	}
	log.Debugf("[rebuild-routes] addresses for app %q: %+v", o.App.GetName(), newRoutes)

	newPrefixMap := make(map[string]appTypes.RoutableAddresses)
	for _, addrs := range newRoutes {
		newPrefixMap[addrs.Prefix] = addrs
		allPrefixes.Add(addrs.Prefix)
	}

	resultCh := make(chan RebuildPrefixResult, len(allPrefixes))
	errorCh := make(chan error, len(allPrefixes))
	wg := sync.WaitGroup{}

	for _, prefix := range allPrefixes.Sorted() {
		if prefix != "" && !isPrefixRouter {
			continue
		}

		newRoutesForPrefix := newPrefixMap[prefix]
		oldRoutesForPrefix := oldPrefixMap[prefix]
		prefix := prefix

		wg.Add(1)
		go func() {
			defer wg.Done()
			prefixResult, prefixErr := syncRoutePrefix(ctx, o, r, prefix, newRoutesForPrefix, oldRoutesForPrefix)
			if prefixErr == nil {
				resultCh <- *prefixResult
			} else {
				errorCh <- prefixErr
			}
		}()
	}
	wg.Wait()
	close(errorCh)
	close(resultCh)

	var multiErr errors.MultiError
	for err = range errorCh {
		multiErr.Add(err)
	}
	if multiErr.Len() > 0 {
		return nil, multiErr.ToError()
	}

	var result RebuildRoutesResult
	for v := range resultCh {
		result.PrefixResults = append(result.PrefixResults, v)
	}

	sort.Slice(result.PrefixResults, func(i, j int) bool {
		return result.PrefixResults[i].Prefix < result.PrefixResults[j].Prefix
	})
	return &result, nil
}

func syncRoutePrefix(ctx context.Context, o RebuildRoutesOpts, r router.Router, prefix string, newRoutesForPrefix, oldRoutesForPrefix appTypes.RoutableAddresses) (*RebuildPrefixResult, error) {
	prefixRouter, _ := r.(router.PrefixRouter)
	var asyncR router.AsyncRouter
	if !o.Wait {
		asyncR, _ = r.(router.AsyncRouter)
	}

	prefixResult := &RebuildPrefixResult{
		Prefix: prefix,
	}

	toAdd, toRemove := diffRoutes(oldRoutesForPrefix.Addresses, newRoutesForPrefix.Addresses)
	for _, toAddURL := range toAdd {
		prefixResult.Added = append(prefixResult.Added, toAddURL.String())
	}
	for _, toRemoveURL := range toRemove {
		prefixResult.Removed = append(prefixResult.Removed, toRemoveURL.String())
	}
	sort.Strings(prefixResult.Added)
	sort.Strings(prefixResult.Removed)

	if o.Dry {
		log.Debugf("[rebuild-routes] nothing to do. DRY mode for app: %q", o.App.GetName())
		return prefixResult, nil
	}

	var prefixMsg string
	if prefix != "" {
		prefixMsg = fmt.Sprintf(" for prefix %q", prefix+".")
	}

	var err error

	fmt.Fprintf(o.Writer, " ---> Updating routes%s: %d added, %d removed\n", prefixMsg, len(toAdd), len(toRemove))
	if prefixRouter != nil {
		newRoutesForPrefix.Addresses = toAdd
		err = prefixRouter.AddRoutesPrefix(ctx, o.App, newRoutesForPrefix, o.Wait)
	} else if asyncR == nil {
		err = r.AddRoutes(ctx, o.App, toAdd)
	} else {
		err = asyncR.AddRoutesAsync(ctx, o.App, toAdd)
	}
	if err != nil {
		return nil, err
	}

	if prefixRouter != nil {
		oldRoutesForPrefix.Addresses = toRemove
		err = prefixRouter.RemoveRoutesPrefix(ctx, o.App, oldRoutesForPrefix, o.Wait)
	} else if asyncR == nil {
		err = r.RemoveRoutes(ctx, o.App, toRemove)
	} else {
		err = asyncR.RemoveRoutesAsync(ctx, o.App, toRemove)
	}
	if err != nil {
		return nil, err
	}
	log.Debugf("[rebuild-routes] routes added for app %q, prefix %q: %s", o.App.GetName(), prefix, strings.Join(prefixResult.Added, ", "))
	log.Debugf("[rebuild-routes] routes removed for app %q, prefix %q: %s", o.App.GetName(), prefix, strings.Join(prefixResult.Removed, ", "))
	fmt.Fprintf(o.Writer, " ---> Done updating routes%s: %d added, %d removed\n", prefixMsg, len(toAdd), len(toRemove))

	return prefixResult, nil
}
