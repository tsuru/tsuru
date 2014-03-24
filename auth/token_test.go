// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto"
	"encoding/json"
	"fmt"
	"github.com/tsuru/config"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"sync"
	"time"
)

func (s *S) TestTokenCannotRepeat(c *gocheck.C) {
	input := "user-token"
	tokens := make([]string, 10)
	var wg sync.WaitGroup
	for i := range tokens {
		wg.Add(1)
		go func(i int) {
			tokens[i] = token(input, crypto.MD5)
			wg.Done()
		}(i)
	}
	wg.Wait()
	reference := tokens[0]
	for _, t := range tokens[1:] {
		c.Check(t, gocheck.Not(gocheck.Equals), reference)
	}
}

func (s *S) TestNewUserToken(c *gocheck.C) {
	u := User{Email: "girl@mj.com"}
	t, err := newUserToken(&u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Expires, gocheck.Equals, tokenExpire)
	c.Assert(t.UserEmail, gocheck.Equals, u.Email)
}

func (s *S) TestNewTokenReturnsErrorWhenUserReferenceDoesNotContainsEmail(c *gocheck.C) {
	u := User{}
	t, err := newUserToken(&u)
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Impossible to generate tokens for users without email$")
}

func (s *S) TestNewTokenReturnsErrorWhenUserIsNil(c *gocheck.C) {
	t, err := newUserToken(nil)
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^User is nil$")
}

func (s *S) TestGetToken(c *gocheck.C) {
	t, err := GetToken("bearer " + s.token.Token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Token, gocheck.Equals, s.token.Token)
}

func (s *S) TestGetTokenEmptyToken(c *gocheck.C) {
	u, err := GetToken("bearer tokenthatdoesnotexist")
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}

func (s *S) TestGetTokenNotFound(c *gocheck.C) {
	t, err := GetToken("bearer invalid")
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}

func (s *S) TestGetTokenInvalid(c *gocheck.C) {
	t, err := GetToken("invalid")
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}

func (s *S) TestGetExpiredToken(c *gocheck.C) {
	t, err := CreateApplicationToken("tsuru-healer")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": t.Token})
	t.Creation = time.Now().Add(-24 * time.Hour)
	t.Expires = time.Hour
	s.conn.Tokens().Update(bson.M{"token": t.Token}, t)
	t2, err := GetToken(t.Token)
	c.Assert(t2, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}

func (s *S) TestCreateApplicationToken(c *gocheck.C) {
	t, err := CreateApplicationToken("tsuru-healer")
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, gocheck.NotNil)
	defer s.conn.Tokens().Remove(bson.M{"token": t.Token})
	n, err := s.conn.Tokens().Find(t).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
	c.Assert(t.AppName, gocheck.Equals, "tsuru-healer")
}

func (s *S) TestTokenMarshalJSON(c *gocheck.C) {
	valid := time.Now()
	t := Token{
		Token:     "12saii",
		Creation:  valid,
		Expires:   time.Hour,
		UserEmail: "something@something.com",
		AppName:   "myapp",
	}
	b, err := json.Marshal(&t)
	c.Assert(err, gocheck.IsNil)
	want := fmt.Sprintf(`{"token":"12saii","creation":%q,"expires":%d,"email":"something@something.com","app":"myapp"}`,
		valid.Format(time.RFC3339Nano), time.Hour)
	c.Assert(string(b), gocheck.Equals, want)
}

func (s *S) TestTokenGetUser(c *gocheck.C) {
	u, err := s.token.User()
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Email, gocheck.Equals, s.user.Email)
}

func (s *S) TestTokenGetUserUnknownEmail(c *gocheck.C) {
	t := Token{UserEmail: "something@something.com"}
	u, err := t.User()
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestDeleteToken(c *gocheck.C) {
	t, err := CreateApplicationToken("tsuru-healer")
	c.Assert(err, gocheck.IsNil)
	err = DeleteToken(t.Token)
	c.Assert(err, gocheck.IsNil)
	_, err = GetToken("bearer " + t.Token)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}

func (s *S) TestCreatePasswordToken(c *gocheck.C) {
	u := User{Email: "pure@alanis.com"}
	t, err := createPasswordToken(&u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.UserEmail, gocheck.Equals, u.Email)
	c.Assert(t.Used, gocheck.Equals, false)
	var dbToken passwordToken
	err = s.conn.PasswordTokens().Find(bson.M{"_id": t.Token}).One(&dbToken)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbToken.Token, gocheck.Equals, t.Token)
	c.Assert(dbToken.UserEmail, gocheck.Equals, t.UserEmail)
	c.Assert(dbToken.Used, gocheck.Equals, t.Used)
}

