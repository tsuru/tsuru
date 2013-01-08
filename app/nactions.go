// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
)

// insertApp is an action that inserts an app in the database in Forward and
// removes it in the Backward.
//
// The first argument in the context must be an App or a pointer to an App.
var insertApp = &action.Action{
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app App
		switch ctx.Params[0].(type) {
		case App:
			app = ctx.Params[0].(App)
		case *App:
			app = *ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be App or *App.")
		}
		err := db.Session.Apps().Insert(app)
		return &app, err
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		db.Session.Apps().Remove(bson.M{"name": app.Name})
	},
	MinParams: 1,
}
