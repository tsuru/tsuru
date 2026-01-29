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
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/streamfmt"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type RebuildRoutesOpts struct {
	App    *appTypes.App
	Writer io.Writer
	Dry    bool
}

func RebuildRoutes(ctx context.Context, opts RebuildRoutesOpts) error {
	multi := errors.NewMultiError()
	writer := opts.Writer

	if writer == nil {
		opts.Writer = io.Discard
	}

	for _, appRouter := range getAppRouters(opts.App) {
		err := RebuildRoutesInRouter(ctx, appRouter, opts)
		if err != nil {
			multi.Add(err)
		}
	}
	return multi.ToError()
}

func getAppRouters(app *appTypes.App) []appTypes.AppRouter {
	routers := append([]appTypes.AppRouter{}, app.Routers...)
	if app.Router != "" {
		for _, r := range routers {
			if r.Name == app.Router {
				return routers
			}
		}
		routers = append([]appTypes.AppRouter{{
			Name: app.Router,
			Opts: app.RouterOpts,
		}}, routers...)
	}
	return routers
}

func RebuildRoutesInRouter(ctx context.Context, appRouter appTypes.AppRouter, o RebuildRoutesOpts) error {
	log.Debugf("[rebuild-routes] rebuilding routes for app %q", o.App.Name)
	if o.Writer == nil {
		o.Writer = io.Discard
	}
	fmt.Fprint(o.Writer, "\n")
	streamfmt.FprintlnSectionf(o.Writer, "Updating router [%s]", appRouter.Name)
	r, err := router.Get(ctx, appRouter.Name)
	if err != nil {
		return err
	}

	provisioner, err := pool.GetProvisionerForPool(ctx, o.App.Pool)
	if err != nil {
		return err
	}
	routes, routesErr := provisioner.RoutableAddresses(ctx, o.App)
	if routesErr != nil {
		return routesErr
	}
	hcData, errHc := servicemanager.App.GetHealthcheckData(ctx, o.App)
	if errHc != nil {
		return errHc
	}
	opts := router.EnsureBackendOpts{
		Opts:        map[string]interface{}{},
		Prefixes:    []router.BackendPrefix{},
		Team:        o.App.TeamOwner,
		CertIssuers: o.App.CertIssuers,
		Tags:        o.App.Tags,
		CNames:      o.App.CName,
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

type initializeFunc func(string) (*appTypes.App, error)

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
