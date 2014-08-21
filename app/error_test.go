// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"

	"launchpad.net/gocheck"
)

func (s *S) TestAppCreationError(c *gocheck.C) {
	e := AppCreationError{app: "myapp", Err: errors.New("failure in app")}
	expected := `tsuru failed to create the app "myapp": failure in app`
	c.Assert(e.Error(), gocheck.Equals, expected)
}

func (s *S) TestNoTeamsError(c *gocheck.C) {
	e := NoTeamsError{}
	c.Assert(e.Error(), gocheck.Equals, "Cannot create app without teams.")
}

func (s *S) TestManyTeamsError(c *gocheck.C) {
	e := ManyTeamsError{}
	c.Assert(e.Error(), gocheck.Equals, "You belong to more than one team, choose one to be owner for this app.")
}
