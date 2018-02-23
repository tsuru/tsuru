// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router/rebuild"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

func (s *S) TestRebuildRoutesPoolApps(c *check.C) {
	var rebuildApps []string
	err := rebuild.RegisterTask(func(appName string) (rebuild.RebuildApp, error) {
		rebuildApps = append(rebuildApps, appName)
		return nil, nil
	})
	c.Assert(err, check.IsNil)
	provision.DefaultProvisioner = "fake"
	app.AuthScheme = auth.ManagedScheme(native.NativeScheme{})
	u, _ := permissiontest.CustomUserWithPermission(c, app.AuthScheme, "majortom", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	err = pool.AddPool(pool.AddPoolOptions{
		Name: "p1",
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(pool.AddPoolOptions{
		Name: "p2",
	})
	c.Assert(err, check.IsNil)
	user := authTypes.User(*u)
	err = auth.TeamService().Create("myteam", &user)
	c.Assert(err, check.IsNil)
	err = app.CreateApp(&app.App{Name: "myapp1", TeamOwner: "myteam", Pool: "p1"}, u)
	c.Assert(err, check.IsNil)
	err = app.CreateApp(&app.App{Name: "myapp2", TeamOwner: "myteam", Pool: "p2"}, u)
	c.Assert(err, check.IsNil)
	RebuildRoutesPoolApps("p1")
	c.Assert(rebuildApps, check.DeepEquals, []string{"myapp1"})
}
