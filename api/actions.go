// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/auth"
)

var addUserToTeamInGandalfAction = action.Action{
	Name: "add-user-to-team-in-gandalf",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		u := ctx.Params[0].(*auth.User)
		t := ctx.Params[1].(*auth.Team)
		return nil, addUserToTeamInGandalf(u, t)
	},
	Backward: func(ctx action.BWContext) {
		u := ctx.Params[0].(*auth.User)
		team := ctx.Params[1].(*auth.Team)
		removeUserFromTeamInGandalf(u, team)
	},
}

var addUserToTeamInDatabaseAction = action.Action{
	Name: "add-user-to-team-in-database",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		u := ctx.Params[0].(*auth.User)
		t := ctx.Params[1].(*auth.Team)
		return nil, addUserToTeamInDatabase(u, t)
	},
	Backward: func(ctx action.BWContext) {
		u := ctx.Params[0].(*auth.User)
		t := ctx.Params[1].(*auth.Team)
		removeUserFromTeamInDatabase(u, t)
	},
}
