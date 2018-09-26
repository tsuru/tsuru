// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ldap

import (
	authTypes "github.com/tsuru/tsuru/types/auth"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/check.v1"
)

func (s *S) TestNativeLoginWithoutEmail(c *check.C) {
	scheme := LdapNativeScheme{}
	params := make(map[string]string)
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, ErrMissingEmailError)
}

func (s *S) TestNativeLoginWithoutPassword(c *check.C) {
	scheme := LdapNativeScheme{}
	params := make(map[string]string)
	params["email"] = "a@a.com"
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, ErrMissingPasswordError)
}

func (s *S) TestNativeLogin(c *check.C) {
	scheme := LdapNativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "123456"
	token, err := scheme.Login(params)
	c.Assert(err, check.IsNil)
	c.Assert(token.GetAppName(), check.Equals, "")
	c.Assert(token.GetValue(), check.Not(check.Equals), "")
	c.Assert(token.IsAppToken(), check.Equals, false)
	u, err := token.User()
	c.Assert(err, check.IsNil)
	c.Assert(u.Email, check.Equals, "timeredbull@globo.com")
}

func (s *S) TestNativeLoginWrongPassword(c *check.C) {
	scheme := LdapNativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "xxxxxx"
	_, err := scheme.Login(params)
	c.Assert(err, check.NotNil)
	_, isAuthFail := err.(auth.AuthenticationFailure)
	c.Assert(isAuthFail, check.Equals, true)
}

func (s *S) TestNativeLoginInvalidUser(c *check.C) {
	scheme := LdapNativeScheme{}
	params := make(map[string]string)
	params["email"] = "xxxxxxx@globo.com"
	params["password"] = "xxxxxx"
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}

func (s *S) TestNativeCreateNoEmail(c *check.C) {
	scheme := LdapNativeScheme{}
	user := &auth.User{Password: "123455"}
	_, err := scheme.Create(user)
	c.Assert(err, check.Equals, ErrInvalidEmail)
}

func (s *S) TestNativeCreateExistingEmail(c *check.C) {
	existingUser := auth.User{Email: "x@x.com"}
	existingUser.Create()
	scheme := LdapNativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	_, err := scheme.Create(user)
	c.Assert(err, check.Equals, ErrEmailRegistered)
}

func (s *S) TestNativeCreate(c *check.C) {
	scheme := LdapNativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	retUser, err := scheme.Create(user)
	c.Assert(err, check.IsNil)
	c.Assert(retUser, check.Equals, user)
	dbUser, err := auth.GetUserByEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(dbUser.Email, check.Equals, user.Email)
	c.Assert(dbUser.Password, check.Not(check.Equals), "123456")
	c.Assert(dbUser.Password, check.Equals, user.Password)
}

func (s *S) TestResetPasswordEmptyToken(c *check.C) {
	scheme := LdapNativeScheme{}
	u := auth.User{Email: "presto@rush.com"}
	err := scheme.ResetPassword(&u, "")
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestNativeRemove(c *check.C) {
	scheme := LdapNativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "123456"
	token, err := scheme.Login(params)
	c.Assert(err, check.IsNil)
	u, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	err = scheme.Remove(u)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	var tokens []Token
	err = conn.Tokens().Find(bson.M{"useremail": "timeredbull@globo.com"}).All(&tokens)
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 0)
	_, err = auth.GetUserByEmail("timeredbull@globo.com")
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}
