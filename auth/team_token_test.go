// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	check "gopkg.in/check.v1"
)

type userToken struct {
	user        *User
	permissions []permission.Permission
}

func (t *userToken) GetValue() string {
	return ""
}
func (t *userToken) GetAppName() string {
	return ""
}
func (t *userToken) GetUserName() string {
	return ""
}
func (t *userToken) IsAppToken() bool {
	return false
}
func (t *userToken) User() (*authTypes.User, error) {
	return ConvertOldUser(t.user, nil)
}
func (t *userToken) Permissions() ([]permission.Permission, error) {
	return t.permissions, nil
}

func (s *S) Test_TeamTokenService_Create(c *check.C) {
	token, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{Team: s.team.Name}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	expected := authTypes.TeamToken{
		Team:         "cobrateam",
		Token:        token.Token,
		TokenID:      "cobrateam-" + token.Token[:5],
		CreatorEmail: s.user.Email,
		CreatedAt:    token.CreatedAt,
		ExpiresAt:    time.Time{},
	}
	c.Assert(token, check.DeepEquals, expected)
	t, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), token.TokenID)
	c.Assert(err, check.IsNil)
	t.CreatedAt = expected.CreatedAt
	c.Assert(t, check.DeepEquals, expected)
}

func (s *S) Test_TeamTokenService_Create_WithExpires(c *check.C) {
	token, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{Team: s.team.Name, ExpiresIn: 60 * 60}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	expected := authTypes.TeamToken{
		Team:         "cobrateam",
		Token:        token.Token,
		TokenID:      "cobrateam-" + token.Token[:5],
		CreatorEmail: s.user.Email,
		CreatedAt:    token.CreatedAt,
		ExpiresAt:    token.CreatedAt.Add(time.Hour),
	}
	c.Assert(token, check.DeepEquals, expected)
	t, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), token.TokenID)
	c.Assert(err, check.IsNil)
	c.Assert(t.ExpiresAt.Sub(t.CreatedAt), check.Equals, time.Hour)
	t.CreatedAt = expected.CreatedAt
	t.ExpiresAt = expected.ExpiresAt
	c.Assert(t, check.DeepEquals, expected)
}

func (s *S) Test_TeamTokenService_Create_ValidationError(c *check.C) {
	invalidTokenErr := errors.New("invalid token_id")
	var tests = []struct {
		tokenID     string
		expectedErr error
	}{
		{"valid-token", nil},
		{"invalid token", invalidTokenErr},
		{"UPPERCASE", invalidTokenErr},
		{"loooooooooooooooooong-token-41-characters", invalidTokenErr},
		{"not-so-loooooooooong-token-40-characters", nil},
	}

	for _, test := range tests {
		_, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{Team: s.team.Name, TokenID: test.tokenID}, &userToken{user: s.user})
		if test.expectedErr == nil {
			c.Check(err, check.IsNil)
		} else {
			c.Check(err, check.ErrorMatches, test.expectedErr.Error())
		}
	}
}

func (s *S) Test_TeamTokenService_Authenticate(c *check.C) {
	token, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{Team: s.team.Name}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	t, err := servicemanager.TeamToken.Authenticate(context.TODO(), "bearer "+token.Token)
	c.Assert(err, check.IsNil)
	c.Assert(t.GetValue(), check.Equals, token.Token)
	c.Assert(t.IsAppToken(), check.Equals, false)
	c.Assert(t.GetAppName(), check.Equals, "")
	c.Assert(t.GetUserName(), check.Equals, fmt.Sprintf("cobrateam-%s", token.Token[:5]))
	namedToken, ok := t.(authTypes.NamedToken)
	c.Assert(ok, check.Equals, true)
	c.Assert(namedToken.GetTokenName(), check.Equals, fmt.Sprintf("cobrateam-%s", token.Token[:5]))
	u, err := t.User()
	c.Assert(err, check.IsNil)
	c.Assert(u, check.DeepEquals, &authTypes.User{Email: fmt.Sprintf("%s@token.tsuru.invalid", namedToken.GetTokenName()), Quota: quota.UnlimitedQuota, FromToken: true})
	perms, err := t.Permissions()
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.HasLen, 0)
	dbToken, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), token.TokenID)
	c.Assert(err, check.IsNil)
	c.Assert(dbToken.LastAccess.IsZero(), check.Equals, false)
}

