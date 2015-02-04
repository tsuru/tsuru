// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"crypto"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestLoadConfigTokenExpire(c *gocheck.C) {
	configuredToken, err := config.Get("auth:token-expire-days")
	c.Assert(err, gocheck.IsNil)
	expected := time.Duration(int64(configuredToken.(int)) * 24 * int64(time.Hour))
	cost = 0
	tokenExpire = 0
	loadConfig()
	c.Assert(tokenExpire, gocheck.Equals, expected)
}

func (s *S) TestLoadConfigUndefinedTokenExpire(c *gocheck.C) {
	tokenExpire = 0
	cost = 0
	key := "auth:token-expire-days"
	oldConfig, err := config.Get(key)
	c.Assert(err, gocheck.IsNil)
	err = config.Unset(key)
	c.Assert(err, gocheck.IsNil)
	defer config.Set(key, oldConfig)
	err = loadConfig()
	c.Assert(err, gocheck.IsNil)
	c.Assert(tokenExpire, gocheck.Equals, defaultExpiration)
}

func (s *S) TestLoadConfigExpireDaysNotInteger(c *gocheck.C) {
	cost = 0
	tokenExpire = 0
	oldValue, err := config.Get("auth:token-expire-days")
	c.Assert(err, gocheck.IsNil)
	config.Set("auth:token-expire-days", "abacaxi")
	defer config.Set("auth:token-expire-days", oldValue)
	err = loadConfig()
	c.Assert(tokenExpire, gocheck.Equals, defaultExpiration)
}

func (s *S) TestLoadConfigCost(c *gocheck.C) {
	key := "auth:hash-cost"
	oldConfig, err := config.Get(key)
	c.Assert(err, gocheck.IsNil)
	config.Set(key, bcrypt.MaxCost)
	defer config.Set(key, oldConfig)
	cost = 0
	tokenExpire = 0
	err = loadConfig()
	c.Assert(err, gocheck.IsNil)
	c.Assert(cost, gocheck.Equals, bcrypt.MaxCost)
}

func (s *S) TestLoadConfigCostUndefined(c *gocheck.C) {
	cost = 0
	tokenExpire = 0
	key := "auth:hash-cost"
	oldConfig, err := config.Get(key)
	c.Assert(err, gocheck.IsNil)
	config.Unset(key)
	defer config.Set(key, oldConfig)
	err = loadConfig()
	c.Assert(err, gocheck.IsNil)
	c.Assert(cost, gocheck.Equals, bcrypt.DefaultCost)
}

func (s *S) TestLoadConfigCostInvalid(c *gocheck.C) {
	values := []int{bcrypt.MinCost - 1, bcrypt.MaxCost + 1}
	key := "auth:hash-cost"
	oldConfig, _ := config.Get(key)
	defer config.Set(key, oldConfig)
	for _, v := range values {
		cost = 0
		tokenExpire = 0
		config.Set(key, v)
		err := loadConfig()
		c.Assert(err, gocheck.NotNil)
		msg := fmt.Sprintf("Invalid value for setting %q: it must be between %d and %d.", key, bcrypt.MinCost, bcrypt.MaxCost)
		c.Assert(err.Error(), gocheck.Equals, msg)
	}
}

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
	u := auth.User{Email: "girl@mj.com"}
	t, err := newUserToken(&u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Expires, gocheck.Equals, tokenExpire)
	c.Assert(t.UserEmail, gocheck.Equals, u.Email)
}

