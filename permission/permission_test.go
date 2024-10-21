// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"context"

	permTypes "github.com/tsuru/tsuru/types/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestPermissionSchemeFullName(c *check.C) {
	table := []struct {
		p      permTypes.PermissionScheme
		result string
	}{
		{permTypes.PermissionScheme{}, ""},
		{permTypes.PermissionScheme{Name: "app"}, "app"},
		{permTypes.PermissionScheme{Name: "app", Parent: &permTypes.PermissionScheme{}}, "app"},
		{permTypes.PermissionScheme{Name: "env", Parent: &permTypes.PermissionScheme{Name: "app"}}, "app.env"},
		{permTypes.PermissionScheme{Name: "set", Parent: &permTypes.PermissionScheme{Name: "en-nv", Parent: &permTypes.PermissionScheme{Name: "app"}}}, "app.en-nv.set"},
	}
	for _, el := range table {
		c.Check(el.p.FullName(), check.Equals, el.result)
	}
}

func (s *S) TestPermissionSchemeAllowedContexts(c *check.C) {
	table := []struct {
		p   permTypes.PermissionScheme
		ctx []permTypes.ContextType
	}{
		{permTypes.PermissionScheme{}, []permTypes.ContextType{permTypes.CtxGlobal}},
		{permTypes.PermissionScheme{Contexts: []permTypes.ContextType{permTypes.CtxApp}}, []permTypes.ContextType{permTypes.CtxGlobal, permTypes.CtxApp}},
		{permTypes.PermissionScheme{Parent: &permTypes.PermissionScheme{Contexts: []permTypes.ContextType{permTypes.CtxApp}}}, []permTypes.ContextType{permTypes.CtxGlobal, permTypes.CtxApp}},
		{permTypes.PermissionScheme{Contexts: []permTypes.ContextType{}, Parent: &permTypes.PermissionScheme{Contexts: []permTypes.ContextType{permTypes.CtxApp}}}, []permTypes.ContextType{permTypes.CtxGlobal}},
		{permTypes.PermissionScheme{Contexts: []permTypes.ContextType{permTypes.CtxTeam}, Parent: &permTypes.PermissionScheme{Contexts: []permTypes.ContextType{permTypes.CtxApp}}}, []permTypes.ContextType{permTypes.CtxGlobal, permTypes.CtxTeam}},
	}
	for _, el := range table {
		c.Check(el.p.AllowedContexts(), check.DeepEquals, el.ctx)
	}
}

type userToken struct {
	permissions []permTypes.Permission
}

func (t *userToken) Permissions(ctx context.Context) ([]permTypes.Permission, error) {
	return t.permissions, nil
}

func (s *S) TestCheck(c *check.C) {
	ctx := context.TODO()
	t := &userToken{
		permissions: []permTypes.Permission{
			{Scheme: PermAppUpdate, Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team1"}},
			{Scheme: PermAppDeploy, Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team3"}},
			{Scheme: PermAppUpdateEnvUnset, Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal}},
		},
	}
	c.Assert(Check(ctx, t, PermAppUpdateEnvSet, permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(ctx, t, PermAppUpdate, permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(ctx, t, PermAppDeploy, permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team1"}), check.Equals, false)
	c.Assert(Check(ctx, t, PermAppDeploy, permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team3"}), check.Equals, true)
	c.Assert(Check(ctx, t, PermAppUpdate, permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team2"}), check.Equals, false)
	c.Assert(Check(ctx, t, PermAppUpdateEnvUnset, permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(ctx, t, PermAppUpdateEnvUnset, permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team10"}), check.Equals, true)
	c.Assert(Check(ctx, t, PermAppUpdateEnvUnset), check.Equals, true)
}

func (s *S) TestCheckSuperToken(c *check.C) {
	ctx := context.TODO()
	t := &userToken{
		permissions: []permTypes.Permission{
			{Scheme: PermAll, Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal}},
		},
	}
	c.Assert(Check(ctx, t, PermAppDeploy, permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team1"}), check.Equals, true)
	c.Assert(Check(ctx, t, PermAppUpdateEnvUnset), check.Equals, true)
}

func (s *S) TestGetTeamForPermission(c *check.C) {
	t := &userToken{
		permissions: []permTypes.Permission{
			{Scheme: PermAppUpdate, Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team1"}},
		},
	}
	team, err := TeamForPermission(context.TODO(), t, PermAppUpdate)
	c.Assert(err, check.IsNil)
	c.Assert(team, check.Equals, "team1")
}

func (s *S) TestGetTeamForPermissionManyTeams(c *check.C) {
	t := &userToken{
		permissions: []permTypes.Permission{
			{Scheme: PermAppUpdate, Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team1"}},
			{Scheme: PermAppUpdate, Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: "team2"}},
		},
	}
	_, err := TeamForPermission(context.TODO(), t, PermAppUpdate)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrTooManyTeams)
}

func (s *S) TestGetTeamForPermissionGlobalMustSpecifyTeam(c *check.C) {
	t := &userToken{
		permissions: []permTypes.Permission{
			{Scheme: PermAll, Context: permTypes.PermissionContext{CtxType: permTypes.CtxGlobal, Value: ""}},
		},
	}
	_, err := TeamForPermission(context.TODO(), t, PermAppUpdate)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrTooManyTeams)
}