func (s *S) Test_TeamTokenService_Authenticate_NotFound(c *check.C) {
	_, err := servicemanager.TeamToken.Authenticate(context.TODO(), "bearer abc")
	c.Assert(err, check.Equals, ErrInvalidToken)
}

func (s *S) Test_TeamTokenService_Authenticate_Expired(c *check.C) {
	token, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{Team: s.team.Name, ExpiresIn: -1}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	_, err = servicemanager.TeamToken.Authenticate(context.TODO(), "bearer "+token.Token)
	c.Assert(err, check.Equals, authTypes.ErrTeamTokenExpired)
}

func (s *S) Test_TeamTokenService_AddRole(c *check.C) {
	_, err := permission.NewRole("app-deployer", "app", "")
	c.Assert(err, check.IsNil)
	token, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{Team: s.team.Name}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), token.TokenID, "app-deployer", "myapp")
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), token.TokenID, "app-deployer", "myapp2")
	c.Assert(err, check.IsNil)
	dbToken, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), token.TokenID)
	c.Assert(err, check.IsNil)
	c.Assert(dbToken.Roles, check.DeepEquals, []authTypes.RoleInstance{
		{Name: "app-deployer", ContextValue: "myapp"},
		{Name: "app-deployer", ContextValue: "myapp2"},
	})
}

func (s *S) Test_TeamTokenService_AddRole_TokenNotFound(c *check.C) {
	_, err := permission.NewRole("app-deployer", "app", "")
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), "invalid-token", "app-deployer", "myapp")
	c.Assert(err, check.Equals, authTypes.ErrTeamTokenNotFound)
}

func (s *S) Test_TeamTokenService_AddRole_RoleNotFound(c *check.C) {
	token, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{Team: s.team.Name}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), token.TokenID, "app-deployer", "myapp")
	c.Assert(err, check.Equals, permTypes.ErrRoleNotFound)
}

func (s *S) Test_TeamTokenService_RemoveRole(c *check.C) {
	_, err := permission.NewRole("app-deployer", "app", "")
	c.Assert(err, check.IsNil)
	token, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{Team: s.team.Name}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), token.TokenID, "app-deployer", "myapp")
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), token.TokenID, "app-deployer", "myapp2")
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.RemoveRole(context.TODO(), token.TokenID, "app-deployer", "myapp")
	c.Assert(err, check.IsNil)
	dbToken, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), token.TokenID)
	c.Assert(err, check.IsNil)
	c.Assert(dbToken.Roles, check.DeepEquals, []authTypes.RoleInstance{
		{Name: "app-deployer", ContextValue: "myapp2"},
	})
	err = servicemanager.TeamToken.RemoveRole(context.TODO(), token.TokenID, "app-deployer", "myapp2")
	c.Assert(err, check.IsNil)
	dbToken, err = servicemanager.TeamToken.FindByTokenID(context.TODO(), token.TokenID)
	c.Assert(err, check.IsNil)
	c.Assert(dbToken.Roles, check.IsNil)
}

func tokensForEquals(v []authTypes.TeamToken) []authTypes.TeamToken {
	sort.Slice(v, func(i, j int) bool {
		return v[i].TokenID < v[j].TokenID
	})
	for i := range v {
		v[i].CreatedAt = time.Time{}
	}
	return v
}

func (s *S) Test_TeamTokenService_FindByUserToken(c *check.C) {
	ctx := context.TODO()
	userToken := &userToken{
		user: s.user,
	}
	t1, err := servicemanager.TeamToken.Create(ctx, authTypes.TeamTokenCreateArgs{
		Team:    s.team.Name,
		TokenID: "t1",
	}, userToken)
	c.Assert(err, check.IsNil)
	t2, err := servicemanager.TeamToken.Create(ctx, authTypes.TeamTokenCreateArgs{
		Team:    s.team.Name,
		TokenID: "t2",
	}, userToken)
	c.Assert(err, check.IsNil)
	teamTokens, err := servicemanager.TeamToken.FindByUserToken(ctx, userToken)
	c.Assert(err, check.IsNil)
	c.Assert(tokensForEquals(teamTokens), check.DeepEquals, tokensForEquals([]authTypes.TeamToken{
		{
			Token:        t1.Token,
			TokenID:      "t1",
			Team:         "cobrateam",
			CreatorEmail: "timeredbull@globo.com",
		},
		{
			Token:        t2.Token,
			TokenID:      "t2",
			Team:         "cobrateam",
			CreatorEmail: "timeredbull@globo.com",
		},
	}))
}

