// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"

	"github.com/pkg/errors"
)

type SchemeInfo struct {
	Name string                 `json:"name"`
	Data map[string]interface{} `json:"data"`
}

type Scheme interface {
	Login(ctx context.Context, params map[string]string) (Token, error)
	Logout(ctx context.Context, token string) error
	Auth(ctx context.Context, token string) (Token, error)
	Info(ctx context.Context) (*SchemeInfo, error)
	Create(ctx context.Context, user *User) (*User, error)
	Remove(ctx context.Context, user *User) error
}

type AppScheme interface {
	AppLogin(ctx context.Context, appName string) (Token, error)
	AppLogout(ctx context.Context, token string) error
}

type ManagedScheme interface {
	Scheme
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

func GetAppScheme() AppScheme {
	scheme, err := GetScheme("native")

	if err != nil {
		panic("native scheme is not linked")
	}

	return scheme.(AppScheme)
}
