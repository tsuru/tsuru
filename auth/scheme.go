// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"

	"github.com/pkg/errors"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

type Scheme interface {
	Auth(ctx context.Context, token string) (Token, error)
	Info(ctx context.Context) (*authTypes.SchemeInfo, error)
}

type MultiScheme interface {
	Infos(ctx context.Context) ([]authTypes.SchemeInfo, error)
}

type UserScheme interface {
	Scheme

	Login(ctx context.Context, params map[string]string) (Token, error)
	Logout(ctx context.Context, token string) error
	Create(ctx context.Context, user *User) (*User, error)
	Remove(ctx context.Context, user *User) error
}

type ManagedScheme interface {
	UserScheme
	StartPasswordReset(ctx context.Context, user *User) error
	ResetPassword(ctx context.Context, user *User, resetToken string) error
	ChangePassword(ctx context.Context, token Token, oldPassword string, newPassword string) error
}

type AuthenticationFailure struct {
	Message string
}

func (a AuthenticationFailure) Error() string {
	if a.Message != "" {
		return a.Message
	}
	return "Authentication failed, wrong password."
}

var schemes = make(map[string]Scheme)

func RegisterScheme(name string, scheme Scheme) {
	schemes[name] = scheme
}

func UnregisterScheme(name string) {
	delete(schemes, name)
}

func GetScheme(name string) (Scheme, error) {
	scheme, ok := schemes[name]
	if !ok {
		return nil, errors.Errorf("Unknown auth scheme: %q.", name)
	}
	return scheme, nil
}
