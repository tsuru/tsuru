// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

// Token type alias exists to ease refactoring while we move auth types to
// types/auth package. Both this type alias and the Convert*User funcs should
// be eliminated once we convert everyone to use authTypes.User.
type Token authTypes.Token

func ConvertOldUser(u *User, err error) (*authTypes.User, error) {
	if u != nil {
		wu := authTypes.User(*u)
		return &wu, err
	}
	return nil, err
}

func ConvertNewUser(u *authTypes.User, err error) (*User, error) {
	if u != nil {
		wu := User(*u)
		return &wu, err
	}
	return nil, err
}

var ErrInvalidToken = errors.New("Invalid token")

// ParseToken extracts token from a header:
// 'type token' or 'token'
func ParseToken(header string) (string, error) {
	s := strings.Split(header, " ")
	var value string
	if len(s) < 3 {
		value = s[len(s)-1]
	}
	if value != "" {
		return value, nil
	}
	return value, ErrInvalidToken
}

func BaseTokenPermission(t Token) ([]permission.Permission, error) {
	if t.IsAppToken() {
		// TODO(cezarsa): Improve handling of app tokens. These permissions
		// listed here are the ones required by deploy-agent and legacy tsuru-
		// unit-agent.
		return []permission.Permission{
			{
				Scheme:  permission.PermAppUpdateUnitRegister,
				Context: permission.Context(permTypes.CtxApp, t.GetAppName()),
			},
			{
				Scheme:  permission.PermAppUpdateLog,
				Context: permission.Context(permTypes.CtxApp, t.GetAppName()),
			},
			{
				Scheme:  permission.PermAppUpdateUnitStatus,
				Context: permission.Context(permTypes.CtxApp, t.GetAppName()),
			},
			{
				Scheme:  permission.PermAppReadDeploy,
				Context: permission.Context(permTypes.CtxApp, t.GetAppName()),
			},
		}, nil
	}
	u, err := ConvertNewUser(t.User())
	if err != nil {
		return nil, err
	}
	return u.Permissions()
}
