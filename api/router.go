// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	permTypes "github.com/tsuru/tsuru/types/permission"
	routerTypes "github.com/tsuru/tsuru/types/router"
)

// title: router add
// path: /routers
// method: POST
// responses:
//   201: Created
//   400: Invalid router
//   409: Router already exists
func addRouter(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var dynamicRouter routerTypes.DynamicRouter
	err = ParseInput(r, &dynamicRouter)
	if err != nil {
		return err
	}

	allowed := permission.Check(t, permission.PermRouterCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}

	_, err = servicemanager.DynamicRouter.Get(ctx, dynamicRouter.Name)
	if err == nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: "dynamic router already exists"}
	}

	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRouter, Value: dynamicRouter.Name},
		Kind:       permission.PermRouterCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRouterReadEvents, permTypes.PermissionContext{CtxType: permTypes.CtxRouter, Value: dynamicRouter.Name}),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = servicemanager.DynamicRouter.Create(ctx, dynamicRouter)
	if err == nil {
		w.WriteHeader(http.StatusCreated)
	}
	return err
}

// title: router update
// path: /routers/{name}
// method: PUT
// responses:
//   200: OK
//   400: Invalid router
//   404: Router not found
func updateRouter(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var dynamicRouter routerTypes.DynamicRouter

	routerName := r.URL.Query().Get(":name")
	err = ParseInput(r, &dynamicRouter)
	if err != nil {
		return err
	}
	dynamicRouter.Name = routerName

	allowed := permission.Check(t, permission.PermRouterUpdate, permTypes.PermissionContext{CtxType: permTypes.CtxRouter, Value: dynamicRouter.Name})
	if !allowed {
		return permission.ErrUnauthorized
	}

	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRouter, Value: dynamicRouter.Name},
		Kind:       permission.PermRouterUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRouterReadEvents, permTypes.PermissionContext{CtxType: permTypes.CtxRouter, Value: dynamicRouter.Name}),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	err = servicemanager.DynamicRouter.Update(ctx, dynamicRouter)
	if err != nil {
		if err == routerTypes.ErrDynamicRouterNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	return nil
}

// title: router delete
// path: /routers/{name}
// method: DELETE
// responses:
//   200: OK
//   404: Router not found
func deleteRouter(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	routerName := r.URL.Query().Get(":name")

	allowed := permission.Check(t, permission.PermRouterDelete, permTypes.PermissionContext{CtxType: permTypes.CtxRouter, Value: routerName})
	if !allowed {
		return permission.ErrUnauthorized
	}

	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeRouter, Value: routerName},
		Kind:       permission.PermRouterDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermRouterReadEvents, permTypes.PermissionContext{CtxType: permTypes.CtxRouter, Value: routerName}),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	err = servicemanager.DynamicRouter.Remove(ctx, routerName)
	if err != nil {
		if err == routerTypes.ErrDynamicRouterNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	return nil
}

// title: router list
// path: /routers
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
func listRouters(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	contexts := permission.ContextsForPermission(t, permission.PermAppCreate)
	var teams []string
	var global bool
contexts:
	for _, c := range contexts {
		switch c.CtxType {
		case permTypes.CtxGlobal:
			global = true
			break contexts
		case permTypes.CtxTeam:
			teams = append(teams, c.Value)
		}
	}
	routers, err := router.ListWithInfo(ctx)
	if err != nil {
		return err
	}
	filteredRouters := routers
	if !global {
		routersAllowed := make(map[string]struct{})
		filteredRouters = []router.PlanRouter{}
		pools, err := pool.ListPossiblePools(ctx, teams)
		if err != nil {
			return err
		}
		for _, p := range pools {
			rs, err := p.GetRouters()
			if err != nil {
				return err
			}
			for _, r := range rs {
				routersAllowed[r] = struct{}{}
			}
		}
		for _, r := range routers {
			if _, ok := routersAllowed[r.Name]; ok {
				filteredRouters = append(filteredRouters, r)
			}
		}
	}
	isRouterCreator := permission.Check(t, permission.PermRouterCreate)
	if !isRouterCreator {
		for i := range filteredRouters {
			filteredRouters[i].Config = nil
		}
	}
	if len(filteredRouters) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(filteredRouters)
}

// title: add app router
// path: /app/{app}/routers
// method: POST
// produce: application/json
// responses:
//   200: OK
//   404: App or router not found
//   400: Invalid request
func addAppRouter(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var appRouter appTypes.AppRouter
	err = ParseInput(r, &appRouter)
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	_, err = router.Get(ctx, appRouter.Name)
	if err != nil {
		if _, isNotFound := err.(*router.ErrRouterNotFound); isNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateRouterAdd,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	p, err := pool.GetPoolByName(ctx, a.Pool)
	if err != nil {
		return err
	}
	err = p.ValidateRouters([]appTypes.AppRouter{appRouter})
	if err != nil {
		if err == pool.ErrPoolHasNoRouter {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
		return err
	}

	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateRouterAdd,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return a.AddRouter(appRouter)
}

// title: update app router
// path: /app/{app}/routers/{name}
// method: PUT
// produce: application/json
// responses:
//   200: OK
//   404: App or router not found
//   400: Invalid request
func updateAppRouter(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var appRouter appTypes.AppRouter
	err = ParseInput(r, &appRouter)
	if err != nil {
		return err
	}
	routerName := r.URL.Query().Get(":router")
	appRouter.Name = routerName
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	_, err = router.Get(ctx, appRouter.Name)
	if err != nil {
		if _, isNotFound := err.(*router.ErrRouterNotFound); isNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateRouterUpdate,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	p, err := pool.GetPoolByName(ctx, a.Pool)
	if err != nil {
		return err
	}
	err = p.ValidateRouters([]appTypes.AppRouter{appRouter})
	if err != nil {
		if err == pool.ErrPoolHasNoRouter {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
		return err
	}

	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateRouterUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return a.UpdateRouter(appRouter)
}

// title: delete app router
// path: /app/{app}/routers/{router}
// method: DELETE
// produce: application/json
// responses:
//   200: OK
//   404: App or router not found
func removeAppRouter(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	appName := r.URL.Query().Get(":app")
	routerName := r.URL.Query().Get(":router")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateRouterRemove,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateRouterRemove,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = a.RemoveRouter(routerName)
	if _, isNotFound := err.(*router.ErrRouterNotFound); isNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return err
}

// title: list app routers
// path: /app/{app}/routers
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
//   404: App not found
func listAppRouters(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	canRead := permission.Check(t, permission.PermAppReadRouter,
		contextsForApp(&a)...,
	)
	if !canRead {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	routers, err := a.GetRoutersWithAddr()
	if err != nil {
		return err
	}
	if len(routers) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return json.NewEncoder(w).Encode(routers)
}

type setRoutableRequest struct {
	IsRoutable bool   `json:"isRoutable"`
	Version    string `json:"version"`
}

// title: toggle an app version as routable
// path: /app/{app}/routable
// method: POST
// responses:
//   200: OK
//   400: Bad request
//   401: Not authorized
//   404: App not found
func appSetRoutable(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var args setRoutableRequest
	err = ParseInput(r, &args)
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppUpdateRoutable,
		contextsForApp(&a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateRoutable,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	version, err := servicemanager.AppVersion.VersionByImageOrVersion(&a, args.Version)
	if err != nil {
		if appTypes.IsInvalidVersionError(err) {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
		return err
	}
	return a.SetRoutable(ctx, version, args.IsRoutable)
}
