// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"sort"

	"gopkg.in/check.v1"
)

func (s *S) TestNewRole(c *check.C) {
	r, err := NewRole("myrole", "app")
	c.Assert(err, check.IsNil)
	c.Assert(r.Name, check.Equals, "myrole")
	c.Assert(r.ContextType, check.Equals, CtxApp)
	_, err = NewRole("myrole", "global")
	c.Assert(err, check.Equals, ErrRoleAlreadyExists)
	_, err = NewRole("  ", "app")
	c.Assert(err, check.ErrorMatches, `invalid role name ""`)
	_, err = NewRole("myrole2", "invalid")
	c.Assert(err, check.ErrorMatches, `invalid context type "invalid"`)
}

func (s *S) TestListRoles(c *check.C) {
	r, err := NewRole("test", "app")
	c.Assert(err, check.IsNil)
	roles, err := ListRoles()
	c.Assert(err, check.IsNil)
	expected := []Role{{Name: "test", ContextType: "app", SchemeNames: []string{}, Events: []RoleEvent{}}}
	c.Assert(roles, check.DeepEquals, expected)
	err = r.AddPermissions("app.deploy", "app.update")
	c.Assert(err, check.IsNil)
	r.SchemeNames = append(r.SchemeNames, "invalid")
	coll, err := rolesCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	err = coll.UpdateId(r.Name, r)
	c.Assert(err, check.IsNil)
	roles, err = ListRoles()
	expected = []Role{{Name: "test", ContextType: "app", Events: []RoleEvent{}, SchemeNames: []string{
		"app.deploy", "app.update",
	}}}
	c.Assert(roles, check.DeepEquals, expected)
}

func (s *S) TestFindRole(c *check.C) {
	_, err := NewRole("myrole", "team")
	c.Assert(err, check.IsNil)
	r, err := FindRole("myrole")
	c.Assert(err, check.IsNil)
	c.Assert(r.Name, check.Equals, "myrole")
	c.Assert(r.ContextType, check.Equals, CtxTeam)
	_, err = FindRole("something")
	c.Assert(err, check.Equals, ErrRoleNotFound)
}

func (s *S) TestRoleAddPermissions(c *check.C) {
	r, err := NewRole("myrole", "team")
	c.Assert(err, check.IsNil)
	err = r.AddPermissions("app.update", "app.update.env.set")
	c.Assert(err, check.IsNil)
	sort.Strings(r.SchemeNames)
	expected := []string{
		"app.update",
		"app.update.env.set",
	}
	c.Assert(r.SchemeNames, check.DeepEquals, expected)
	dbR, err := FindRole("myrole")
	c.Assert(err, check.IsNil)
	sort.Strings(dbR.SchemeNames)
	c.Assert(dbR.SchemeNames, check.DeepEquals, expected)
}

func (s *S) TestRoleGlobalAddPermissions(c *check.C) {
	r, err := NewRole("myrole", "global")
	c.Assert(err, check.IsNil)
	err = r.AddPermissions("")
	c.Assert(err, check.ErrorMatches, "empty permission name")
	err = r.AddPermissions("*")
	c.Assert(err, check.IsNil)
	sort.Strings(r.SchemeNames)
	expected := []string{"*"}
	c.Assert(r.SchemeNames, check.DeepEquals, expected)
	dbR, err := FindRole("myrole")
	c.Assert(err, check.IsNil)
	sort.Strings(dbR.SchemeNames)
	c.Assert(dbR.SchemeNames, check.DeepEquals, expected)
	err = r.AddPermissions("app.deploy")
	c.Assert(err, check.IsNil)
}

func (s *S) TestRoleAddPermissionsInvalid(c *check.C) {
	r, err := NewRole("myrole", "team")
	c.Assert(err, check.IsNil)
	err = r.AddPermissions("app.update.env.set.nih")
	c.Assert(err, check.ErrorMatches, `permission named "app.update.env.set.nih" not found`)
	err = r.AddPermissions("node.create")
	c.Assert(err, check.ErrorMatches, `permission "node.create" not allowed with context of type "team"`)
}

func (s *S) TestRemovePermissions(c *check.C) {
	r, err := NewRole("myrole", "team")
	c.Assert(err, check.IsNil)
	err = r.AddPermissions("app.update", "app.update.env.set")
	c.Assert(err, check.IsNil)
	err = r.RemovePermissions("app.update")
	c.Assert(err, check.IsNil)
	expected := []string{"app.update.env.set"}
	c.Assert(r.SchemeNames, check.DeepEquals, expected)
	dbR, err := FindRole("myrole")
	c.Assert(err, check.IsNil)
	c.Assert(dbR.SchemeNames, check.DeepEquals, expected)
}

