// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"github.com/tsuru/tsuru/auth"
	"launchpad.net/gocheck"
)

func (s *S) TestNativeLoginWithoutEmail(c *gocheck.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.Equals, ErrMissingEmailError)
}

func (s *S) TestNativeLoginWithoutPassword(c *gocheck.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "a@a.com"
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.Equals, ErrMissingPasswordError)
}

func (s *S) TestNativeLogin(c *gocheck.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "123456"
	token, err := scheme.Login(params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(token.GetAppName(), gocheck.Equals, "")
	c.Assert(token.GetValue(), gocheck.Not(gocheck.Equals), "")
	c.Assert(token.IsAppToken(), gocheck.Equals, false)
	u, err := token.User()
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Email, gocheck.Equals, "timeredbull@globo.com")
}

func (s *S) TestNativeLoginWrongPassword(c *gocheck.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "xxxxxx"
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.NotNil)
	_, isAuthFail := err.(auth.AuthenticationFailure)
	c.Assert(isAuthFail, gocheck.Equals, true)
}

func (s *S) TestNativeLoginInvalidUser(c *gocheck.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "xxxxxxx@globo.com"
	params["password"] = "xxxxxx"
	_, err := scheme.Login(params)
	c.Assert(err, gocheck.Equals, auth.ErrUserNotFound)
}
