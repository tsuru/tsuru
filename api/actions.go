// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/auth"
)

// addKeyToUserAction creates a user in gandalf server.
// It expects a *auth.Key and a *auth.User from the executor.
var addKeyInGandalfAction = action.Action{
	Name: "add-key-in-gandalf",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		key := ctx.Params[0].(*auth.Key)
		u := ctx.Params[1].(*auth.User)
		return nil, addKeyInGandalf(key, u)
	},
	Backward: func(ctx action.BWContext) {
		key := ctx.Params[0].(*auth.Key)
		u := ctx.Params[1].(*auth.User)
		removeKeyFromGandalf(key, u)
	},
}

// addKeyInDatabaseAction adds a key to a user in the database.
// It expects a *auth.Key and a *auth.User from the executor.
var addKeyInDatabaseAction = action.Action{
	Name: "add-key-in-database",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		key := ctx.Params[0].(*auth.Key)
		u := ctx.Params[1].(*auth.User)
		return nil, addKeyInDatabase(key, u)
	},
	Backward: func(ctx action.BWContext) {
		key := ctx.Params[0].(*auth.Key)
		u := ctx.Params[1].(*auth.User)
		removeKeyFromDatabase(key, u)
	},
}

var addUserToTeamInGandalfAction = action.Action{
	Name: "add-user-to-team-in-gandalf",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		email := ctx.Params[0].(string)
		u := ctx.Params[1].(*auth.User)
		t := ctx.Params[2].(*auth.Team)
		return nil, addUserToTeamInGandalf(email, u, t)
	},
	Backward: func(ctx action.BWContext) {
		email := ctx.Params[0].(string)
		u, err := auth.GetUserByEmail(email)
		if err != nil {
			return
		}
		t := ctx.Params[2].(*auth.Team)
		removeUserFromTeamInGandalf(u, t.Name)
	},
}

var addUserToTeamInDatabaseAction = action.Action{
	Name: "add-user-to-team-in-database",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		email := ctx.Params[0].(string)
		u, err := auth.GetUserByEmail(email)
		if err != nil {
			return nil, err
		}
		t := ctx.Params[2].(*auth.Team)
		return nil, addUserToTeamInDatabase(u, t)
	},
	Backward: func(ctx action.BWContext) {
		email := ctx.Params[0].(string)
		u, err := auth.GetUserByEmail(email)
		if err != nil {
			return
		}
		t := ctx.Params[2].(*auth.Team)
		removeUserFromTeamInDatabase(u, t)
	},
}
