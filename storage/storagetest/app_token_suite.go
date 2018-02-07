// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"sort"

	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

type AppTokenSuite struct {
	SuiteHooks
	AppTokenService auth.AppTokenService
}

func (s *AppTokenSuite) TestInsertAppToken(c *check.C) {
	roles := []string{"app.deploy", "app.token.read"}
	t := auth.AppToken{Token: "9382908", AppName: "myapp", Roles: roles}
	err := s.AppTokenService.Insert(t)
	c.Assert(err, check.IsNil)
	token, err := s.AppTokenService.FindByToken(t.Token)
	c.Assert(err, check.IsNil)
	c.Assert(token.Token, check.Equals, t.Token)
	c.Assert(token.AppName, check.Equals, t.AppName)
	c.Assert(token.Roles, check.DeepEquals, roles)
}

func (s *AppTokenSuite) TestInsertDuplicateAppToken(c *check.C) {
	t := auth.AppToken{Token: "9382908", AppName: "myapp"}
	err := s.AppTokenService.Insert(t)
	c.Assert(err, check.IsNil)
	err = s.AppTokenService.Insert(t)
	c.Assert(err, check.Equals, auth.ErrAppTokenAlreadyExists)
}

func (s *AppTokenSuite) TestFindAppTokenByToken(c *check.C) {
	t := auth.AppToken{Token: "1234"}
	err := s.AppTokenService.Insert(t)
	c.Assert(err, check.IsNil)
	token, err := s.AppTokenService.FindByToken(t.Token)
	c.Assert(err, check.IsNil)
	c.Assert(token.Token, check.Equals, t.Token)
}

func (s *AppTokenSuite) TestFindAppTokenByTokenNotFound(c *check.C) {
	token, err := s.AppTokenService.FindByToken("wat")
	c.Assert(err, check.Equals, auth.ErrAppTokenNotFound)
	c.Assert(token, check.IsNil)
}

func (s *AppTokenSuite) TestFindAppTokensByAppName(c *check.C) {
	err := s.AppTokenService.Insert(auth.AppToken{Token: "123", AppName: "app1"})
	c.Assert(err, check.IsNil)
	err = s.AppTokenService.Insert(auth.AppToken{Token: "456", AppName: "app2"})
	c.Assert(err, check.IsNil)
	err = s.AppTokenService.Insert(auth.AppToken{Token: "789", AppName: "app1"})
	c.Assert(err, check.IsNil)
	tokens, err := s.AppTokenService.FindByAppName("app1")
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 2)
	values := []string{tokens[0].Token, tokens[1].Token}
	sort.Strings(values)
	c.Assert(values, check.DeepEquals, []string{"123", "789"})
}

func (s *AppTokenSuite) TestFindAppTokenByAppNameNotFound(c *check.C) {
	t1 := auth.AppToken{Token: "123", AppName: "app1"}
	err := s.AppTokenService.Insert(t1)
	c.Assert(err, check.IsNil)
	teams, err := s.AppTokenService.FindByAppName("app2")
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 0)
}

func (s *AppTokenSuite) TestAuthenticateAppToken(c *check.C) {
	appToken := auth.AppToken{Token: "123"}
	err := s.AppTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	t, err := s.AppTokenService.Authenticate(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Token, check.Equals, appToken.Token)
	c.Assert(t.LastAccess, check.NotNil)
}

func (s *AppTokenSuite) TestAuthenticateAppTokenNotFound(c *check.C) {
	t, err := s.AppTokenService.Authenticate("token-not-found")
	c.Assert(err, check.NotNil)
	c.Assert(t, check.IsNil)
}

func (s *AppTokenSuite) TestAddRoles(c *check.C) {
	appToken := auth.AppToken{Token: "123", AppName: "app1"}
	err := s.AppTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("role1", "app", "")
	c.Assert(err, check.IsNil)

	err = s.AppTokenService.AddRoles(appToken, "role1")
	c.Assert(err, check.IsNil)

	t, err := s.AppTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.DeepEquals, []string{"role1"})
}

func (s *AppTokenSuite) TestAddRolesAppTokenNotFound(c *check.C) {
	appToken := auth.AppToken{Token: "123", AppName: "app1"}
	err := s.AppTokenService.AddRoles(appToken, "role1")
	c.Assert(err, check.ErrorMatches, "app token not found")
}

func (s *AppTokenSuite) TestAddRolesNoDuplicates(c *check.C) {
	appToken := auth.AppToken{Token: "123", AppName: "app1"}
	err := s.AppTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("role1", "app", "")
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("role2", "app", "")
	c.Assert(err, check.IsNil)

	err = s.AppTokenService.AddRoles(appToken, "role1", "role2", "role1")
	c.Assert(err, check.IsNil)

	t, err := s.AppTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.DeepEquals, []string{"role1", "role2"})
}

func (s *AppTokenSuite) TestRemoveRoles(c *check.C) {
	_, err := permission.NewRole("role1", "app", "")
	c.Assert(err, check.IsNil)
	appToken := auth.AppToken{Token: "123", AppName: "app1"}
	err = s.AppTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	err = s.AppTokenService.AddRoles(appToken, "role1", "role2", "role3")
	c.Assert(err, check.IsNil)

	err = s.AppTokenService.RemoveRoles(appToken, "role2", "role1", "role4")
	c.Assert(err, check.IsNil)

	t, err := s.AppTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.DeepEquals, []string{"role3"})
}

func (s *AppTokenSuite) TestRemoveRolesAppTokenNotFound(c *check.C) {
	appToken := auth.AppToken{Token: "123", AppName: "app1"}
	err := s.AppTokenService.RemoveRoles(appToken, "role1")
	c.Assert(err, check.ErrorMatches, "app token not found")
}

func (s *AppTokenSuite) TestRemoveRolesNotFound(c *check.C) {
	appToken := auth.AppToken{Token: "123", AppName: "app1"}
	err := s.AppTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("role1", "app", "")
	c.Assert(err, check.IsNil)

	err = s.AppTokenService.RemoveRoles(appToken, "role1")
	c.Assert(err, check.IsNil)

	t, err := s.AppTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.HasLen, 0)
}

func (s *AppTokenSuite) TestDeleteAppToken(c *check.C) {
	token := auth.AppToken{Token: "abc123"}
	err := s.AppTokenService.Insert(token)
	c.Assert(err, check.IsNil)
	err = s.AppTokenService.Delete(token)
	c.Assert(err, check.IsNil)
	t, err := s.AppTokenService.FindByToken(token.Token)
	c.Assert(err, check.Equals, auth.ErrAppTokenNotFound)
	c.Assert(t, check.IsNil)
}

func (s *AppTokenSuite) TestDeleteAppTokenNotFound(c *check.C) {
	err := s.AppTokenService.Delete(auth.AppToken{Token: "abc123"})
	c.Assert(err, check.Equals, auth.ErrAppTokenNotFound)
}
