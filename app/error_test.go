// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"

	"gopkg.in/check.v1"
)

func (s *S) TestAppCreationError(c *check.C) {
	e := AppCreationError{app: "myapp", Err: errors.New("failure in app")}
	expected := `tsuru failed to create the app "myapp": failure in app`
	c.Assert(e.Error(), check.Equals, expected)
}

func (s *S) TestNoTeamsError(c *check.C) {
	e := NoTeamsError{}
	c.Assert(e.Error(), check.Equals, "Cannot create app without teams.")
}

func (s *S) TestManyTeamsError(c *check.C) {
	e := ManyTeamsError{}
	c.Assert(e.Error(), check.Equals, "You belong to more than one team, choose one to be owner for this app.")
}
