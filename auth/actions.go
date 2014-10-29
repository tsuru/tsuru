// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import "github.com/tsuru/tsuru/action"

var addKeyInGandalfAction = action.Action{
	Name: "add-key-in-gandalf",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		key := ctx.Params[0].(*Key)
		u := ctx.Params[1].(*User)
		return nil, u.addKeyGandalf(key)
	},
	Backward: func(ctx action.BWContext) {
		key := ctx.Params[0].(*Key)
		u := ctx.Params[1].(*User)
		u.removeKeyGandalf(key)
	},
}

// addKeyInDatabaseAction adds a key to a user in the database.
// It expects a *auth.Key and a *auth.User from the executor.
var addKeyInDatabaseAction = action.Action{
	Name: "add-key-in-database",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		key := ctx.Params[0].(*Key)
		u := ctx.Params[1].(*User)
		return nil, u.addKeyDB(key)
	},
	Backward: func(ctx action.BWContext) {
		key := ctx.Params[0].(*Key)
		u := ctx.Params[1].(*User)
		u.RemoveKey(*key)
	},
}