func (s *S) TestDestroyRole(c *check.C) {
	_, err := NewRole("myrole", "team")
	c.Assert(err, check.IsNil)
	err = DestroyRole("myrole")
	c.Assert(err, check.IsNil)
	err = DestroyRole("myrole")
	c.Assert(err, check.Equals, ErrRoleNotFound)
}

func (s *S) TestPermissionsFor(c *check.C) {
	r, err := NewRole("myrole", "team")
	c.Assert(err, check.IsNil)
	perms := r.PermissionsFor("something")
	c.Assert(perms, check.DeepEquals, []Permission{})
	err = r.AddPermissions("app.update", "app.update.env.set")
	c.Assert(err, check.IsNil)
	expected := []Permission{
		{Scheme: PermissionRegistry.get("app.update"), Context: PermissionContext{CtxType: CtxTeam, Value: "something"}},
		{Scheme: PermissionRegistry.get("app.update.env.set"), Context: PermissionContext{CtxType: CtxTeam, Value: "something"}},
	}
	perms = r.PermissionsFor("something")
	c.Assert(perms, check.DeepEquals, expected)
	r.SchemeNames = append(r.SchemeNames, "invalidxxx")
	perms = r.PermissionsFor("something")
	c.Assert(perms, check.DeepEquals, expected)
}

func (s *S) TestRoleAddEvent(c *check.C) {
	r, err := NewRole("myrole", "team")
	c.Assert(err, check.IsNil)
	err = r.AddEvent(RoleEventTeamCreate)
	c.Assert(err, check.IsNil)
	c.Assert(r.Events, check.DeepEquals, []RoleEvent{RoleEventTeamCreate})
	err = r.AddEvent(RoleEventTeamCreate)
	c.Assert(err, check.IsNil)
	c.Assert(r.Events, check.DeepEquals, []RoleEvent{RoleEventTeamCreate})
	dbR, err := FindRole("myrole")
	c.Assert(err, check.IsNil)
	c.Assert(dbR.Events, check.DeepEquals, []RoleEvent{RoleEventTeamCreate})
	err = r.AddEvent(RoleEvent("someother"))
	c.Assert(err, check.IsNil)
	var evts []string
	for _, e := range r.Events {
		evts = append(evts, string(e))
	}
	sort.Strings(evts)
	c.Assert(evts, check.DeepEquals, []string{"someother", string(RoleEventTeamCreate)})
}

func (s *S) TestRoleRemoveEvent(c *check.C) {
	r, err := NewRole("myrole", "team")
	c.Assert(err, check.IsNil)
	err = r.AddEvent(RoleEventTeamCreate)
	c.Assert(err, check.IsNil)
	dbR, err := FindRole("myrole")
	c.Assert(err, check.IsNil)
	err = dbR.RemoveEvent(RoleEventTeamCreate)
	c.Assert(err, check.IsNil)
	c.Assert(dbR.Events, check.DeepEquals, []RoleEvent{})
	dbR, err = FindRole("myrole")
	c.Assert(err, check.IsNil)
	c.Assert(dbR.Events, check.DeepEquals, []RoleEvent{})
}

func (s *S) TestListRolesWithEvents(c *check.C) {
	_, err := NewRole("myrole1", "team")
	c.Assert(err, check.IsNil)
	r2, err := NewRole("myrole2", "team")
	c.Assert(err, check.IsNil)
	r3, err := NewRole("myrole3", "team")
	c.Assert(err, check.IsNil)
	err = r2.AddEvent(RoleEventTeamCreate)
	c.Assert(err, check.IsNil)
	err = r3.AddEvent(RoleEventTeamCreate)
	c.Assert(err, check.IsNil)
	roles, err := ListRolesWithEvents()
	c.Assert(err, check.IsNil)
	var names []string
	for _, r := range roles {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"myrole2", "myrole3"})
}

func (s *S) TestListRolesForEvent(c *check.C) {
	_, err := NewRole("myrole1", "team")
	c.Assert(err, check.IsNil)
	r2, err := NewRole("myrole2", "team")
	c.Assert(err, check.IsNil)
	r3, err := NewRole("myrole3", "global")
	c.Assert(err, check.IsNil)
	err = r2.AddEvent(RoleEventTeamCreate)
	c.Assert(err, check.IsNil)
	err = r3.AddEvent(RoleEventUserCreate)
	c.Assert(err, check.IsNil)
	roles, err := ListRolesForEvent(RoleEventTeamCreate)
	c.Assert(err, check.IsNil)
	c.Assert(roles, check.HasLen, 1)
	c.Assert(roles[0].Name, check.Equals, "myrole2")
}