func (s *S) Test_TeamTokenService_FindByUserToken_ValidatePermissions(c *check.C) {
	r1, err := permission.NewRole("app-deployer", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions(permission.PermAppDeploy.FullName())
	c.Assert(err, check.IsNil)
	userToken := &userToken{
		user: s.user,
		permissions: []permission.Permission{
			{
				Scheme:  permission.PermAppDeploy,
				Context: permission.Context(permTypes.CtxApp, "myapp"),
			},
		},
	}
	t1, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:    s.team.Name,
		TokenID: "t1",
	}, userToken)
	c.Assert(err, check.IsNil)
	t2, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:    s.team.Name,
		TokenID: "t2",
	}, userToken)
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), t1.TokenID, "app-deployer", "myapp")
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.AddRole(context.TODO(), t2.TokenID, "app-deployer", "myapp2")
	c.Assert(err, check.IsNil)
	teamTokens, err := servicemanager.TeamToken.FindByUserToken(context.TODO(), userToken)
	c.Assert(err, check.IsNil)
	c.Assert(tokensForEquals(teamTokens), check.DeepEquals, tokensForEquals([]authTypes.TeamToken{
		{
			Token:        t1.Token,
			TokenID:      "t1",
			Team:         "cobrateam",
			CreatorEmail: "timeredbull@globo.com",
			Roles:        []authTypes.RoleInstance{{Name: "app-deployer", ContextValue: "myapp"}},
		},
		{
			Token:        "",
			TokenID:      "t2",
			Team:         "cobrateam",
			CreatorEmail: "timeredbull@globo.com",
			Roles:        []authTypes.RoleInstance{{Name: "app-deployer", ContextValue: "myapp2"}},
		},
	}))
}

func (s *S) Test_TeamTokenService_FindByUserToken_Empty(c *check.C) {
	userToken := &userToken{
		user: s.user,
	}
	teamTokens, err := servicemanager.TeamToken.FindByUserToken(context.TODO(), userToken)
	c.Assert(err, check.IsNil)
	c.Assert(teamTokens, check.HasLen, 0)
}

func (s *S) Test_TeamTokenService_Update_Description(c *check.C) {
	_, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:    s.team.Name,
		TokenID: "t1",
	}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	token, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), "t1")
	c.Assert(err, check.IsNil)
	updatedToken, err := servicemanager.TeamToken.Update(context.TODO(), authTypes.TeamTokenUpdateArgs{
		TokenID:     "t1",
		Description: "xyz",
	}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	expected := authTypes.TeamToken{
		Team:         "cobrateam",
		Description:  "xyz",
		Token:        token.Token,
		TokenID:      "t1",
		CreatorEmail: s.user.Email,
		CreatedAt:    token.CreatedAt,
		ExpiresAt:    time.Time{},
	}
	c.Assert(updatedToken, check.DeepEquals, expected)
	t, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), "t1")
	c.Assert(err, check.IsNil)
	c.Assert(t, check.DeepEquals, expected)
}

func (s *S) Test_TeamTokenService_Update_Regenerate(c *check.C) {
	_, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:        s.team.Name,
		TokenID:     "t1",
		Description: "abc",
	}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	token, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), "t1")
	c.Assert(err, check.IsNil)
	updatedToken, err := servicemanager.TeamToken.Update(context.TODO(), authTypes.TeamTokenUpdateArgs{
		TokenID:    "t1",
		Regenerate: true,
	}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	c.Assert(updatedToken.Token, check.Not(check.Equals), token.Token)
	expected := authTypes.TeamToken{
		Team:         "cobrateam",
		Description:  "abc",
		Token:        updatedToken.Token,
		TokenID:      "t1",
		CreatorEmail: s.user.Email,
		CreatedAt:    token.CreatedAt,
		ExpiresAt:    time.Time{},
	}
	c.Assert(updatedToken, check.DeepEquals, expected)
	t, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), "t1")
	c.Assert(err, check.IsNil)
	c.Assert(t, check.DeepEquals, expected)
}

