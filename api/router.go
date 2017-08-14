// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router"
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
	routers, err := router.List()
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
