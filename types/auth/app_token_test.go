// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"gopkg.in/check.v1"
)

func (s *S) TestAppTokenAddRole(c *check.C) {
	appToken := &AppToken{AppName: "appname"}
	appToken.AddRole("app.update")
	c.Assert(appToken.Roles, check.DeepEquals, []string{"app.update"})
	appToken.AddRole("app.deploy")
	c.Assert(appToken.Roles, check.DeepEquals, []string{"app.update", "app.deploy"})
}

func (s *S) TestAppTokenAddRoleNoDuplicates(c *check.C) {
	appToken := &AppToken{AppName: "appname"}
	appToken.AddRole("app.delete")
	c.Assert(appToken.Roles, check.DeepEquals, []string{"app.delete"})
	appToken.AddRole("app.delete")
	c.Assert(appToken.Roles, check.DeepEquals, []string{"app.delete"})
}
