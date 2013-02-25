// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	. "launchpad.net/gocheck"
)

func (s *S) TestValidationError(c *C) {
	e := ValidationError{Message: "something"}
	c.Assert(e.Error(), Equals, "something")
}

func (s *S) TestAppCreationError(c *C) {
	e := appCreationError{app: "myapp", err: errors.New("failure in app")}
	expected := `Tsuru failed to create the app "myapp": failure in app`
	c.Assert(e.Error(), Equals, expected)
}

func (s *S) TestNoTeamsError(c *C) {
	e := NoTeamsError{}
	c.Assert(e.Error(), Equals, "Cannot create app without teams.")
}
