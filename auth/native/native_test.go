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

func (s *S) TestNativeCreateNoPassword(c *gocheck.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com"}
	_, err := scheme.Create(user)
	c.Assert(err, gocheck.Equals, ErrInvalidPassword)
}

func (s *S) TestNativeCreateNoEmail(c *gocheck.C) {
	scheme := NativeScheme{}
	user := &auth.User{Password: "123455"}
	_, err := scheme.Create(user)
	c.Assert(err, gocheck.Equals, ErrInvalidEmail)
}

func (s *S) TestNativeCreateInvalidPassword(c *gocheck.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123"}
	_, err := scheme.Create(user)
	c.Assert(err, gocheck.Equals, ErrInvalidPassword)
}

func (s *S) TestNativeCreateExistingEmail(c *gocheck.C) {
	existingUser := auth.User{Email: "x@x.com"}
	existingUser.Create()
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	_, err := scheme.Create(user)
	c.Assert(err, gocheck.Equals, ErrEmailRegistered)
}

func (s *S) TestNativeCreate(c *gocheck.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	retUser, err := scheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retUser, gocheck.Equals, user)
	dbUser, err := auth.GetUserByEmail(user.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbUser.Email, gocheck.Equals, user.Email)
	c.Assert(dbUser.Password, gocheck.Not(gocheck.Equals), "123456")
	c.Assert(dbUser.Password, gocheck.Equals, user.Password)
}

func (s *S) TestChangePasswordMissmatch(c *gocheck.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	_, err := scheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	token, err := scheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	err = scheme.ChangePassword(token, "1234567", "999999")
	c.Assert(err, gocheck.Equals, ErrPasswordMismatch)
}

func (s *S) TestChangePassword(c *gocheck.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	_, err := scheme.Create(user)
	c.Assert(err, gocheck.IsNil)
	token, err := scheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	err = scheme.ChangePassword(token, "123456", "999999")
	c.Assert(err, gocheck.IsNil)
	_, err = scheme.Login(map[string]string{"email": user.Email, "password": "999999"})
	c.Assert(err, gocheck.IsNil)
}
