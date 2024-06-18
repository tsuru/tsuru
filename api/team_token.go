// Copyright 2018 tsuru authors. All rights reserved.
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
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

// title: token list
// path: /tokens
// method: GET
// produce: application/json
// responses:
//
//	200: List tokens
//	204: No content
//	401: Unauthorized
func tokenList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	tokens, err := servicemanager.TeamToken.FindByUserToken(ctx, t)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(tokens)
}

// title: token info
// path: /tokens/{token_id}
// method: GET
// produce: application/json
// responses:
//
//	200: Get token
//	401: Unauthorized
func tokenInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	tokenID := r.URL.Query().Get(":token_id")
	if tokenID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return nil
	}

	teamToken, err := servicemanager.TeamToken.Info(ctx, tokenID, t)
	if err == authTypes.ErrTeamTokenNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	}
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermTeamTokenRead,
		permission.Context(permTypes.CtxTeam, teamToken.Team),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(teamToken)
}

// title: token create
// path: /tokens
// method: POST
// produce: application/json
// responses:
//
//	201: Token created
//	401: Unauthorized
//	409: Token already exists
func tokenCreate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var args authTypes.TeamTokenCreateArgs
	err = ParseInput(r, &args)
	if err != nil {
		return err
	}
	if args.Team == "" {
		args.Team, err = autoTeamOwner(ctx, t, permission.PermTeamTokenCreate)
		if err != nil {
			return err
		}
	}
	allowed := permission.Check(t, permission.PermTeamTokenCreate,
		permission.Context(permTypes.CtxTeam, args.Team),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     teamTarget(args.Team),
		Kind:       permission.PermTeamTokenCreate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermTeamReadEvents, permission.Context(permTypes.CtxTeam, args.Team)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	token, err := servicemanager.TeamToken.Create(ctx, args, t)
	if err != nil {
		return err
	}
	if err != nil {
		if err == authTypes.ErrTeamTokenAlreadyExists {
			return &errors.HTTP{
				Code:    http.StatusConflict,
				Message: err.Error(),
			}
		}
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(token)
}

// title: token update
// path: /tokens/{token_id}
// method: PUT
// produce: application/json
// responses:
//
//	200: Token updated
//	401: Unauthorized
//	404: Token not found
func tokenUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var args authTypes.TeamTokenUpdateArgs
	err = ParseInput(r, &args)
	if err != nil {
		return err
	}
	args.TokenID = r.URL.Query().Get(":token_id")
	teamToken, err := servicemanager.TeamToken.FindByTokenID(ctx, args.TokenID)
	if err != nil {
		if err == authTypes.ErrTeamTokenNotFound {
			return &errors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}
	allowed := permission.Check(t, permission.PermTeamTokenUpdate,
		permission.Context(permTypes.CtxTeam, teamToken.Team),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     teamTarget(teamToken.Team),
		Kind:       permission.PermTeamTokenUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermTeamReadEvents, permission.Context(permTypes.CtxTeam, teamToken.Team)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	teamToken, err = servicemanager.TeamToken.Update(ctx, args, t)
	if err == authTypes.ErrTeamTokenNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(teamToken)
}

// title: token delete
// path: /tokens/{token_id}
// method: DELETE
// produce: application/json
// responses:
//
//	200: Token created
//	401: Unauthorized
//	404: Token not found
func tokenDelete(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	tokenID := r.URL.Query().Get(":token_id")
	teamToken, err := servicemanager.TeamToken.FindByTokenID(ctx, tokenID)
	if err != nil {
		if err == authTypes.ErrTeamTokenNotFound {
			return &errors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}
	teamName := teamToken.Team
	allowed := permission.Check(t, permission.PermTeamTokenDelete,
		permission.Context(permTypes.CtxTeam, teamName),
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     teamTarget(teamName),
		Kind:       permission.PermTeamTokenDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermTeamReadEvents, permission.Context(permTypes.CtxTeam, teamName)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = servicemanager.TeamToken.Delete(ctx, tokenID)
	if err == authTypes.ErrTeamTokenNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	}
	return err
}
