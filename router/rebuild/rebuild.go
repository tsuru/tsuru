// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	pkgErrors "github.com/pkg/errors"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router"
	appTypes "github.com/tsuru/tsuru/types/app"
	routerTypes "github.com/tsuru/tsuru/types/router"
)

type RebuildApp interface {
	router.App
	GetCname() []string
	GetRouters() []appTypes.AppRouter
	GetHealthcheckData() (routerTypes.HealthcheckData, error)
	RoutableAddresses(context.Context) ([]appTypes.RoutableAddresses, error)
}

type RebuildRoutesOpts struct {
	App    RebuildApp
	Writer io.Writer
	Dry    bool
}

func RebuildRoutes(ctx context.Context, opts RebuildRoutesOpts) error {
	multi := errors.NewMultiError()
	writer := opts.Writer

	if writer == nil {
		opts.Writer = io.Discard
	}

	for _, appRouter := range opts.App.GetRouters() {
		err := RebuildRoutesInRouter(ctx, appRouter, opts)
		if err != nil {
			multi.Add(err)
		}
	}
	return multi.ToError()
}

func RebuildRoutesInRouter(ctx context.Context, appRouter appTypes.AppRouter, o RebuildRoutesOpts) error {
	log.Debugf("[rebuild-routes] rebuilding routes for app %q", o.App.GetName())
	if o.Writer == nil {
		o.Writer = io.Discard
	}
	fmt.Fprintf(o.Writer, "\n---- Updating router [%s] ----\n", appRouter.Name)
	r, err := router.Get(ctx, appRouter.Name)
	if err != nil {
		return err
	}

	routes, routesErr := o.App.RoutableAddresses(ctx)
	if routesErr != nil {
		return routesErr
	}
	hcData, errHc := o.App.GetHealthcheckData()
	if errHc != nil {
		return errHc
	}
	opts := router.EnsureBackendOpts{
		Opts:        map[string]interface{}{},
		Prefixes:    []router.BackendPrefix{},
		CNames:      o.App.GetCname(),
		Healthcheck: hcData,
	}
	for key, opt := range appRouter.Opts {
		opts.Opts[key] = opt
	}
	for _, route := range routes {
		opts.Prefixes = append(opts.Prefixes, router.BackendPrefix{
			Prefix: route.Prefix,
			Target: route.ExtraData,
		})
	}
	return r.EnsureBackend(ctx, o.App, opts)
}

type initializeFunc func(string) (RebuildApp, error)

var appFinder = atomic.Pointer[initializeFunc]{}

func Initialize(finder initializeFunc) {
	appFinder.Store(&finder)
}

func RebuildRoutesWithAppName(appName string, w io.Writer) error {
	finderPtr := appFinder.Load()
	if finderPtr == nil {
		log.Errorf("[routes-rebuild-task] rebuild is not initialized")
		return nil
	}
	finder := *finderPtr
	a, err := finder(appName)
	if err != nil {
		return pkgErrors.Wrapf(err, "error getting app %q", appName)
	}
	if a == nil {
		return pkgErrors.New("app not found")
	}

	return RebuildRoutes(context.TODO(), RebuildRoutesOpts{
		App:    a,
		Writer: w,
	})
}
