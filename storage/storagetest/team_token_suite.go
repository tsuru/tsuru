// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"
	"sort"
	"time"

	"github.com/tsuru/tsuru/types/auth"
	check "gopkg.in/check.v1"
)

type TeamTokenSuite struct {
	SuiteHooks
	TeamTokenStorage auth.TeamTokenStorage
}

func (s *TeamTokenSuite) TestInsertTeamToken(c *check.C) {
	roles := []auth.RoleInstance{{Name: "app.deploy", ContextValue: "t1"}, {Name: "app.token.read", ContextValue: "t2"}}
	t := auth.TeamToken{Token: "9382908", Team: "team1", Roles: roles}
	err := s.TeamTokenStorage.Insert(context.TODO(), t)
	c.Assert(err, check.IsNil)
	token, err := s.TeamTokenStorage.FindByToken(context.TODO(), t.Token)
	c.Assert(err, check.IsNil)
	c.Assert(token.Token, check.Equals, t.Token)
	c.Assert(token.Team, check.DeepEquals, t.Team)
	c.Assert(token.Roles, check.DeepEquals, roles)
}

func (s *TeamTokenSuite) TestInsertDuplicateTeamToken(c *check.C) {
	t := auth.TeamToken{Token: "9382908", Team: "myteam"}
	err := s.TeamTokenStorage.Insert(context.TODO(), t)
	c.Assert(err, check.IsNil)
	err = s.TeamTokenStorage.Insert(context.TODO(), t)
	c.Assert(err, check.Equals, auth.ErrTeamTokenAlreadyExists)
}

func (s *TeamTokenSuite) TestInsertDuplicateTeamTokenID(c *check.C) {
	t := auth.TeamToken{Token: "1234", TokenID: "1", Team: "myteam"}
	err := s.TeamTokenStorage.Insert(context.TODO(), t)
	c.Assert(err, check.IsNil)
	t = auth.TeamToken{Token: "5678", TokenID: "1", Team: "myteam"}
	err = s.TeamTokenStorage.Insert(context.TODO(), t)
	c.Assert(err, check.Equals, auth.ErrTeamTokenAlreadyExists)
}

func (s *TeamTokenSuite) TestFindTeamTokenByToken(c *check.C) {
	t := auth.TeamToken{Token: "1234"}
	err := s.TeamTokenStorage.Insert(context.TODO(), t)
	c.Assert(err, check.IsNil)
	token, err := s.TeamTokenStorage.FindByToken(context.TODO(), t.Token)
	c.Assert(err, check.IsNil)
	c.Assert(token.Token, check.Equals, t.Token)
}

func (s *TeamTokenSuite) TestFindTeamTokenByTokenNotFound(c *check.C) {
	token, err := s.TeamTokenStorage.FindByToken(context.TODO(), "wat")
	c.Assert(err, check.Equals, auth.ErrTeamTokenNotFound)
	c.Assert(token, check.IsNil)
}

func (s *TeamTokenSuite) TestFindTeamTokensByTeams(c *check.C) {
	err := s.TeamTokenStorage.Insert(context.TODO(), auth.TeamToken{Token: "123", TokenID: "1", Team: "team1"})
	c.Assert(err, check.IsNil)
	err = s.TeamTokenStorage.Insert(context.TODO(), auth.TeamToken{Token: "456", TokenID: "4", Team: "team2"})
	c.Assert(err, check.IsNil)
	err = s.TeamTokenStorage.Insert(context.TODO(), auth.TeamToken{Token: "789", TokenID: "7", Team: "team1"})
	c.Assert(err, check.IsNil)

	tokens, err := s.TeamTokenStorage.FindByTeams(context.TODO(), []string{"team1"})
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 2)
	values := []string{tokens[0].Token, tokens[1].Token}
	sort.Strings(values)
	c.Assert(values, check.DeepEquals, []string{"123", "789"})

	tokens, err = s.TeamTokenStorage.FindByTeams(context.TODO(), []string{"team1", "team2", "teamnotfound"})
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 3)
	values = []string{tokens[0].Token, tokens[1].Token, tokens[2].Token}
	sort.Strings(values)
	c.Assert(values, check.DeepEquals, []string{"123", "456", "789"})

	tokens, err = s.TeamTokenStorage.FindByTeams(context.TODO(), []string{"teamnotfound"})
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 0)

	tokens, err = s.TeamTokenStorage.FindByTeams(context.TODO(), []string{})
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 0)

	tokens, err = s.TeamTokenStorage.FindByTeams(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 3)
	values = []string{tokens[0].Token, tokens[1].Token, tokens[2].Token}
	sort.Strings(values)
	c.Assert(values, check.DeepEquals, []string{"123", "456", "789"})
}

