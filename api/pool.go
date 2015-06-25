// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"gopkg.in/mgo.v2/bson"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/rec"
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
	teams, err := u.Teams()
	if err != nil {
		return err
	}
	var poolsByTeam []PoolsByTeam
	for _, t := range teams {
		pools, err := provision.ListPools(bson.M{"teams": t.Name})
		if err != nil {
			return err
		}
		pbt := PoolsByTeam{Team: t.Name, Pools: provision.GetPoolsNames(pools)}
		poolsByTeam = append(poolsByTeam, pbt)
	}
	publicPools, err := provision.ListPools(bson.M{"public": true})
	if err != nil {
		return err
	}
	p := map[string]interface{}{
		"pools_by_team": poolsByTeam,
		"public_pools":  publicPools,
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(p)
}

type addPoolOptions struct {
	Name   string
	Public bool
}

func addPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var p addPoolOptions
	err = json.Unmarshal(b, &p)
	if err != nil {
		return err
	}
	return provision.AddPool(p.Name, p.Public)
}

func removePoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
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

func listPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	pools, err := provision.ListPools(nil)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(pools)
}

type teamsToPoolParams struct {
	Pool  string   `json:"pool"`
	Teams []string `json:"teams"`
}

func addTeamToPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var params teamsToPoolParams
	err = json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	return provision.AddTeamsToPool(params.Pool, params.Teams)
}

func removeTeamToPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var params teamsToPoolParams
	err = json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	return provision.RemoveTeamsFromPool(params.Pool, params.Teams)
}

func poolUpdateHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var params provision.PoolUpdateOptions
	err = json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	params.Name = r.URL.Query().Get(":name")
	return provision.PoolUpdate(params)
}
