// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
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
	// TODO: Allow creation of api tokens with a subset of app's
	// permissions.
	return BaseTokenPermission(t)
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

func AppTokenAuth(token string) (Token, error) {
	t, err := AppTokenService().FindByToken(token)
	if err != nil {
		if err == authTypes.ErrAppTokenNotFound {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	appToken := AppToken(*t)
	return &appToken, nil
}
