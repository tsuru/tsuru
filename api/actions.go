// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/auth"
)

// addKeyToUserAction creates a user in gandalf server.
// It expects a *auth.Key and a *auth.User from the executor.
var addKeyInGandalfAction = action.Action{
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

var addKeyInDatabaseAction = action.Action{
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
