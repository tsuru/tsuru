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

type AppToken authTypes.AppToken

var _ Token = &AppToken{}

func (t *AppToken) GetValue() string {
	return t.Token
}

func (t *AppToken) User() (*User, error) {
	return nil, nil
}

func (t *AppToken) IsAppToken() bool {
	return true
}

func (t *AppToken) GetUserName() string {
	return ""
}

func (t *AppToken) GetAppName() string {
	return t.AppName
}

func (t *AppToken) Permissions() ([]permission.Permission, error) {
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

func AppTokenService() authTypes.AppTokenService {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil
		}
	}
	return dbDriver.AppTokenService
}

func AppTokenAuth(header string) (Token, error) {
	token, err := ParseToken(header)
	if err != nil {
		return nil, err
	}
	t, err := AppTokenService().Authenticate(token)
	if err != nil {
		if err == authTypes.ErrAppTokenNotFound {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	appToken := AppToken(*t)
	return &appToken, nil
}
