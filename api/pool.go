// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/tsuru/tsuru/auth"
	terrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/rec"
	"gopkg.in/mgo.v2/bson"
)

type PoolsByTeam struct {
	Team  string
	Pools []string
}

func listPoolsToUser(w http.ResponseWriter, r *http.Request, t auth.Token) error {
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
	var filter bson.M
	if teams != nil {
		filter = bson.M{"teams": bson.M{"$in": teams}}
	}
	pools, err := provision.ListPools(filter)
	if err != nil {
		return err
	}
	poolsByTeam := []PoolsByTeam{}
	poolsByTeamMap := map[string]*PoolsByTeam{}
	for _, p := range pools {
		for _, t := range p.Teams {
			if poolsByTeamMap[t] == nil {
				poolsByTeam = append(poolsByTeam, PoolsByTeam{Team: t})
				poolsByTeamMap[t] = &poolsByTeam[len(poolsByTeam)-1]
			}
			poolsByTeamMap[t].Pools = append(poolsByTeamMap[t].Pools, p.Name)
		}
	}
	publicPools, err := provision.ListPools(bson.M{"public": true})
	if err != nil {
		return err
	}
	allowedDefault := permission.Check(t, permission.PermPoolUpdate)
	defaultPool := []provision.Pool{}
	if allowedDefault {
		defaultPool, err = provision.ListPools(bson.M{"default": true})
		if err != nil {
			return err
		}
	}
	p := map[string]interface{}{
		"pools_by_team": poolsByTeam,
		"public_pools":  publicPools,
		"default_pool":  defaultPool,
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(p)
}

func addPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPoolCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var p provision.AddPoolOptions
	err = json.Unmarshal(b, &p)
	if err != nil {
		return err
	}
	forceAdd, _ := strconv.ParseBool(r.URL.Query().Get("force"))
	p.Force = forceAdd
	err = provision.AddPool(p)
	if err != nil {
		if err == provision.ErrDefaultPoolAlreadyExists {
			return &terrors.HTTP{
				Code:    http.StatusConflict,
				Message: "Default pool already exists.",
			}
		}
		return err
	}
	return nil
}

func removePoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPoolDelete)
	if !allowed {
		return permission.ErrUnauthorized
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var params map[string]string
	err = json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	return provision.RemovePool(params["pool"])
}

type teamsToPoolParams struct {
	Teams []string `json:"teams"`
}

func addTeamToPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPoolUpdate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var params teamsToPoolParams
	err = json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	pool := r.URL.Query().Get(":name")
	return provision.AddTeamsToPool(pool, params.Teams)
}

func removeTeamToPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPoolUpdate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var params teamsToPoolParams
	err = json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	pool := r.URL.Query().Get(":name")
	return provision.RemoveTeamsFromPool(pool, params.Teams)
}

func poolUpdateHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPoolUpdate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var params map[string]*bool
	err = json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	query := bson.M{}
	for k, v := range params {
		if v != nil {
			query[k] = *v
		}
	}
	poolName := r.URL.Query().Get(":name")
	forceDefault, _ := strconv.ParseBool(r.URL.Query().Get("force"))
	err = provision.PoolUpdate(poolName, query, forceDefault)
	if err != nil {
		if err == provision.ErrDefaultPoolAlreadyExists {
			return &terrors.HTTP{
				Code:    http.StatusPreconditionFailed,
				Message: "Default pool already exists.",
			}
		}
		return err
	}
	return nil
}
