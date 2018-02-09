// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"

	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/storage"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

type TeamToken authTypes.TeamToken

var _ Token = &TeamToken{}

func (t *TeamToken) GetValue() string {
	return t.Token
}

func (t *TeamToken) User() (*User, error) {
	return nil, nil
}

func (t *TeamToken) IsAppToken() bool {
	return true
}

func (t *TeamToken) GetUserName() string {
	return ""
}

func (t *TeamToken) GetAppName() string {
	return t.AppName
}

func (t *TeamToken) Permissions() ([]permission.Permission, error) {
	if len(t.Roles) == 0 {
		return BaseTokenPermission(t)
	}
	// TODO: refactoring, this code is almost equal to auth.User.Permissions()
	permissions := []permission.Permission{}
	roles := make(map[string]*permission.Role)
	for _, roleName := range t.Roles {
		role := roles[roleName]
		if role == nil {
			foundRole, err := permission.FindRole(roleName)
			if err != nil && err != permission.ErrRoleNotFound {
				return nil, err
			}
			if foundRole.ContextType != permission.CtxApp {
				return nil, fmt.Errorf("App token can't have permissions outside the app context")
			}
			role = &foundRole
			roles[roleName] = role
		}
		permissions = append(permissions, role.PermissionsFor(t.GetAppName())...)
	}
	return permissions, nil
}

func TeamTokenService() authTypes.TeamTokenService {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil
		}
	}
	return dbDriver.TeamTokenService
}

func TeamTokenAuth(header string) (Token, error) {
	token, err := ParseToken(header)
	if err != nil {
		return nil, err
	}
	t, err := TeamTokenService().Authenticate(token)
	if err != nil {
		if err == authTypes.ErrTeamTokenNotFound {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	appToken := TeamToken(*t)
	return &appToken, nil
}
