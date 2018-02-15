// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"sort"
	"time"

	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

type TeamTokenSuite struct {
	SuiteHooks
	TeamTokenService auth.TeamTokenService
}

func (s *TeamTokenSuite) TestInsertTeamToken(c *check.C) {
	roles := []string{"app.deploy", "app.token.read"}
	t := auth.TeamToken{Token: "9382908", AppName: "myapp", Roles: roles}
	err := s.TeamTokenService.Insert(t)
	c.Assert(err, check.IsNil)
	token, err := s.TeamTokenService.FindByToken(t.Token)
	c.Assert(err, check.IsNil)
	c.Assert(token.Token, check.Equals, t.Token)
	c.Assert(token.AppName, check.Equals, t.AppName)
	c.Assert(token.Roles, check.DeepEquals, roles)
}

func (s *TeamTokenSuite) TestInsertDuplicateTeamToken(c *check.C) {
	t := auth.TeamToken{Token: "9382908", AppName: "myapp"}
	err := s.TeamTokenService.Insert(t)
	c.Assert(err, check.IsNil)
	err = s.TeamTokenService.Insert(t)
	c.Assert(err, check.Equals, auth.ErrTeamTokenAlreadyExists)
}

func (s *TeamTokenSuite) TestFindTeamTokenByToken(c *check.C) {
	t := auth.TeamToken{Token: "1234"}
	err := s.TeamTokenService.Insert(t)
	c.Assert(err, check.IsNil)
	token, err := s.TeamTokenService.FindByToken(t.Token)
	c.Assert(err, check.IsNil)
	c.Assert(token.Token, check.Equals, t.Token)
}

func (s *TeamTokenSuite) TestFindTeamTokenByTokenNotFound(c *check.C) {
	token, err := s.TeamTokenService.FindByToken("wat")
	c.Assert(err, check.Equals, auth.ErrTeamTokenNotFound)
	c.Assert(token, check.IsNil)
}

func (s *TeamTokenSuite) TestFindTeamTokensByTeam(c *check.C) {
	err := s.TeamTokenService.Insert(auth.TeamToken{Token: "123", Teams: []string{"team1", "team2", "team3"}})
	c.Assert(err, check.IsNil)
	err = s.TeamTokenService.Insert(auth.TeamToken{Token: "456", Teams: []string{"team2"}})
	c.Assert(err, check.IsNil)
	err = s.TeamTokenService.Insert(auth.TeamToken{Token: "789", Teams: []string{"team1"}})
	c.Assert(err, check.IsNil)

	tokens, err := s.TeamTokenService.FindByTeam("team1")
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 2)
	values := []string{tokens[0].Token, tokens[1].Token}
	sort.Strings(values)
	c.Assert(values, check.DeepEquals, []string{"123", "789"})

	tokens, err = s.TeamTokenService.FindByTeam("team3")
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 1)
	c.Assert(tokens[0].Token, check.Equals, "123")
}

func (s *TeamTokenSuite) TestFindTeamTokenByTeamNotFound(c *check.C) {
	t1 := auth.TeamToken{Token: "123", Teams: []string{"team1"}}
	err := s.TeamTokenService.Insert(t1)
	c.Assert(err, check.IsNil)
	teams, err := s.TeamTokenService.FindByTeam("team2")
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 0)
}

func (s *TeamTokenSuite) TestAuthenticateTeamToken(c *check.C) {
	expiresAt := time.Now().Add(1 * time.Hour)
	appToken := auth.TeamToken{Token: "123", ExpiresAt: &expiresAt}
	err := s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	t, err := s.TeamTokenService.Authenticate(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Token, check.Equals, appToken.Token)
	c.Assert(t.LastAccess, check.NotNil)
}

func (s *TeamTokenSuite) TestAuthenticateTeamTokenNoExpiration(c *check.C) {
	appToken := auth.TeamToken{Token: "123"}
	err := s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	t, err := s.TeamTokenService.Authenticate(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Token, check.Equals, appToken.Token)
	c.Assert(t.LastAccess, check.NotNil)
}

func (s *TeamTokenSuite) TestAuthenticateTeamTokenExpired(c *check.C) {
	expiresAt := time.Now().Add(-1 * time.Second)
	appToken := auth.TeamToken{Token: "123", ExpiresAt: &expiresAt}
	err := s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	t, err := s.TeamTokenService.Authenticate(appToken.Token)
	c.Assert(err, check.Equals, auth.ErrTeamTokenExpired)
	c.Assert(t, check.IsNil)
}

func (s *TeamTokenSuite) TestAuthenticateTeamTokenNotFound(c *check.C) {
	t, err := s.TeamTokenService.Authenticate("token-not-found")
	c.Assert(err, check.Equals, auth.ErrTeamTokenNotFound)
	c.Assert(t, check.IsNil)
}

