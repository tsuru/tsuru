// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"crypto"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	"github.com/tsuru/tsuru/validation"
)

func IsEmailFromTeamToken(email string) bool {
	return strings.HasSuffix(email, fmt.Sprintf("@%s", authTypes.TsuruTokenEmailDomain))
}

type teamToken authTypes.TeamToken

var (
	_ authTypes.Token      = &teamToken{}
	_ authTypes.NamedToken = &teamToken{}
)

func (t *teamToken) GetValue() string {
	return t.Token
}

func (t *teamToken) User(ctx context.Context) (*authTypes.User, error) {
	return &authTypes.User{
		Email:     fmt.Sprintf("%s@%s", t.TokenID, authTypes.TsuruTokenEmailDomain),
		Quota:     quota.UnlimitedQuota,
		Roles:     t.Roles,
		FromToken: true,
	}, nil
}

func (t *teamToken) GetUserName() string {
	return t.GetTokenName()
}

func (t *teamToken) GetTokenName() string {
	return t.TokenID
}

func (t *teamToken) Engine() string {
	return "team"
}

func (t *teamToken) Permissions(ctx context.Context) ([]permTypes.Permission, error) {
	return expandRolePermissions(ctx, t.Roles)
}

type teamTokenService struct {
	storage authTypes.TeamTokenStorage
}

func TeamTokenService() (authTypes.TeamTokenService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &teamTokenService{
		storage: dbDriver.TeamTokenStorage,
	}, nil
}

func (s *teamTokenService) Authenticate(ctx context.Context, header string) (authTypes.Token, error) {
	tokenStr, err := ParseToken(header)
	if err != nil {
		return nil, err
	}
	storedToken, err := s.storage.FindByToken(ctx, tokenStr)
	if err != nil {
		if err == authTypes.ErrTeamTokenNotFound {
			err = ErrInvalidToken
		}
		return nil, err
	}
	now := time.Now()
	if !storedToken.ExpiresAt.IsZero() && storedToken.ExpiresAt.Before(now) {
		return nil, authTypes.ErrTeamTokenExpired
	}
	err = s.storage.UpdateLastAccess(ctx, tokenStr)
	if err != nil {
		return nil, err
	}
	token := teamToken(*storedToken)
	return &token, nil
}

func (s *teamTokenService) Delete(ctx context.Context, tokenID string) error {
	token, err := s.storage.FindByTokenID(ctx, tokenID)
	if err != nil {
		return err
	}
	tt := teamToken(*token)
	u, err := tt.User(ctx)
	if err != nil {
		return err
	}
	apps, err := servicemanager.App.List(ctx, &appTypes.Filter{UserOwner: u.Email})
	if err != nil {
		return err
	}
	if len(apps) > 0 {
		return authTypes.ErrCannotRemoveTeamTokenWhoOwnsApps
	}
	return s.storage.Delete(ctx, tokenID)
}

func (s *teamTokenService) Create(ctx context.Context, args authTypes.TeamTokenCreateArgs, token authTypes.Token) (authTypes.TeamToken, error) {
	u, err := token.User(ctx)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	_, err = servicemanager.Team.FindByName(ctx, args.Team)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	now := time.Now().UTC()
	resultToken := authTypes.TeamToken{
		Token:        generateToken(args.Team, crypto.SHA256),
		TokenID:      args.TokenID,
		Description:  args.Description,
		Team:         args.Team,
		CreatedAt:    now,
		CreatorEmail: u.Email,
	}
	if args.ExpiresIn != 0 {
		resultToken.ExpiresAt = now.Add(time.Duration(args.ExpiresIn) * time.Second)
	}
	if resultToken.TokenID == "" {
		resultToken.TokenID = fmt.Sprintf("%s-%s", resultToken.Team, resultToken.Token[:5])
	}
	if !validation.ValidateName(resultToken.TokenID) {
		return authTypes.TeamToken{}, errors.New("invalid token_id")
	}
	err = s.storage.Insert(ctx, resultToken)
	return resultToken, err
}

