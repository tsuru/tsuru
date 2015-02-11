// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"bytes"
	"strings"
	"time"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestNativeLoginWithoutEmail(c *check.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, ErrMissingEmailError)
}

func (s *S) TestNativeLoginWithoutPassword(c *check.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "a@a.com"
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, ErrMissingPasswordError)
}

func (s *S) TestNativeLogin(c *check.C) {
	scheme := NativeScheme{}
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
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "xxxxxx"
	_, err := scheme.Login(params)
	c.Assert(err, check.NotNil)
	_, isAuthFail := err.(auth.AuthenticationFailure)
	c.Assert(isAuthFail, check.Equals, true)
}

func (s *S) TestNativeLoginInvalidUser(c *check.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "xxxxxxx@globo.com"
	params["password"] = "xxxxxx"
	_, err := scheme.Login(params)
	c.Assert(err, check.Equals, auth.ErrUserNotFound)
}

func (s *S) TestNativeCreateNoPassword(c *check.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com"}
	_, err := scheme.Create(user)
	c.Assert(err, check.Equals, ErrInvalidPassword)
}

func (s *S) TestNativeCreateNoEmail(c *check.C) {
	scheme := NativeScheme{}
	user := &auth.User{Password: "123455"}
	_, err := scheme.Create(user)
	c.Assert(err, check.Equals, ErrInvalidEmail)
}

func (s *S) TestNativeCreateInvalidPassword(c *check.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123"}
	_, err := scheme.Create(user)
	c.Assert(err, check.Equals, ErrInvalidPassword)
}

func (s *S) TestNativeCreateExistingEmail(c *check.C) {
	existingUser := auth.User{Email: "x@x.com"}
	existingUser.Create()
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	_, err := scheme.Create(user)
	c.Assert(err, check.Equals, ErrEmailRegistered)
}

func (s *S) TestNativeCreate(c *check.C) {
	scheme := NativeScheme{}
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

func (s *S) TestChangePasswordMissmatch(c *check.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	_, err := scheme.Create(user)
	c.Assert(err, check.IsNil)
	token, err := scheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	err = scheme.ChangePassword(token, "1234567", "999999")
	c.Assert(err, check.Equals, ErrPasswordMismatch)
}

func (s *S) TestChangePassword(c *check.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	_, err := scheme.Create(user)
	c.Assert(err, check.IsNil)
	token, err := scheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	err = scheme.ChangePassword(token, "123456", "999999")
	c.Assert(err, check.IsNil)
	_, err = scheme.Login(map[string]string{"email": user.Email, "password": "999999"})
	c.Assert(err, check.IsNil)
}

func (s *S) TestStartPasswordReset(c *check.C) {
	scheme := NativeScheme{}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer s.server.Reset()
	u := auth.User{Email: "thank@alanis.com"}
	err = scheme.StartPasswordReset(&u)
	c.Assert(err, check.IsNil)
	var token passwordToken
	err = conn.PasswordTokens().Find(bson.M{"useremail": u.Email}).One(&token)
	c.Assert(err, check.IsNil)
	time.Sleep(1e9) // Let the email flow.
	s.server.Lock()
	defer s.server.Unlock()
	c.Assert(s.server.MailBox, check.HasLen, 1)
	m := s.server.MailBox[0]
	c.Assert(m.From, check.Equals, "root")
	c.Assert(m.To, check.DeepEquals, []string{u.Email})
	var buf bytes.Buffer
	err = resetEmailData.Execute(&buf, token)
	c.Assert(err, check.IsNil)
	expected := strings.Replace(buf.String(), "\n", "\r\n", -1) + "\r\n"
	c.Assert(string(m.Data), check.Equals, expected)
}

func (s *S) TestResetPassword(c *check.C) {
	scheme := NativeScheme{}
	defer s.server.Reset()
	u := auth.User{Email: "blues@rush.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	p := u.Password
	err = scheme.StartPasswordReset(&u)
	c.Assert(err, check.IsNil)
	time.Sleep(1e6) // Let the email flow
	var token passwordToken
	err = s.conn.PasswordTokens().Find(bson.M{"useremail": u.Email}).One(&token)
	c.Assert(err, check.IsNil)
	err = scheme.ResetPassword(&u, token.Token)
	c.Assert(err, check.IsNil)
	u2, _ := auth.GetUserByEmail(u.Email)
	c.Assert(u2.Password, check.Not(check.Equals), p)
	time.Sleep(1e9) // Let the email flow
	s.server.Lock()
	defer s.server.Unlock()
	c.Assert(s.server.MailBox, check.HasLen, 2)
	m := s.server.MailBox[1]
	c.Assert(m.From, check.Equals, "root")
	c.Assert(m.To, check.DeepEquals, []string{u.Email})
	var buf bytes.Buffer
	err = passwordResetConfirm.Execute(&buf, map[string]string{"email": u.Email, "password": ""})
	c.Assert(err, check.IsNil)
	expected := strings.Replace(buf.String(), "\n", "\r\n", -1) + "\r\n"
	lines := strings.Split(string(m.Data), "\r\n")
	lines[len(lines)-4] = ""
	c.Assert(strings.Join(lines, "\r\n"), check.Equals, expected)
	err = s.conn.PasswordTokens().Find(bson.M{"useremail": u.Email}).One(&token)
	c.Assert(err, check.IsNil)
	c.Assert(token.Used, check.Equals, true)
}

func (s *S) TestResetPasswordThirdToken(c *check.C) {
	scheme := NativeScheme{}
	u := auth.User{Email: "profecia@raul.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	t, err := createPasswordToken(&u)
	c.Assert(err, check.IsNil)
	defer s.conn.PasswordTokens().Remove(bson.M{"_id": t.Token})
	u2 := auth.User{Email: "tsuru@globo.com"}
	err = scheme.ResetPassword(&u2, t.Token)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestResetPasswordEmptyToken(c *check.C) {
	scheme := NativeScheme{}
	u := auth.User{Email: "presto@rush.com"}
	err := scheme.ResetPassword(&u, "")
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestNativeRemove(c *check.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "123456"
	token, err := scheme.Login(params)
	c.Assert(err, check.IsNil)
	u, err := token.User()
	c.Assert(err, check.IsNil)
	err = scheme.Remove(u)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	var tokens []Token
	err = conn.Tokens().Find(bson.M{"useremail": "timeredbull@globo.com"}).All(&tokens)
	c.Assert(err, check.IsNil)
	c.Assert(len(tokens), check.Equals, 0)
	_, err = auth.GetUserByEmail("timeredbull@globo.com")
	c.Assert(err, check.Equals, auth.ErrUserNotFound)
}