func (s *TeamTokenSuite) TestAddTeams(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)

	err = s.TeamTokenService.AddTeams(appToken, "team1")
	c.Assert(err, check.IsNil)

	t, err := s.TeamTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Teams, check.DeepEquals, []string{"team1"})
}

func (s *TeamTokenSuite) TestAddTeamsTeamTokenNotFound(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.AddTeams(appToken, "team1")
	c.Assert(err, check.ErrorMatches, "team token not found")
}

func (s *TeamTokenSuite) TestAddTeamsNoDuplicates(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)

	err = s.TeamTokenService.AddTeams(appToken, "team1", "team2", "team1")
	c.Assert(err, check.IsNil)

	t, err := s.TeamTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Teams, check.DeepEquals, []string{"team1", "team2"})
}

func (s *TeamTokenSuite) TestRemoveTeams(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	err = s.TeamTokenService.AddTeams(appToken, "team1", "team2", "team3")
	c.Assert(err, check.IsNil)

	err = s.TeamTokenService.RemoveTeams(appToken, "team2", "team1", "team4")
	c.Assert(err, check.IsNil)

	t, err := s.TeamTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Teams, check.DeepEquals, []string{"team3"})
}

func (s *TeamTokenSuite) TestRemoveTeamsTeamTokenNotFound(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.RemoveTeams(appToken, "team1")
	c.Assert(err, check.ErrorMatches, "team token not found")
}

func (s *TeamTokenSuite) TestRemoveTeamsNotFound(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)

	err = s.TeamTokenService.RemoveTeams(appToken, "team1")
	c.Assert(err, check.IsNil)

	t, err := s.TeamTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Teams, check.HasLen, 0)
}

func (s *TeamTokenSuite) TestAddRoles(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("role1", "app", "")
	c.Assert(err, check.IsNil)

	err = s.TeamTokenService.AddRoles(appToken, "role1")
	c.Assert(err, check.IsNil)

	t, err := s.TeamTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.DeepEquals, []string{"role1"})
}

func (s *TeamTokenSuite) TestAddRolesTeamTokenNotFound(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.AddRoles(appToken, "role1")
	c.Assert(err, check.ErrorMatches, "team token not found")
}

func (s *TeamTokenSuite) TestAddRolesNoDuplicates(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("role1", "app", "")
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("role2", "app", "")
	c.Assert(err, check.IsNil)

	err = s.TeamTokenService.AddRoles(appToken, "role1", "role2", "role1")
	c.Assert(err, check.IsNil)

	t, err := s.TeamTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.DeepEquals, []string{"role1", "role2"})
}

func (s *TeamTokenSuite) TestRemoveRoles(c *check.C) {
	_, err := permission.NewRole("role1", "app", "")
	c.Assert(err, check.IsNil)
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err = s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	err = s.TeamTokenService.AddRoles(appToken, "role1", "role2", "role3")
	c.Assert(err, check.IsNil)

	err = s.TeamTokenService.RemoveRoles(appToken, "role2", "role1", "role4")
	c.Assert(err, check.IsNil)

	t, err := s.TeamTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.DeepEquals, []string{"role3"})
}

func (s *TeamTokenSuite) TestRemoveRolesTeamTokenNotFound(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.RemoveRoles(appToken, "role1")
	c.Assert(err, check.ErrorMatches, "team token not found")
}

func (s *TeamTokenSuite) TestRemoveRolesNotFound(c *check.C) {
	appToken := auth.TeamToken{Token: "123", AppName: "app1"}
	err := s.TeamTokenService.Insert(appToken)
	c.Assert(err, check.IsNil)
	_, err = permission.NewRole("role1", "app", "")
	c.Assert(err, check.IsNil)

	err = s.TeamTokenService.RemoveRoles(appToken, "role1")
	c.Assert(err, check.IsNil)

	t, err := s.TeamTokenService.FindByToken(appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.Roles, check.HasLen, 0)
}

func (s *TeamTokenSuite) TestDeleteTeamToken(c *check.C) {
	token := auth.TeamToken{Token: "abc123"}
	err := s.TeamTokenService.Insert(token)
	c.Assert(err, check.IsNil)
	err = s.TeamTokenService.Delete(token)
	c.Assert(err, check.IsNil)
	t, err := s.TeamTokenService.FindByToken(token.Token)
	c.Assert(err, check.Equals, auth.ErrTeamTokenNotFound)
	c.Assert(t, check.IsNil)
}

func (s *TeamTokenSuite) TestDeleteTeamTokenNotFound(c *check.C) {
	err := s.TeamTokenService.Delete(auth.TeamToken{Token: "abc123"})
	c.Assert(err, check.Equals, auth.ErrTeamTokenNotFound)
}
