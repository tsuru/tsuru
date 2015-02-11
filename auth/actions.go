// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import "github.com/tsuru/tsuru/action"

var addKeyInRepositoryAction = action.Action{
	Name: "add-key-in-repository",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		key := ctx.Params[0].(*Key)
		u := ctx.Params[1].(*User)
		return nil, u.addKeyRepository(key)
	},
	Backward: func(ctx action.BWContext) {
		key := ctx.Params[0].(*Key)
		u := ctx.Params[1].(*User)
		u.removeKeyRepository(key)
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
		err := u.RemoveKey(*key)
		if err != nil {
			println(err.Error())
		}
	},
}
