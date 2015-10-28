// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"gopkg.in/check.v1"
)

func (s *S) TestPermissionSchemeFullName(c *check.C) {
	table := []struct {
		p      permissionScheme
		result string
	}{
		{permissionScheme{}, ""},
		{permissionScheme{name: "app"}, "app"},
		{permissionScheme{name: "app", parent: &permissionScheme{}}, "app"},
		{permissionScheme{name: "env", parent: &permissionScheme{name: "app"}}, "app.env"},
		{permissionScheme{name: "set", parent: &permissionScheme{name: "en-nv", parent: &permissionScheme{name: "app"}}}, "app.en-nv.set"},
	}
	for _, el := range table {
		c.Check(el.p.FullName(), check.Equals, el.result)
	}
}

func (s *S) TestPermissionSchemeIdentifier(c *check.C) {
	table := []struct {
		p      permissionScheme
		result string
	}{
		{permissionScheme{}, "All"},
		{permissionScheme{name: "app"}, "App"},
		{permissionScheme{name: "app", parent: &permissionScheme{}}, "App"},
		{permissionScheme{name: "env", parent: &permissionScheme{name: "app"}}, "AppEnv"},
		{permissionScheme{name: "set", parent: &permissionScheme{name: "en-nv", parent: &permissionScheme{name: "app"}}}, "AppEnNvSet"},
	}
	for _, el := range table {
		c.Check(el.p.Identifier(), check.Equals, el.result)
	}
}

func (s *S) TestPermissionSchemeAllowedContexts(c *check.C) {
	table := []struct {
		p   permissionScheme
		ctx []contextType
	}{
		{permissionScheme{}, []contextType{CtxGlobal}},
		{permissionScheme{contexts: []contextType{CtxApp}}, []contextType{CtxGlobal, CtxApp}},
		{permissionScheme{parent: &permissionScheme{contexts: []contextType{CtxApp}}}, []contextType{CtxGlobal, CtxApp}},
		{permissionScheme{contexts: []contextType{}, parent: &permissionScheme{contexts: []contextType{CtxApp}}}, []contextType{CtxGlobal}},
		{permissionScheme{contexts: []contextType{CtxTeam}, parent: &permissionScheme{contexts: []contextType{CtxApp}}}, []contextType{CtxGlobal, CtxTeam}},
	}
	for _, el := range table {
		c.Check(el.p.AllowedContexts(), check.DeepEquals, el.ctx)
	}
}

type userToken struct {
	permissions []Permission
}

func (t *userToken) Permissions() ([]Permission, error) {
	return t.permissions, nil
}

func (s *S) TestCheck(c *check.C) {
	t := &userToken{
		permissions: []Permission{
			{Scheme: PermAppUpdate, Context: permissionContext{CtxType: CtxTeam, Value: "team1"}},
			{Scheme: PermAppDeploy, Context: permissionContext{CtxType: CtxTeam, Value: "team3"}},
			{Scheme: PermAppUpdateEnvUnset, Context: permissionContext{CtxType: CtxGlobal}},
		},
	}
	c.Assert(Check(t, PermAppUpdateEnvSet, permissionContext{CtxType: CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(t, PermAppUpdate, permissionContext{CtxType: CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(t, PermAppDeploy, permissionContext{CtxType: CtxTeam, Value: "team1"}), check.Equals, false)
	c.Assert(Check(t, PermAppDeploy, permissionContext{CtxType: CtxTeam, Value: "team3"}), check.Equals, true)
	c.Assert(Check(t, PermAppUpdate, permissionContext{CtxType: CtxTeam, Value: "team2"}), check.Equals, false)
	c.Assert(Check(t, PermAppUpdateEnvUnset, permissionContext{CtxType: CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(t, PermAppUpdateEnvUnset, permissionContext{CtxType: CtxTeam, Value: "team10"}), check.Equals, true)
	c.Assert(Check(t, PermAppUpdateEnvUnset), check.Equals, true)
}

func (s *S) TestCheckSuperToken(c *check.C) {
	t := &userToken{
		permissions: []Permission{
			{Scheme: PermAll, Context: permissionContext{CtxType: CtxGlobal}},
		},
	}
	c.Assert(Check(t, PermAppDeploy, permissionContext{CtxType: CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(t, PermAppUpdateEnvUnset), check.Equals, true)
}
