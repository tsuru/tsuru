// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func (s *S) TestRegister(c *check.C) {
	mngr := nopManager{}
	Register("nope", mngr)
	defer func() {
		delete(managers, "nope")
	}()
	c.Assert(managers["nope"], check.Equals, mngr)
}

func (s *S) TestRegisterOnNilMap(c *check.C) {
	oldManagers := managers
	managers = nil
	defer func() {
		managers = oldManagers
	}()
	mngr := nopManager{}
	Register("nope", mngr)
	c.Assert(managers["nope"], check.Equals, mngr)
}

func (s *S) TestManager(c *check.C) {
	mngr := nopManager{}
	Register("nope", mngr)
	config.Set("repo-manager", "nope")
	defer config.Unset("repo-manager")
	current := Manager()
	c.Assert(current, check.Equals, mngr)
}

func (s *S) TestManagerUnconfigured(c *check.C) {
	mngr := nopManager{}
	Register("nope", mngr)
	gandalf := nopManager{}
	Register("gandalf", gandalf)
	config.Unset("repo-manager")
	current := Manager()
	c.Assert(current, check.Equals, gandalf)
}

func (s *S) TestManagerUnknown(c *check.C) {
	config.Set("repo-manager", "something")
	defer config.Unset("repo-manager")
	current := Manager()
	c.Assert(current, check.FitsTypeOf, nopManager{})
}
