// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"

	authTypes "github.com/tsuru/tsuru/types/auth"
	check "gopkg.in/check.v1"
)

type TestScheme struct{}

var _ Scheme = &TestScheme{}

func (t TestScheme) AppLogin(ctx context.Context, appName string) (Token, error) {
	return nil, nil
}
func (t TestScheme) AppLogout(ctx context.Context, token string) error {
	return nil
}
func (t TestScheme) Login(ctx context.Context, params map[string]string) (Token, error) {
	return nil, nil
}
func (t TestScheme) Logout(ctx context.Context, token string) error {
	return nil
}
func (t TestScheme) Auth(ctx context.Context, token string) (Token, error) {
	return nil, nil
}
func (t TestScheme) Info(ctx context.Context) (*authTypes.SchemeInfo, error) {
	return nil, nil
}
func (t TestScheme) Name() string {
	return "test"
}
func (t TestScheme) Create(ctx context.Context, u *User) (*User, error) {
	return nil, nil
}
func (t TestScheme) Remove(ctx context.Context, u *User) error {
	return nil
}

func (s *S) TestRegisterScheme(c *check.C) {
	instance := TestScheme{}
	RegisterScheme("x", instance)
	defer UnregisterScheme("x")
	c.Assert(schemes["x"], check.Equals, instance)
}

func (s *S) TestUnregisterScheme(c *check.C) {
	instance := TestScheme{}
	RegisterScheme("x", instance)
	UnregisterScheme("x")
	c.Assert(schemes["x"], check.Equals, nil)
}

func (s *S) TestGetScheme(c *check.C) {
	instance := TestScheme{}
	RegisterScheme("x", instance)
	defer UnregisterScheme("x")
	scheme, err := GetScheme("x")
	c.Assert(err, check.IsNil)
	c.Assert(scheme, check.Equals, instance)
}

func (s *S) TestGetSchemeInvalidScheme(c *check.C) {
	_, err := GetScheme("x")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `Unknown auth scheme: "x".`)
}
