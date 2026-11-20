// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package peer

import (
	"context"
	"crypto/subtle"
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"

	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

func Auth(ctx context.Context, token string) (auth.Token, error) {
	expectedToken := TokenValue()
	if expectedToken == "" {
		// Fail closed: without a configured peer token there is no peer
		// authentication. There is intentionally no default value, as a
		// source-shipped constant would grant global log read access to
		// anyone who reads the repository.
		return nil, auth.ErrInvalidToken
	}
	parsedToken, err := auth.ParseToken(token)
	if err != nil {
		return nil, err
	}

	if subtle.ConstantTimeCompare([]byte(expectedToken), []byte(parsedToken)) == 1 {
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

func (t *Token) User(ctx context.Context) (*authTypes.User, error) {
	return nil, errors.New("no token user")
}

func (t *Token) GetUserName() string {
	return "peer"
}

func (t *Token) Engine() string {
	return "peer"
}

func (t *Token) Permissions(ctx context.Context) ([]permTypes.Permission, error) {
	return []permTypes.Permission{
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
	return token
}