func (s *S) Test_TeamTokenService_Update_Expires(c *check.C) {
	token, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{
		Team:        s.team.Name,
		TokenID:     "t1",
		Description: "abc",
	}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	updatedToken, err := servicemanager.TeamToken.Update(context.TODO(), authTypes.TeamTokenUpdateArgs{
		TokenID:   "t1",
		ExpiresIn: 60 * 60,
	}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	c.Assert(updatedToken.Description, check.Equals, "abc")
	c.Assert(updatedToken.Token, check.Equals, token.Token)
	c.Assert(updatedToken.ExpiresAt.IsZero(), check.Equals, false)
	t, err := servicemanager.TeamToken.FindByTokenID(context.TODO(), "t1")
	c.Assert(err, check.IsNil)
	c.Assert(t.ExpiresAt.IsZero(), check.Equals, false)

	updatedToken, err = servicemanager.TeamToken.Update(context.TODO(), authTypes.TeamTokenUpdateArgs{
		TokenID:   "t1",
		ExpiresIn: 0,
	}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	c.Assert(updatedToken.ExpiresAt.IsZero(), check.Equals, false)

	updatedToken, err = servicemanager.TeamToken.Update(context.TODO(), authTypes.TeamTokenUpdateArgs{
		TokenID:   "t1",
		ExpiresIn: -1,
	}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	c.Assert(updatedToken.ExpiresAt.IsZero(), check.Equals, true)
}

func (s *S) Test_TeamToken_Permissions(c *check.C) {
	r1, err := permission.NewRole("app-deployer", "app", "")
	c.Assert(err, check.IsNil)
	err = r1.AddPermissions("app.read", "app.deploy")
	c.Assert(err, check.IsNil)
	r2, err := permission.NewRole("app-updater", "app", "")
	c.Assert(err, check.IsNil)
	err = r2.AddPermissions("app.update")
	c.Assert(err, check.IsNil)
	token := &teamToken{
		Team: s.team.Name,
		Roles: []authTypes.RoleInstance{
			{Name: "app-deployer", ContextValue: "myapp"},
			{Name: "app-updater", ContextValue: "myapp"},
		},
	}
	perms, err := token.Permissions()
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.HasLen, 3)
	sort.Slice(perms, func(i, j int) bool { return perms[i].Scheme.FullName() < perms[j].Scheme.FullName() })
	c.Assert(perms, check.DeepEquals, []permission.Permission{
		{Scheme: permission.PermAppDeploy, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppRead, Context: permission.Context(permTypes.CtxApp, "myapp")},
		{Scheme: permission.PermAppUpdate, Context: permission.Context(permTypes.CtxApp, "myapp")},
	})
}

func (s *S) Test_TeamToken_RemoveTokenWithApps(c *check.C) {
	var appListCalled bool
	servicemanager.App = &appTypes.MockAppService{
		OnList: func(filter *appTypes.Filter) ([]appTypes.App, error) {
			appListCalled = true
			c.Assert(filter, check.DeepEquals, &appTypes.Filter{UserOwner: "my-awesome-token@token.tsuru.invalid"})
			return []appTypes.App{&appTypes.MockApp{Name: "my-app1"}}, nil
		},
	}
	token, err := servicemanager.TeamToken.Create(context.TODO(), authTypes.TeamTokenCreateArgs{TokenID: "my-awesome-token", Team: s.team.Name}, &userToken{user: s.user})
	c.Assert(err, check.IsNil)
	err = servicemanager.TeamToken.Delete(context.TODO(), token.TokenID)
	c.Assert(appListCalled, check.Equals, true)
	c.Assert(err, check.DeepEquals, authTypes.ErrCannotRemoveTeamTokenWhoOwnsApps)
}

func (s *S) Test_IsEmailFromTeamToken(c *check.C) {
	tests := []struct {
		email    string
		expected bool
	}{
		{email: "tsuru@tsuru.io"},
		{email: "my-token@tsuru.io"},
		{email: "my-awesome-token@token.tsuru.invalid", expected: true},
		{email: "my-awesome-token@my.company.invalid"},
		{email: "tsuru@token.tsuru.invalid", expected: true},
	}
	for _, tt := range tests {
		got := IsEmailFromTeamToken(tt.email)
		c.Assert(got, check.DeepEquals, tt.expected)
	}
}
