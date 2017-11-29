// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router"
	appTypes "github.com/tsuru/tsuru/types/app"
)

// title: router list
// path: /routers
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
func listRouters(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	contexts := permission.ContextsForPermission(t, permission.PermAppCreate)
	var teams []string
	var global bool
contexts:
	for _, c := range contexts {
		switch c.CtxType {
		case permission.CtxGlobal:
			global = true
			break contexts
		case permission.CtxTeam:
			teams = append(teams, c.Value)
		}
	}
	routers, err := router.ListWithInfo()
	if err != nil {
		return err
	}
	filteredRouters := routers
	if !global {
		routersAllowed := make(map[string]struct{})
		filteredRouters = []router.PlanRouter{}
		pools, err := pool.ListPossiblePools(teams)
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
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var appRouter appTypes.AppRouter
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	dec.IgnoreCase(true)
	err = dec.DecodeValues(&appRouter, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	_, err = router.Get(appRouter.Name)
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

	p, err := pool.GetPoolByName(a.Pool)
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
		CustomData: event.FormToCustomData(r.Form),
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
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var appRouter appTypes.AppRouter
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	dec.IgnoreCase(true)
	err = dec.DecodeValues(&appRouter, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	routerName := r.URL.Query().Get(":router")
	appRouter.Name = routerName
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	_, err = router.Get(appRouter.Name)
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

	p, err := pool.GetPoolByName(a.Pool)
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
		CustomData: event.FormToCustomData(r.Form),
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
	r.ParseForm()
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
		CustomData: event.FormToCustomData(r.Form),
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
