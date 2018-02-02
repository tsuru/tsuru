// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"sort"

	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

func (s *S) TestAppTokenAuth(c *check.C) {
	appToken := authTypes.AppToken{Token: "abcdef"}
	err := AppTokenService().Insert(appToken)
	c.Assert(err, check.IsNil)
	t, err := AppTokenAuth(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.GetValue(), check.Equals, appToken.Token)
}

func (s *S) TestAppTokenAuthNotFound(c *check.C) {
	t, err := AppTokenAuth("bearer invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, ErrInvalidToken)
}

func (s *S) TestAppTokenPermissions(c *check.C) {
	r1, err := permission.NewRole("app-deployer", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions("app.read", "app.deploy")
	c.Assert(err, check.IsNil)
	r2, err := permission.NewRole("app-updater", "app", "")
	c.Assert(err, check.IsNil)
	err = r2.AddPermissions("app.update")
	c.Assert(err, check.IsNil)

	appToken := &AppToken{AppName: "myapp", Roles: []string{"app-deployer", "app-updater"}}
	perms, err := appToken.Permissions()
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.HasLen, 3)
	sort.Slice(perms, func(i, j int) bool { return perms[i].Scheme.FullName() < perms[j].Scheme.FullName() })
	c.Assert(perms, check.DeepEquals, []permission.Permission{
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permission.CtxApp, "myapp")},
		{Scheme: permission.PermAppRead, Context: permission.Context(permission.CtxApp, "myapp")},
		{Scheme: permission.PermAppUpdate, Context: permission.Context(permission.CtxApp, "myapp")},
	})
}

func (s *S) TestAppTokenPermissionsWithInvalidPermission(c *check.C) {
	r1, err := permission.NewRole("pool-reader", "pool", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions("pool.read")
	c.Assert(err, check.IsNil)

	appToken := &AppToken{AppName: "myapp", Roles: []string{"pool-reader"}}
	_, err = appToken.Permissions()
	c.Assert(err, check.NotNil)
}
