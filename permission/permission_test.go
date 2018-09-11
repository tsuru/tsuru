// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	permTypes "github.com/tsuru/tsuru/types/permission"
	"gopkg.in/check.v1"
)

func (s *S) TestPermissionSchemeFullName(c *check.C) {
	table := []struct {
		p      PermissionScheme
		result string
	}{
		{PermissionScheme{}, ""},
		{PermissionScheme{name: "app"}, "app"},
		{PermissionScheme{name: "app", parent: &PermissionScheme{}}, "app"},
		{PermissionScheme{name: "env", parent: &PermissionScheme{name: "app"}}, "app.env"},
		{PermissionScheme{name: "set", parent: &PermissionScheme{name: "en-nv", parent: &PermissionScheme{name: "app"}}}, "app.en-nv.set"},
	}
	for _, el := range table {
		c.Check(el.p.FullName(), check.Equals, el.result)
	}
}

func (s *S) TestPermissionSchemeIdentifier(c *check.C) {
	table := []struct {
		p      PermissionScheme
		result string
	}{
		{PermissionScheme{}, "All"},
		{PermissionScheme{name: "app"}, "App"},
		{PermissionScheme{name: "app", parent: &PermissionScheme{}}, "App"},
		{PermissionScheme{name: "env", parent: &PermissionScheme{name: "app"}}, "AppEnv"},
		{PermissionScheme{name: "set", parent: &PermissionScheme{name: "en-nv", parent: &PermissionScheme{name: "app"}}}, "AppEnNvSet"},
	}
	for _, el := range table {
		c.Check(el.p.Identifier(), check.Equals, el.result)
	}
}

func (s *S) TestPermissionSchemeAllowedContexts(c *check.C) {
	table := []struct {
		p   PermissionScheme
		ctx []permTypes.ContextType
	}{
		{PermissionScheme{}, []permTypes.ContextType{CtxGlobal}},
		{PermissionScheme{contexts: []permTypes.ContextType{CtxApp}}, []permTypes.ContextType{CtxGlobal, CtxApp}},
		{PermissionScheme{parent: &PermissionScheme{contexts: []permTypes.ContextType{CtxApp}}}, []permTypes.ContextType{CtxGlobal, CtxApp}},
		{PermissionScheme{contexts: []permTypes.ContextType{}, parent: &PermissionScheme{contexts: []permTypes.ContextType{CtxApp}}}, []permTypes.ContextType{CtxGlobal}},
		{PermissionScheme{contexts: []permTypes.ContextType{CtxTeam}, parent: &PermissionScheme{contexts: []permTypes.ContextType{CtxApp}}}, []permTypes.ContextType{CtxGlobal, CtxTeam}},
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
			{Scheme: PermAppUpdate, Context: permTypes.PermissionContext{CtxType: CtxTeam, Value: "team1"}},
			{Scheme: PermAppDeploy, Context: permTypes.PermissionContext{CtxType: CtxTeam, Value: "team3"}},
			{Scheme: PermAppUpdateEnvUnset, Context: permTypes.PermissionContext{CtxType: CtxGlobal}},
		},
	}
	c.Assert(Check(t, PermAppUpdateEnvSet, permTypes.PermissionContext{CtxType: CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(t, PermAppUpdate, permTypes.PermissionContext{CtxType: CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(t, PermAppDeploy, permTypes.PermissionContext{CtxType: CtxTeam, Value: "team1"}), check.Equals, false)
	c.Assert(Check(t, PermAppDeploy, permTypes.PermissionContext{CtxType: CtxTeam, Value: "team3"}), check.Equals, true)
	c.Assert(Check(t, PermAppUpdate, permTypes.PermissionContext{CtxType: CtxTeam, Value: "team2"}), check.Equals, false)
	c.Assert(Check(t, PermAppUpdateEnvUnset, permTypes.PermissionContext{CtxType: CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(t, PermAppUpdateEnvUnset, permTypes.PermissionContext{CtxType: CtxTeam, Value: "team10"}), check.Equals, true)
	c.Assert(Check(t, PermAppUpdateEnvUnset), check.Equals, true)
}

func (s *S) TestCheckSuperToken(c *check.C) {
	t := &userToken{
		permissions: []Permission{
			{Scheme: PermAll, Context: permTypes.PermissionContext{CtxType: CtxGlobal}},
		},
	}
	c.Assert(Check(t, PermAppDeploy, permTypes.PermissionContext{CtxType: CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(t, PermAppUpdateEnvUnset), check.Equals, true)
}

func (s *S) TestGetTeamForPermission(c *check.C) {
	t := &userToken{
		permissions: []Permission{
			{Scheme: PermAppUpdate, Context: permTypes.PermissionContext{CtxType: CtxTeam, Value: "team1"}},
		},
	}
	team, err := TeamForPermission(t, PermAppUpdate)
	c.Assert(err, check.IsNil)
	c.Assert(team, check.Equals, "team1")
}

func (s *S) TestGetTeamForPermissionManyTeams(c *check.C) {
	t := &userToken{
		permissions: []Permission{
			{Scheme: PermAppUpdate, Context: permTypes.PermissionContext{CtxType: CtxTeam, Value: "team1"}},
			{Scheme: PermAppUpdate, Context: permTypes.PermissionContext{CtxType: CtxTeam, Value: "team2"}},
		},
	}
	_, err := TeamForPermission(t, PermAppUpdate)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrTooManyTeams)
}

func (s *S) TestGetTeamForPermissionGlobalMustSpecifyTeam(c *check.C) {
	t := &userToken{
		permissions: []Permission{
			{Scheme: PermAll, Context: permTypes.PermissionContext{CtxType: CtxGlobal, Value: ""}},
		},
	}
	_, err := TeamForPermission(t, PermAppUpdate)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrTooManyTeams)
}