func (s *teamTokenService) AddRole(ctx context.Context, tokenID string, roleName, contextValue string) error {
	_, err := permission.FindRole(ctx, roleName)
	if err != nil {
		return err
	}
	token, err := s.storage.FindByTokenID(ctx, tokenID)
	if err != nil {
		return err
	}

	newRoleInstance := authTypes.RoleInstance{
		Name:         roleName,
		ContextValue: contextValue,
	}

	for _, currentRole := range token.Roles {
		if currentRole == newRoleInstance {
			return nil
		}
	}

	token.Roles = append(token.Roles, newRoleInstance)
	return s.storage.Update(ctx, *token)
}

func (s *teamTokenService) RemoveRole(ctx context.Context, tokenID string, roleName, contextValue string) error {
	token, err := s.storage.FindByTokenID(ctx, tokenID)
	if err != nil {
		return err
	}
	for i := 0; i < len(token.Roles); i++ {
		r := token.Roles[i]
		if r.Name == roleName && r.ContextValue == contextValue {
			token.Roles = append(token.Roles[:i], token.Roles[i+1:]...)
			i--
		}
	}
	return s.storage.Update(ctx, *token)
}

func (s *teamTokenService) FindByTokenID(ctx context.Context, tokenID string) (authTypes.TeamToken, error) {
	t, err := s.storage.FindByTokenID(ctx, tokenID)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	return *t, nil
}

func getTokenTeams(ctx context.Context, t Token) []string {
	var teams []string
	contexts := permission.ContextsForPermission(ctx, t, permission.PermTeamTokenRead, permTypes.CtxGlobal, permTypes.CtxTeam)
	for _, ctx := range contexts {
		if ctx.CtxType == permTypes.CtxGlobal {
			teams = nil
			break
		}
		if ctx.CtxType == permTypes.CtxTeam {
			teams = append(teams, ctx.Value)
		}
	}
	return teams
}

func (s *teamTokenService) Update(ctx context.Context, args authTypes.TeamTokenUpdateArgs, t authTypes.Token) (authTypes.TeamToken, error) {
	token, err := s.storage.FindByTokenID(ctx, args.TokenID)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	if args.Description != "" {
		token.Description = args.Description
	}
	if args.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().UTC().Add(time.Duration(args.ExpiresIn) * time.Second)
	} else if args.ExpiresIn < 0 {
		token.ExpiresAt = time.Time{}
	}
	if args.Regenerate {
		token.Token = generateToken(token.Team, crypto.SHA256)
	}
	err = s.storage.Update(ctx, *token)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	userPerms, err := t.Permissions(ctx)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	canView, err := canViewTokenValue(ctx, userPerms, token)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	if !canView {
		token.Token = ""
	}
	return *token, nil
}

func (s *teamTokenService) Info(ctx context.Context, tokenID string, t authTypes.Token) (authTypes.TeamToken, error) {
	token, err := s.storage.FindByTokenID(ctx, tokenID)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	userPerms, err := t.Permissions(ctx)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	canView, err := canViewTokenValue(ctx, userPerms, token)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	if !canView {
		token.Token = ""
	}
	return *token, nil
}

func (s *teamTokenService) FindByUserToken(ctx context.Context, t authTypes.Token) ([]authTypes.TeamToken, error) {
	teamTokens, err := s.storage.FindByTeams(ctx, getTokenTeams(ctx, t))
	if err != nil {
		return nil, err
	}
	userPerms, err := t.Permissions(ctx)
	if err != nil {
		return nil, err
	}
	for i, teamToken := range teamTokens {
		canView, err := canViewTokenValue(ctx, userPerms, &teamToken)
		if err != nil {
			return nil, err
		}
		if !canView {
			teamTokens[i].Token = ""
		}
	}
	return teamTokens, nil
}

func canUseRole(ctx context.Context, userPerms []permTypes.Permission, roleName, contextValue string) (bool, error) {
	role, err := permission.FindRole(ctx, roleName)
	if err != nil {
		return false, err
	}
	perms := role.PermissionsFor(contextValue)
	for _, p := range perms {
		if !permission.CheckFromPermList(userPerms, p.Scheme, p.Context) {
			return false, nil
		}
	}
	return true, nil
}

func canViewTokenValue(ctx context.Context, userPerms []permTypes.Permission, teamToken *authTypes.TeamToken) (bool, error) {
	for _, roleInstance := range teamToken.Roles {
		canUse, err := canUseRole(ctx, userPerms, roleInstance.Name, roleInstance.ContextValue)
		if err != nil {
			return false, err
		}
		if !canUse {
			return false, nil
		}
	}
	return true, nil
}
