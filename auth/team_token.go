// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/storage"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/validation"
)

type teamToken authTypes.TeamToken

var (
	_ authTypes.Token      = &teamToken{}
	_ authTypes.NamedToken = &teamToken{}
)

func (t *teamToken) GetValue() string {
	return t.Token
}

func (t *teamToken) User() (*authTypes.User, error) {
	return nil, errors.New("team token is not a user token")
}

func (t *teamToken) IsAppToken() bool {
	return false
}

func (t *teamToken) GetUserName() string {
	return t.GetTokenName()
}

func (t *teamToken) GetTokenName() string {
	return t.TokenID
}

func (t *teamToken) GetAppName() string {
	return ""
}

func (t *teamToken) Permissions() ([]permission.Permission, error) {
	return expandRolePermissions(t.Roles)
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

func (s *teamTokenService) Authenticate(header string) (authTypes.Token, error) {
	tokenStr, err := ParseToken(header)
	if err != nil {
		return nil, err
	}
	storedToken, err := s.storage.FindByToken(tokenStr)
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
	err = s.storage.UpdateLastAccess(tokenStr)
	if err != nil {
		return nil, err
	}
	token := teamToken(*storedToken)
	return &token, nil
}

func (s *teamTokenService) Delete(tokenID string) error {
	return s.storage.Delete(tokenID)
}

func (s *teamTokenService) Create(args authTypes.TeamTokenCreateArgs, token authTypes.Token) (authTypes.TeamToken, error) {
	u, err := token.User()
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	_, err = servicemanager.Team.FindByName(args.Team)
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
	err = s.storage.Insert(resultToken)
	return resultToken, err
}

func (s *teamTokenService) AddRole(tokenID string, roleName, contextValue string) error {
	_, err := permission.FindRole(roleName)
	if err != nil {
		return err
	}
	token, err := s.storage.FindByTokenID(tokenID)
	if err != nil {
		return err
	}
	token.Roles = append(token.Roles, authTypes.RoleInstance{
		Name: roleName, ContextValue: contextValue,
	})
	return s.storage.Update(*token)
}

func (s *teamTokenService) RemoveRole(tokenID string, roleName, contextValue string) error {
	token, err := s.storage.FindByTokenID(tokenID)
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
	return s.storage.Update(*token)
}

func (s *teamTokenService) FindByTokenID(tokenID string) (authTypes.TeamToken, error) {
	t, err := s.storage.FindByTokenID(tokenID)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	return *t, nil
}

func getTokenTeams(t Token) []string {
	var teams []string
	contexts := permission.ContextsForPermission(t, permission.PermTeamTokenRead, permission.CtxGlobal, permission.CtxTeam)
	for _, ctx := range contexts {
		if ctx.CtxType == permission.CtxGlobal {
			teams = nil
			break
		}
		if ctx.CtxType == permission.CtxTeam {
			teams = append(teams, ctx.Value)
		}
	}
	return teams
}

func (s *teamTokenService) Update(args authTypes.TeamTokenUpdateArgs, t authTypes.Token) (authTypes.TeamToken, error) {
	token, err := s.storage.FindByTokenID(args.TokenID)
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
	err = s.storage.Update(*token)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	userPerms, err := t.Permissions()
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	canView, err := canViewTokenValue(userPerms, token)
	if err != nil {
		return authTypes.TeamToken{}, err
	}
	if !canView {
		token.Token = ""
	}
	return *token, nil
}

func (s *teamTokenService) FindByUserToken(t authTypes.Token) ([]authTypes.TeamToken, error) {
	teamTokens, err := s.storage.FindByTeams(getTokenTeams(t))
	if err != nil {
		return nil, err
	}
	userPerms, err := t.Permissions()
	if err != nil {
		return nil, err
	}
	for i, teamToken := range teamTokens {
		canView, err := canViewTokenValue(userPerms, &teamToken)
		if err != nil {
			return nil, err
		}
		if !canView {
			teamTokens[i].Token = ""
		}
	}
	return teamTokens, nil
}

func canUseRole(userPerms []permission.Permission, roleName, contextValue string) (bool, error) {
	role, err := permission.FindRole(roleName)
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

func canViewTokenValue(userPerms []permission.Permission, teamToken *authTypes.TeamToken) (bool, error) {
	for _, roleInstance := range teamToken.Roles {
		canUse, err := canUseRole(userPerms, roleInstance.Name, roleInstance.ContextValue)
		if err != nil {
			return false, err
		}
		if !canUse {
			return false, nil
		}
	}
	return true, nil
}
