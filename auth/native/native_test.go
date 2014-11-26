// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"bytes"
	"strings"
	"time"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/mgo.v2/bson"
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

func (s *S) TestStartPasswordReset(c *gocheck.C) {
	scheme := NativeScheme{}
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	defer s.server.Reset()
	u := auth.User{Email: "thank@alanis.com"}
	err = scheme.StartPasswordReset(&u)
	c.Assert(err, gocheck.IsNil)
	var token passwordToken
	err = conn.PasswordTokens().Find(bson.M{"useremail": u.Email}).One(&token)
	c.Assert(err, gocheck.IsNil)
	time.Sleep(1e9) // Let the email flow.
	s.server.Lock()
	defer s.server.Unlock()
	c.Assert(s.server.MailBox, gocheck.HasLen, 1)
	m := s.server.MailBox[0]
	c.Assert(m.From, gocheck.Equals, "root")
	c.Assert(m.To, gocheck.DeepEquals, []string{u.Email})
	var buf bytes.Buffer
	err = resetEmailData.Execute(&buf, token)
	c.Assert(err, gocheck.IsNil)
	expected := strings.Replace(buf.String(), "\n", "\r\n", -1) + "\r\n"
	c.Assert(string(m.Data), gocheck.Equals, expected)
}

func (s *S) TestResetPassword(c *gocheck.C) {
	scheme := NativeScheme{}
	defer s.server.Reset()
	u := auth.User{Email: "blues@rush.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	p := u.Password
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	err = scheme.StartPasswordReset(&u)
	c.Assert(err, gocheck.IsNil)
	time.Sleep(1e6) // Let the email flow
	var token passwordToken
	err = s.conn.PasswordTokens().Find(bson.M{"useremail": u.Email}).One(&token)
	c.Assert(err, gocheck.IsNil)
	err = scheme.ResetPassword(&u, token.Token)
	c.Assert(err, gocheck.IsNil)
	u2, _ := auth.GetUserByEmail(u.Email)
	c.Assert(u2.Password, gocheck.Not(gocheck.Equals), p)
	time.Sleep(1e9) // Let the email flow
	s.server.Lock()
	defer s.server.Unlock()
	c.Assert(s.server.MailBox, gocheck.HasLen, 2)
	m := s.server.MailBox[1]
	c.Assert(m.From, gocheck.Equals, "root")
	c.Assert(m.To, gocheck.DeepEquals, []string{u.Email})
	var buf bytes.Buffer
	err = passwordResetConfirm.Execute(&buf, map[string]string{"email": u.Email, "password": ""})
	c.Assert(err, gocheck.IsNil)
	expected := strings.Replace(buf.String(), "\n", "\r\n", -1) + "\r\n"
	lines := strings.Split(string(m.Data), "\r\n")
	lines[len(lines)-4] = ""
	c.Assert(strings.Join(lines, "\r\n"), gocheck.Equals, expected)
	err = s.conn.PasswordTokens().Find(bson.M{"useremail": u.Email}).One(&token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(token.Used, gocheck.Equals, true)
}

func (s *S) TestResetPasswordThirdToken(c *gocheck.C) {
	scheme := NativeScheme{}
	u := auth.User{Email: "profecia@raul.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	t, err := createPasswordToken(&u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.PasswordTokens().Remove(bson.M{"_id": t.Token})
	u2 := auth.User{Email: "tsuru@globo.com"}
	err = scheme.ResetPassword(&u2, t.Token)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestResetPasswordEmptyToken(c *gocheck.C) {
	scheme := NativeScheme{}
	u := auth.User{Email: "presto@rush.com"}
	err := scheme.ResetPassword(&u, "")
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestNativeRemove(c *gocheck.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "123456"
	token, err := scheme.Login(params)
	c.Assert(err, gocheck.IsNil)
	u, err := token.User()
	c.Assert(err, gocheck.IsNil)
	err = scheme.Remove(u)
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	var tokens []Token
	err = conn.Tokens().Find(bson.M{"useremail": "timeredbull@globo.com"}).All(&tokens)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(tokens), gocheck.Equals, 0)
	_, err = auth.GetUserByEmail("timeredbull@globo.com")
	c.Assert(err, gocheck.Equals, auth.ErrUserNotFound)
}
