// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
)

func (s *S) TestInsertAppForward(c *C) {
	app := App{Name: "conviction", Framework: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	a, ok := r.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	c.Assert(a.Framework, Equals, app.Framework)
	err = app.Get()
	c.Assert(err, IsNil)
}

func (s *S) TestInsertAppForwardAppPointer(c *C) {
	app := App{Name: "conviction", Framework: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{&app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name})
	a, ok := r.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	c.Assert(a.Framework, Equals, app.Framework)
	err = app.Get()
	c.Assert(err, IsNil)
}

func (s *S) TestInsertAppForwardInvalidValue(c *C) {
	ctx := action.FWContext{
		Params: []interface{}{"hello"},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(r, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "First parameter must be App or *App.")
}

func (s *S) TestInsertAppBackward(c *C) {
	app := App{Name: "conviction", Framework: "evergrey"}
	ctx := action.BWContext{
		Params:   []interface{}{app},
		FWResult: &app,
	}
	err := db.Session.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": app.Name}) // sanity
	insertApp.Backward(ctx)
	n, err := db.Session.Apps().Find(bson.M{"name": app.Name}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 0)
}

func (s *S) TestInsertAppMinimumParams(c *C) {
	c.Assert(insertApp.MinParams, Equals, 1)
}
