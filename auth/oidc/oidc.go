// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oidc

import (
	"context"
	"errors"

	"github.com/tsuru/tsuru/auth"
)

var (
	errNotImplementedYet             = errors.New("not implemented yet")
	_                    auth.Scheme = &oidcScheme{}
)

func init() {
	auth.RegisterScheme("oidc", &oidcScheme{})
}

type oidcScheme struct{}

func (s *oidcScheme) Name() string {
	return "oidc"
}

func (s *oidcScheme) AppLogin(ctx context.Context, appName string) (auth.Token, error) {
	return nil, errNotImplementedYet
}

func (s *oidcScheme) AppLogout(ctx context.Context, token string) error {
	return errNotImplementedYet
}

func (s *oidcScheme) Login(ctx context.Context, params map[string]string) (auth.Token, error) {
	return nil, errNotImplementedYet
}

func (s *oidcScheme) Logout(ctx context.Context, token string) error {
	return errNotImplementedYet
}

func (s *oidcScheme) Auth(ctx context.Context, token string) (auth.Token, error) {
	return nil, errNotImplementedYet
}

func (s *oidcScheme) Info(ctx context.Context) (auth.SchemeInfo, error) {
	return nil, errNotImplementedYet
}

func (s *oidcScheme) Create(ctx context.Context, user *auth.User) (*auth.User, error) {
	return nil, errNotImplementedYet
}

func (s *oidcScheme) Remove(ctx context.Context, user *auth.User) error {
	return errNotImplementedYet
}
