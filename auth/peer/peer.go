// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package peer

import (
	"context"
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"

	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

var DefaultToken = "97ef27b972cb168a9875a32cba96010e1eb6a6065e9f4cc5928a7cb66d2a6b86" // sha256 of "tsuru"

func Auth(ctx context.Context, token string) (auth.Token, error) {
	expectedToken := TokenValue()
	parsedToken, err := auth.ParseToken(token)
	if err != nil {
		return nil, err
	}

	if expectedToken == parsedToken {
		return &Token{Token: parsedToken}, nil
	}

	return nil, auth.ErrInvalidToken
}

type Token struct {
	Token string
}

func (t *Token) GetValue() string {
	return t.Token
}

func (t *Token) User() (*authTypes.User, error) {
	return nil, errors.New("no token user")
}

func (t *Token) IsAppToken() bool {
	return false
}

func (t *Token) GetUserName() string {
	return "peer"
}

func (t *Token) GetAppName() string {
	return ""
}

func (t *Token) Engine() string {
	return "peer"
}

func (t *Token) Permissions() ([]permission.Permission, error) {
	return []permission.Permission{
		{
			Scheme:  permission.PermAppReadLog,
			Context: permission.Context(permTypes.CtxGlobal, ""),
		},
		{
			Scheme:  permission.PermJobReadLogs,
			Context: permission.Context(permTypes.CtxGlobal, ""),
		},
	}, nil
}

func TokenValue() string {
	token, _ := config.GetString("auth:peer:token")

	if token == "" {
		return DefaultToken
	}

	return token
}
