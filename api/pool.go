// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/tsuru/tsuru/auth"
	terrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/rec"
	"gopkg.in/mgo.v2/bson"
)

func poolList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "pool-list")
	teams := []string{}
	contexts := permission.ContextsForPermission(t, permission.PermAppCreate)
	for _, c := range contexts {
		if c.CtxType == permission.CtxGlobal {
			teams = nil
			break
		}
		if c.CtxType != permission.CtxTeam {
			continue
		}
		teams = append(teams, c.Value)
	}
	query := []bson.M{{"public": true}, {"default": true}}
	if teams == nil {
		filter := bson.M{"default": false, "public": false}
		query = append(query, filter)
	}
	if teams != nil && len(teams) > 0 {
		filter := bson.M{
			"default": false,
			"public":  false,
			"teams":   bson.M{"$in": teams},
		}
		query = append(query, filter)
	}
	pools, err := provision.ListPools(bson.M{"$or": query})
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(pools)
}

func addPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPoolCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	public, _ := strconv.ParseBool(r.FormValue("public"))
	isDefault, _ := strconv.ParseBool(r.FormValue("default"))
	force, _ := strconv.ParseBool(r.FormValue("force"))
	p := provision.AddPoolOptions{
		Name:    r.FormValue("name"),
		Public:  public,
		Default: isDefault,
		Force:   force,
	}
	err := provision.AddPool(p)
	if err == provision.ErrDefaultPoolAlreadyExists {
		return &terrors.HTTP{
			Code:    http.StatusConflict,
			Message: err.Error(),
		}
	}
	if err == provision.ErrPoolNameIsRequired {
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

func removePoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPoolDelete)
	if !allowed {
		return permission.ErrUnauthorized
	}
	return provision.RemovePool(r.URL.Query().Get(":name"))
}

func addTeamToPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPoolUpdate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	err := r.ParseForm()
	if err != nil {
		msg := "You must provide the team."
		return &terrors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	pool := r.URL.Query().Get(":name")
	return provision.AddTeamsToPool(pool, r.Form["team"])
}

func removeTeamToPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPoolUpdate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	pool := r.URL.Query().Get(":name")
	teams := r.URL.Query()["teams"]
	return provision.RemoveTeamsFromPool(pool, teams)
}

func poolUpdateHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPoolUpdate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	query := bson.M{}
	if v := r.FormValue("default"); v != "" {
		d, _ := strconv.ParseBool(v)
		query["default"] = d
	}
	if v := r.FormValue("public"); v != "" {
		public, _ := strconv.ParseBool(v)
		query["public"] = public
	}
	poolName := r.URL.Query().Get(":name")
	forceDefault, _ := strconv.ParseBool(r.FormValue("force"))
	err := provision.PoolUpdate(poolName, query, forceDefault)
	if err == provision.ErrDefaultPoolAlreadyExists {
		return &terrors.HTTP{
			Code:    http.StatusConflict,
			Message: err.Error(),
		}
	}
	return err
}
