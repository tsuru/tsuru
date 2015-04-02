// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/rec"
)

func listPoolsToUser(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "pool-list")
	pools, err := []string{}, nil //app.Provisioner.ListPoolToUser(u)
	if err != nil {
		return err
	}
	if len(pools) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(pools)
}

func addPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var params map[string]string
	err = json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	return provision.AddPool(params["pool"])
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