func (s *S) TestNewTokenReturnsErrorWhenUserReferenceDoesNotContainsEmail(c *gocheck.C) {
	u := auth.User{}
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

func (s *S) TestCreateTokenShouldSaveTheTokenInTheDatabase(c *gocheck.C) {
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	_, err = createToken(&u, "123456")
	c.Assert(err, gocheck.IsNil)
	var result Token
	err = s.conn.Tokens().Find(bson.M{"useremail": u.Email}).One(&result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result.Token, gocheck.NotNil)
}

func (s *S) TestCreateTokenRemoveOldTokens(c *gocheck.C) {
	config.Set("auth:max-simultaneous-sessions", 2)
	u := auth.User{Email: "para@xmen.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	defer s.conn.Tokens().RemoveAll(bson.M{"useremail": u.Email})
	t1, err := newUserToken(&u)
	c.Assert(err, gocheck.IsNil)
	t2 := t1
	t2.Token += "aa"
	err = s.conn.Tokens().Insert(t1, t2)
	_, err = createToken(&u, "123456")
	c.Assert(err, gocheck.IsNil)
	ok := make(chan bool, 1)
	go func() {
		for {
			ct, err := s.conn.Tokens().Find(bson.M{"useremail": u.Email}).Count()
			c.Assert(err, gocheck.IsNil)
			if ct == 2 {
				ok <- true
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-ok:
	case <-time.After(2e9):
		c.Fatal("Did not remove old tokens after 2 seconds")
	}
}

func (s *S) TestCreateTokenUsesDefaultCostWhenHasCostIsUndefined(c *gocheck.C) {
	err := config.Unset("auth:hash-cost")
	c.Assert(err, gocheck.IsNil)
	defer config.Set("auth:hash-cost", bcrypt.MinCost)
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	_, err = nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	cost = 0
	tokenExpire = 0
	_, err = createToken(&u, "123456")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestCreateTokenShouldReturnErrorIfTheProvidedUserDoesNotHaveEmailDefined(c *gocheck.C) {
	u := auth.User{Password: "123"}
	_, err := createToken(&u, "123")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^User does not have an email$")
}

func (s *S) TestCreateTokenShouldValidateThePassword(c *gocheck.C) {
	u := auth.User{Email: "me@gmail.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	_, err = createToken(&u, "123")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestGetToken(c *gocheck.C) {
	t, err := getToken("bearer " + s.token.GetValue())
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Token, gocheck.Equals, s.token.GetValue())
}

func (s *S) TestGetTokenEmptyToken(c *gocheck.C) {
	u, err := getToken("bearer tokenthatdoesnotexist")
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetTokenNotFound(c *gocheck.C) {
	t, err := getToken("bearer invalid")
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetTokenInvalid(c *gocheck.C) {
	t, err := getToken("invalid")
	c.Assert(t, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetExpiredToken(c *gocheck.C) {
	t, err := createApplicationToken("tsuru-healer")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": t.Token})
	t.Creation = time.Now().Add(-24 * time.Hour)
	t.Expires = time.Hour
	s.conn.Tokens().Update(bson.M{"token": t.Token}, t)
	t2, err := getToken(t.Token)
	c.Assert(t2, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestDeleteToken(c *gocheck.C) {
	t, err := createApplicationToken("tsuru-healer")
	c.Assert(err, gocheck.IsNil)
	err = deleteToken(t.Token)
	c.Assert(err, gocheck.IsNil)
	_, err = getToken("bearer " + t.Token)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestCreateApplicationToken(c *gocheck.C) {
	t, err := createApplicationToken("tsuru-healer")
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, gocheck.NotNil)
	defer s.conn.Tokens().Remove(bson.M{"token": t.Token})
	n, err := s.conn.Tokens().Find(t).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
	c.Assert(t.AppName, gocheck.Equals, "tsuru-healer")
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

func (s *S) TestTokenIsAppToken(c *gocheck.C) {
	t := Token{AppName: "myapp"}
	isAppToken := t.IsAppToken()
	c.Assert(isAppToken, gocheck.Equals, true)

	t = Token{UserEmail: "something@something.com"}
	isAppToken = t.IsAppToken()
	c.Assert(isAppToken, gocheck.Equals, false)
}

func (s *S) TestUserCheckPasswordUsesBcrypt(c *gocheck.C) {
	u := auth.User{Email: "paradisum", Password: "abcd1234"}
	err := hashPassword(&u)
	c.Assert(err, gocheck.IsNil)
	err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte("abcd1234"))
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUserCheckPasswordRightPassword(c *gocheck.C) {
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	err := hashPassword(&u)
	c.Assert(err, gocheck.IsNil)
	err = checkPassword(u.Password, "123456")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUserCheckPasswordChecksBcryptPasswordFirst(c *gocheck.C) {
	passwd, err := bcrypt.GenerateFromPassword([]byte("123456"), cost)
	c.Assert(err, gocheck.IsNil)
	u := auth.User{Email: "wolverine@xmen", Password: string(passwd)}
	err = checkPassword(u.Password, "123456")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUserCheckPasswordReturnsFalseIfThePasswordDoesNotMatch(c *gocheck.C) {
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	err := hashPassword(&u)
	c.Assert(err, gocheck.IsNil)
	err = checkPassword(u.Password, "654321")
	c.Assert(err, gocheck.NotNil)
	_, ok := err.(auth.AuthenticationFailure)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestUserCheckPasswordValidatesThePassword(c *gocheck.C) {
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	err := hashPassword(&u)
	c.Assert(err, gocheck.IsNil)
	err = checkPassword(u.Password, "123")
	c.Check(err, gocheck.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Check(ok, gocheck.Equals, true)
	c.Check(e.Message, gocheck.Equals, passwordError)
	var p [51]byte
	p[0] = 'a'
	p[50] = 'z'
	err = checkPassword(u.Password, string(p[:]))
	c.Check(err, gocheck.NotNil)
	e, ok = err.(*errors.ValidationError)
	c.Check(ok, gocheck.Equals, true)
	c.Check(e.Message, gocheck.Equals, passwordError)
}

func (s *S) TestDeleteAllTokens(c *gocheck.C) {
	tokens := []Token{
		{Token: "t1", UserEmail: "x@x.com", Creation: time.Now(), Expires: time.Hour},
		{Token: "t2", UserEmail: "x@x.com", Creation: time.Now(), Expires: time.Hour},
		{Token: "t3", UserEmail: "y@y.com", Creation: time.Now(), Expires: time.Hour},
	}
	err := s.conn.Tokens().Insert(tokens[0])
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Tokens().Insert(tokens[1])
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Tokens().Insert(tokens[2])
	c.Assert(err, gocheck.IsNil)
	err = deleteAllTokens("x@x.com")
	c.Assert(err, gocheck.IsNil)
	var tokensDB []Token
	err = s.conn.Tokens().Find(bson.M{"useremail": "x@x.com"}).All(&tokensDB)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(tokensDB), gocheck.Equals, 0)
	err = s.conn.Tokens().Find(bson.M{"useremail": "y@y.com"}).All(&tokensDB)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(tokensDB), gocheck.Equals, 1)
}