func (s *TeamTokenSuite) TestFindTeamTokenByTeamsNotFound(c *check.C) {
	t1 := auth.TeamToken{Token: "123", Team: "team1"}
	err := s.TeamTokenStorage.Insert(context.TODO(), t1)
	c.Assert(err, check.IsNil)
	teams, err := s.TeamTokenStorage.FindByTeams(context.TODO(), []string{"team2"})
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 0)
}

func (s *TeamTokenSuite) TestUpdateLastAccessTeamToken(c *check.C) {
	expiresAt := time.Now().Add(1 * time.Hour)
	appToken := auth.TeamToken{Token: "123", ExpiresAt: expiresAt}
	err := s.TeamTokenStorage.Insert(context.TODO(), appToken)
	c.Assert(err, check.IsNil)
	t, err := s.TeamTokenStorage.FindByToken(context.TODO(), appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.LastAccess.IsZero(), check.Equals, true)
	err = s.TeamTokenStorage.UpdateLastAccess(context.TODO(), appToken.Token)
	c.Assert(err, check.IsNil)
	t, err = s.TeamTokenStorage.FindByToken(context.TODO(), appToken.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.LastAccess.IsZero(), check.Equals, false)
}

func (s *TeamTokenSuite) TestUpdateLastAccessTokenNotFound(c *check.C) {
	err := s.TeamTokenStorage.UpdateLastAccess(context.TODO(), "token-not-found")
	c.Assert(err, check.Equals, auth.ErrTeamTokenNotFound)
}

func (s *TeamTokenSuite) TestDeleteTeamToken(c *check.C) {
	token := auth.TeamToken{Token: "abc123", TokenID: "abc"}
	err := s.TeamTokenStorage.Insert(context.TODO(), token)
	c.Assert(err, check.IsNil)
	err = s.TeamTokenStorage.Delete(context.TODO(), token.TokenID)
	c.Assert(err, check.IsNil)
	t, err := s.TeamTokenStorage.FindByToken(context.TODO(), token.Token)
	c.Assert(err, check.Equals, auth.ErrTeamTokenNotFound)
	c.Assert(t, check.IsNil)
}

func (s *TeamTokenSuite) TestDeleteTeamTokenNotFound(c *check.C) {
	err := s.TeamTokenStorage.Delete(context.TODO(), "abc")
	c.Assert(err, check.Equals, auth.ErrTeamTokenNotFound)
}

func (s *TeamTokenSuite) TestUpdateTeamToken(c *check.C) {
	t := auth.TeamToken{Token: "9382908", TokenID: "a", Team: "team1"}
	err := s.TeamTokenStorage.Insert(context.TODO(), t)
	c.Assert(err, check.IsNil)
	t.Roles = []auth.RoleInstance{{Name: "app.deploy", ContextValue: "t1"}, {Name: "app.token.read", ContextValue: "t2"}}
	err = s.TeamTokenStorage.Update(context.TODO(), t)
	c.Assert(err, check.IsNil)
	token, err := s.TeamTokenStorage.FindByToken(context.TODO(), t.Token)
	c.Assert(err, check.IsNil)
	c.Assert(token.Token, check.Equals, t.Token)
	c.Assert(token.Team, check.DeepEquals, t.Team)
	c.Assert(token.Roles, check.DeepEquals, t.Roles)
}

func (s *TeamTokenSuite) TestUpdateTeamTokenNotFound(c *check.C) {
	t := auth.TeamToken{Token: "9382908", TokenID: "a", Team: "team1"}
	err := s.TeamTokenStorage.Update(context.TODO(), t)
	c.Assert(err, check.Equals, auth.ErrTeamTokenNotFound)
}
