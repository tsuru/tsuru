// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"launchpad.net/gocheck"
)

type TestScheme struct{}

func (t TestScheme) AppLogin(appName string) (Token, error) {
	return nil, nil
}
func (t TestScheme) Login(params map[string]string) (Token, error) {
	return nil, nil
}
func (t TestScheme) Logout(token string) error {
	return nil
}
func (t TestScheme) Auth(token string) (Token, error) {
	return nil, nil
}
func (t TestScheme) Info() (SchemeInfo, error) {
	return nil, nil
}
func (t TestScheme) Name() string {
	return "test"
}
func (t TestScheme) Create(u *User) (*User, error) {
	return nil, nil
}
func (t TestScheme) Remove(token Token) error {
	return nil
}

func (s *S) TestRegisterScheme(c *gocheck.C) {
	instance := TestScheme{}
	RegisterScheme("x", instance)
	defer UnregisterScheme("x")
	c.Assert(schemes["x"], gocheck.Equals, instance)
}

func (s *S) TestUnregisterScheme(c *gocheck.C) {
	instance := TestScheme{}
	RegisterScheme("x", instance)
	UnregisterScheme("x")
	c.Assert(schemes["x"], gocheck.Equals, nil)
}

func (s *S) TestGetScheme(c *gocheck.C) {
	instance := TestScheme{}
	RegisterScheme("x", instance)
	defer UnregisterScheme("x")
	scheme, err := GetScheme("x")
	c.Assert(err, gocheck.IsNil)
	c.Assert(scheme, gocheck.Equals, instance)
}

func (s *S) TestGetSchemeInvalidScheme(c *gocheck.C) {
	_, err := GetScheme("x")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Unknown scheme: "x".`)
}
