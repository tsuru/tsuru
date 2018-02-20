// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
)

func getTeamsPermissions(t auth.Token) (map[string][]string, error) {
	permsForTeam := permission.PermissionRegistry.PermissionsWithContextType(permission.CtxTeam)
	teams, err := auth.ListTeams()
	if err != nil {
		return nil, err
	}
	teamsMap := map[string][]string{}
	perms, err := t.Permissions()
	if err != nil {
		return nil, err
	}
	for _, team := range teams {
		teamCtx := permission.Context(permission.CtxTeam, team.Name)
		var parent *permission.PermissionScheme
		for _, p := range permsForTeam {
			if parent != nil && parent.IsParent(p) {
				continue
			}
			if permission.CheckFromPermList(perms, p, teamCtx) {
				parent = p
				teamsMap[team.Name] = append(teamsMap[team.Name], p.FullName())
			}
		}
	}
	return teamsMap, nil
}

// title: team token list
// path: /teamtokens
// method: GET
// produce: application/json
// responses:
//   200: List team tokens
//   204: No content
//   401: Unauthorized
func teamTokenList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	tm, err := getTeamsPermissions(t)
	if err != nil {
		return err
	}
	if len(tm) == 0 {
		return permission.ErrUnauthorized
	}
	teamNames := make([]string, len(tm))
	i := 0
	for name := range tm {
		teamNames[i] = name
		i++
	}
	canRead := permission.Check(t, permission.PermTeamTokenRead,
		permission.Contexts(permission.CtxTeam, teamNames)...,
	)
	if !canRead {
		return permission.ErrUnauthorized
	}

	teamTokens, err := auth.TeamTokenService().FindByTeams(teamNames)
	if err != nil {
		return err
	}
	if len(teamTokens) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(teamTokens)
}

// title: team token create
// path: /apps/{app}/tokens
// method: POST
// produce: application/json
// responses:
//   201: App token created
//   401: Unauthorized
//   409: App token already exists
func appTokenCreate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	appName := r.URL.Query().Get(":app")
	app, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppTokenCreate,
		contextsForApp(&app)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	appToken := authTypes.NewTeamToken(appName, t.GetUserName())
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppTokenCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&app)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	err = auth.TeamTokenService().Insert(appToken)
	if err != nil {
		if err == authTypes.ErrTeamTokenAlreadyExists {
			w.WriteHeader(http.StatusConflict)
		}
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(appToken)
}

// title: team token delete
// path: /apps/{app}/tokens/{token}
// method: DELETE
// produce: application/json
// responses:
//   200: App token created
//   401: Unauthorized
//   404: App token not found
func appTokenDelete(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	appName := r.URL.Query().Get(":app")
	app, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppTokenDelete,
		contextsForApp(&app)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppTokenDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&app)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	token := r.URL.Query().Get(":token")
	appToken := authTypes.TeamToken{Token: token}
	err = auth.TeamTokenService().Delete(appToken)
	if err == authTypes.ErrTeamTokenNotFound {
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}
