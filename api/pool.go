// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	terrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
)

// title: pool list
// path: /pools
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
//   401: Unauthorized
func poolList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var teams, poolNames []string
	isGlobal := false
	contexts := permission.ContextsForPermission(t, permission.PermAppCreate)
	contexts = append(contexts, permission.ContextsForPermission(t, permission.PermPoolRead)...)
	for _, c := range contexts {
		if c.CtxType == permission.CtxGlobal {
			isGlobal = true
			break
		}
		if c.CtxType == permission.CtxTeam {
			teams = append(teams, c.Value)
		}
		if c.CtxType == permission.CtxPool {
			poolNames = append(poolNames, c.Value)
		}
	}
	var pools []pool.Pool
	var err error
	if isGlobal {
		pools, err = pool.ListAllPools()
		if err != nil {
			return err
		}
	} else {
		pools, err = pool.ListPossiblePools(teams)
		if err != nil {
			return err
		}
		if len(poolNames) > 0 {
			namedPools, err := pool.ListPools(poolNames...)
			if err != nil {
				return err
			}
			pools = append(pools, namedPools...)
		}
	}
	poolsMap := make(map[string]struct{})
	var poolList []pool.Pool
	for _, p := range pools {
		if _, ok := poolsMap[p.Name]; ok {
			continue
		}
		poolList = append(poolList, p)
		poolsMap[p.Name] = struct{}{}
	}
	if len(poolList) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(poolList)
}

// title: pool create
// path: /pools
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   201: Pool created
//   400: Invalid data
//   401: Unauthorized
//   409: Pool already exists
func addPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowed := permission.Check(t, permission.PermPoolCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	var addOpts pool.AddPoolOptions
	err = r.ParseForm()
	if err == nil {
		err = dec.DecodeValues(&addOpts, r.Form)
	}
	if err != nil {
		return &terrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if addOpts.Name == "" {
		return &terrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: pool.ErrPoolNameIsRequired.Error(),
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: addOpts.Name},
		Kind:       permission.PermPoolCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, permission.Context(permission.CtxPool, addOpts.Name)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = pool.AddPool(addOpts)
	if err == pool.ErrDefaultPoolAlreadyExists || err == pool.ErrPoolAlreadyExists {
		return &terrors.HTTP{
			Code:    http.StatusConflict,
			Message: err.Error(),
		}
	}
	if err == pool.ErrPoolNameIsRequired {
		return &terrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if err == nil {
		w.WriteHeader(http.StatusCreated)
	}
	return err
}

// title: remove pool
// path: /pools/{name}
// method: DELETE
// responses:
//   200: Pool removed
//   401: Unauthorized
//   403: Pool still has apps
//   404: Pool not found
func removePoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	allowed := permission.Check(t, permission.PermPoolDelete)
	if !allowed {
		return permission.ErrUnauthorized
	}
	poolName := r.URL.Query().Get(":name")
	filter := &app.Filter{}
	filter.Pool = poolName
	apps, err := app.List(appFilterByContext([]permission.PermissionContext{}, filter))
	if err != nil {
		return err
	}
	if len(apps) > 0 {
		return &terrors.HTTP{Code: http.StatusForbidden, Message: "This pool has apps, you need to migrate or remove them before removing the pool"}
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: poolName},
		Kind:       permission.PermPoolDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, permission.Context(permission.CtxPool, poolName)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = pool.RemovePool(poolName)
	if err == pool.ErrPoolNotFound {
		return &terrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return err
}

// title: add team too pool
// path: /pools/{name}/team
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Pool updated
//   401: Unauthorized
//   400: Invalid data
//   404: Pool not found
func addTeamToPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	err = r.ParseForm()
	if err != nil {
		return &terrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	poolName := r.URL.Query().Get(":name")
	allowed := permission.Check(t, permission.PermPoolUpdateTeamAdd, permission.Context(permission.CtxPool, poolName))
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: poolName},
		Kind:       permission.PermPoolUpdateTeamAdd,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, permission.Context(permission.CtxPool, poolName)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	if teams, ok := r.Form["team"]; ok {
		err := pool.AddTeamsToPool(poolName, teams)
		if err == pool.ErrPoolNotFound {
			return &terrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	return &terrors.HTTP{Code: http.StatusBadRequest, Message: "You must provide the team."}
}

// title: remove team from pool
// path: /pools/{name}/team
// method: DELETE
// responses:
//   200: Pool updated
//   401: Unauthorized
//   400: Invalid data
//   404: Pool not found
func removeTeamToPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	poolName := r.URL.Query().Get(":name")
	allowed := permission.Check(t, permission.PermPoolUpdateTeamRemove, permission.Context(permission.CtxPool, poolName))
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: poolName},
		Kind:       permission.PermPoolUpdateTeamRemove,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, permission.Context(permission.CtxPool, poolName)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	if teams, ok := r.URL.Query()["team"]; ok {
		err := pool.RemoveTeamsFromPool(poolName, teams)
		if err == pool.ErrPoolNotFound {
			return &terrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	return &terrors.HTTP{
		Code:    http.StatusBadRequest,
		Message: "You must provide the team",
	}
}

// title: pool update
// path: /pools/{name}
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: Pool updated
//   401: Unauthorized
//   404: Pool not found
//   409: Default pool already defined
func poolUpdateHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	allowed := permission.Check(t, permission.PermPoolUpdate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	poolName := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: poolName},
		Kind:       permission.PermPoolUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, permission.Context(permission.CtxPool, poolName)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	var updateOpts pool.UpdatePoolOptions
	err = dec.DecodeValues(&updateOpts, r.Form)
	if err != nil {
		return &terrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	err = pool.PoolUpdate(poolName, updateOpts)
	if err == pool.ErrPoolNotFound {
		return &terrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if err == pool.ErrDefaultPoolAlreadyExists {
		return &terrors.HTTP{
			Code:    http.StatusConflict,
			Message: err.Error(),
		}
	}
	return err
}

// title: pool constraints list
// path: /constraints
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
//   401: Unauthorized
func poolConstraintList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermPoolReadConstraints) {
		return permission.ErrUnauthorized
	}
	constraints, err := pool.ListPoolsConstraints(nil)
	if err != nil {
		return err
	}
	if len(constraints) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(constraints)
}

// title: set a pool constraint
// path: /constraints
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: OK
//   401: Unauthorized
func poolConstraintSet(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermPoolUpdateConstraintsSet) {
		return permission.ErrUnauthorized
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	var poolConstraint pool.PoolConstraint
	err = r.ParseForm()
	if err == nil {
		err = dec.DecodeValues(&poolConstraint, r.Form)
	}
	if err != nil {
		return &terrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if poolConstraint.PoolExpr == "" {
		return &terrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "You must provide a Pool Expression",
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: poolConstraint.PoolExpr},
		Kind:       permission.PermPoolUpdateConstraintsSet,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	append := false
	if appendStr := r.FormValue("append"); appendStr != "" {
		append, _ = strconv.ParseBool(appendStr)
	}
	if append {
		return pool.AppendPoolConstraint(&poolConstraint)
	}
	return pool.SetPoolConstraint(&poolConstraint)
}
