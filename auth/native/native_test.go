// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/authtest"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/tsurutest"
	authTypes "github.com/tsuru/tsuru/types/auth"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestNativeLoginWithoutEmail(c *check.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	_, err := scheme.Login(context.TODO(), params)
	c.Assert(err, check.Equals, ErrMissingEmailError)
}

func (s *S) TestNativeLoginWithoutPassword(c *check.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "a@a.com"
	_, err := scheme.Login(context.TODO(), params)
	c.Assert(err, check.Equals, ErrMissingPasswordError)
}

func (s *S) TestNativeLogin(c *check.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "123456"
	token, err := scheme.Login(context.TODO(), params)
	c.Assert(err, check.IsNil)
	c.Assert(token.GetValue(), check.Not(check.Equals), "")
	u, err := token.User()
	c.Assert(err, check.IsNil)
	c.Assert(u.Email, check.Equals, "timeredbull@globo.com")
}

func (s *S) TestNativeLoginWrongPassword(c *check.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "xxxxxx"
	_, err := scheme.Login(context.TODO(), params)
	c.Assert(err, check.NotNil)
	_, isAuthFail := err.(auth.AuthenticationFailure)
	c.Assert(isAuthFail, check.Equals, true)
}

func (s *S) TestNativeLoginInvalidUser(c *check.C) {
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "xxxxxxx@globo.com"
	params["password"] = "xxxxxx"
	_, err := scheme.Login(context.TODO(), params)
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}

func (s *S) TestNativeCreateNoPassword(c *check.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com"}
	_, err := scheme.Create(context.TODO(), user)
	c.Assert(err, check.Equals, ErrInvalidPassword)
}

func (s *S) TestNativeCreateNoEmail(c *check.C) {
	scheme := NativeScheme{}
	user := &auth.User{Password: "123455"}
	_, err := scheme.Create(context.TODO(), user)
	c.Assert(err, check.Equals, ErrInvalidEmail)
}

func (s *S) TestNativeCreateInvalidPassword(c *check.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123"}
	_, err := scheme.Create(context.TODO(), user)
	c.Assert(err, check.Equals, ErrInvalidPassword)
}

func (s *S) TestNativeCreateExistingEmail(c *check.C) {
	existingUser := auth.User{Email: "x@x.com"}
	existingUser.Create(context.TODO())
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	_, err := scheme.Create(context.TODO(), user)
	c.Assert(err, check.Equals, ErrEmailRegistered)
}

func (s *S) TestNativeCreate(c *check.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	retUser, err := scheme.Create(context.TODO(), user)
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
	_, err := scheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	token, err := scheme.Login(context.TODO(), map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	err = scheme.ChangePassword(context.TODO(), token, "1234567", "999999")
	c.Assert(err, check.Equals, ErrPasswordMismatch)
}

func (s *S) TestChangePassword(c *check.C) {
	scheme := NativeScheme{}
	user := &auth.User{Email: "x@x.com", Password: "123456"}
	_, err := scheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	token, err := scheme.Login(context.TODO(), map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	err = scheme.ChangePassword(context.TODO(), token, "123456", "999999")
	c.Assert(err, check.IsNil)
	_, err = scheme.Login(context.TODO(), map[string]string{"email": user.Email, "password": "999999"})
	c.Assert(err, check.IsNil)
}

func (s *S) TestStartPasswordReset(c *check.C) {
	scheme := NativeScheme{}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer s.server.Reset()
	u := auth.User{Email: "thank@alanis.com"}
	err = scheme.StartPasswordReset(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	var token passwordToken
	err = conn.PasswordTokens().Find(bson.M{"useremail": u.Email}).One(&token)
	c.Assert(err, check.IsNil)
	var m authtest.Mail
	err = tsurutest.WaitCondition(time.Second, func() bool {
		s.server.Lock()
		defer s.server.Unlock()
		if len(s.server.MailBox) != 1 {
			return false
		}
		m = s.server.MailBox[0]
		return true
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.From, check.Equals, "root")
	c.Assert(m.To, check.DeepEquals, []string{u.Email})
	var buf bytes.Buffer
	template, err := getEmailResetPasswordTemplate()
	c.Assert(err, check.IsNil)
	err = template.Execute(&buf, token)
	c.Assert(err, check.IsNil)
	expected := strings.Replace(buf.String(), "\n", "\r\n", -1) + "\r\n"
	c.Assert(string(m.Data), check.Equals, expected)
}

func (s *S) TestResetPassword(c *check.C) {
	scheme := NativeScheme{}
	defer s.server.Reset()
	u := auth.User{Email: "blues@rush.com"}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	defer u.Delete()
	p := u.Password
	err = scheme.StartPasswordReset(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(time.Second, func() bool {
		s.server.RLock()
		defer s.server.RUnlock()
		return len(s.server.MailBox) == 1
	})
	c.Assert(err, check.IsNil)
	var token passwordToken
	err = s.conn.PasswordTokens().Find(bson.M{"useremail": u.Email}).One(&token)
	c.Assert(err, check.IsNil)
	err = scheme.ResetPassword(context.TODO(), &u, token.Token)
	c.Assert(err, check.IsNil)
	u2, _ := auth.GetUserByEmail(u.Email)
	c.Assert(u2.Password, check.Not(check.Equals), p)
	var m authtest.Mail
	err = tsurutest.WaitCondition(time.Second, func() bool {
		s.server.RLock()
		defer s.server.RUnlock()
		if len(s.server.MailBox) != 2 {
			return false
		}
		m = s.server.MailBox[1]
		return true
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.From, check.Equals, "root")
	c.Assert(m.To, check.DeepEquals, []string{u.Email})
	var buf bytes.Buffer
	template, err := getEmailResetPasswordSucessfullyTemplate()
	c.Assert(err, check.IsNil)
	err = template.Execute(&buf, map[string]string{"email": u.Email, "password": ""})
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
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	defer u.Delete()
	t, err := createPasswordToken(&u)
	c.Assert(err, check.IsNil)
	defer s.conn.PasswordTokens().Remove(bson.M{"_id": t.Token})
	u2 := auth.User{Email: "tsuru@globo.com"}
	err = scheme.ResetPassword(context.TODO(), &u2, t.Token)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestResetPasswordEmptyToken(c *check.C) {
	scheme := NativeScheme{}
	u := auth.User{Email: "presto@rush.com"}
	err := scheme.ResetPassword(context.TODO(), &u, "")
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestNativeRemove(c *check.C) {
	ctx := context.TODO()
	scheme := NativeScheme{}
	params := make(map[string]string)
	params["email"] = "timeredbull@globo.com"
	params["password"] = "123456"
	token, err := scheme.Login(ctx, params)
	c.Assert(err, check.IsNil)
	u, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	err = scheme.Remove(ctx, u)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	var tokens []Token

	tokensCollection, err := storagev2.TokensCollection()
	c.Assert(err, check.IsNil)

	cursor, err := tokensCollection.Find(ctx, mongoBSON.M{"useremail": "timeredbull@globo.com"})
	c.Assert(err, check.IsNil)

	err = cursor.All(ctx, &tokens)
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 0)
	_, err = auth.GetUserByEmail("timeredbull@globo.com")
	c.Assert(err, check.Equals, authTypes.ErrUserNotFound)
}