func (s *S) TestCreatePasswordTokenErrors(c *gocheck.C) {
	var tests = []struct {
		input *User
		want  string
	}{
		{nil, "User is nil"},
		{&User{}, "User email is empty"},
	}
	for _, t := range tests {
		token, err := createPasswordToken(t.input)
		c.Check(token, gocheck.IsNil)
		c.Check(err, gocheck.NotNil)
		c.Check(err.Error(), gocheck.Equals, t.want)
	}
}

func (s *S) TestPasswordTokenUser(c *gocheck.C) {
	u := User{Email: "need@who.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	t, err := createPasswordToken(&u)
	c.Assert(err, gocheck.IsNil)
	u2, err := t.user()
	u2.Keys = u.Keys
	c.Assert(err, gocheck.IsNil)
	c.Assert(*u2, gocheck.DeepEquals, u)
}

func (s *S) TestGetPasswordToken(c *gocheck.C) {
	u := User{Email: "porcelain@opeth.com"}
	t, err := createPasswordToken(&u)
	c.Assert(err, gocheck.IsNil)
	t2, err := getPasswordToken(t.Token)
	t2.Creation = t.Creation
	c.Assert(err, gocheck.IsNil)
	c.Assert(t2, gocheck.DeepEquals, t)
}

func (s *S) TestGetPasswordTokenUnknown(c *gocheck.C) {
	t, err := getPasswordToken("what??")
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}

func (s *S) TestGetPasswordUsedToken(c *gocheck.C) {
	u := User{Email: "porcelain@opeth.com"}
	t, err := createPasswordToken(&u)
	c.Assert(err, gocheck.IsNil)
	t.Used = true
	err = s.conn.PasswordTokens().UpdateId(t.Token, t)
	c.Assert(err, gocheck.IsNil)
	t2, err := getPasswordToken(t.Token)
	c.Assert(t2, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}

func (s *S) TestPasswordTokensAreValidFor24Hours(c *gocheck.C) {
	u := User{Email: "porcelain@opeth.com"}
	t, err := createPasswordToken(&u)
	c.Assert(err, gocheck.IsNil)
	t.Creation = time.Now().Add(-24 * time.Hour)
	err = s.conn.PasswordTokens().UpdateId(t.Token, t)
	c.Assert(err, gocheck.IsNil)
	t2, err := getPasswordToken(t.Token)
	c.Assert(t2, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Invalid token")
}

func (s *S) TestParseToken(c *gocheck.C) {
	t, err := parseToken("type token")
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, gocheck.Equals, "token")
	t, err = parseToken("token")
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
	c.Assert(t, gocheck.Equals, "")
	t, err = parseToken("type ble ble")
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
	c.Assert(t, gocheck.Equals, "")
	t, err = parseToken("")
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
	c.Assert(t, gocheck.Equals, "")
}

func (s *S) TestRemoveOld(c *gocheck.C) {
	config.Set("auth:max-simultaneous-sessions", 6)
	defer config.Unset("auth:max-simultaneous-sessions")
	user := "removeme@tsuru.io"
	defer s.conn.Tokens().RemoveAll(bson.M{"useremail": user})
	initial := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 30; i++ {
		token := Token{
			Token:     fmt.Sprintf("blastoise-%d", i),
			Expires:   100 * 24 * time.Hour,
			Creation:  initial.Add(time.Duration(i) * time.Hour),
			UserEmail: user,
		}
		err := s.conn.Tokens().Insert(token)
		c.Check(err, gocheck.IsNil)
	}
	err := removeOldTokens(user)
	c.Assert(err, gocheck.IsNil)
	var tokens []Token
	err = s.conn.Tokens().Find(bson.M{"useremail": user}).All(&tokens)
	c.Assert(err, gocheck.IsNil)
	c.Assert(tokens, gocheck.HasLen, 6)
	names := make([]string, len(tokens))
	for i := range tokens {
		names[i] = tokens[i].Token
	}
	expected := []string{
		"blastoise-24", "blastoise-25", "blastoise-26",
		"blastoise-27", "blastoise-28", "blastoise-29",
	}
	c.Assert(names, gocheck.DeepEquals, expected)
}

func (s *S) TestRemoveOldNothingToRemove(c *gocheck.C) {
	config.Set("auth:max-simultaneous-sessions", 6)
	defer config.Unset("auth:max-simultaneous-sessions")
	user := "removeme@tsuru.io"
	defer s.conn.Tokens().RemoveAll(bson.M{"useremail": user})
	t := Token{
		Token:     "blablabla",
		UserEmail: user,
		Creation:  time.Now(),
		Expires:   time.Hour,
	}
	err := s.conn.Tokens().Insert(t)
	c.Assert(err, gocheck.IsNil)
	err = removeOldTokens(user)
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Tokens().Find(bson.M{"useremail": user}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 1)
}

func (s *S) TestRemoveOldWithoutSetting(c *gocheck.C) {
	err := removeOldTokens("something@tsuru.io")
	c.Assert(err, gocheck.NotNil)
}
