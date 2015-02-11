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
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestLoadConfigTokenExpire(c *check.C) {
	configuredToken, err := config.Get("auth:token-expire-days")
	c.Assert(err, check.IsNil)
	expected := time.Duration(int64(configuredToken.(int)) * 24 * int64(time.Hour))
	cost = 0
	tokenExpire = 0
	loadConfig()
	c.Assert(tokenExpire, check.Equals, expected)
}

func (s *S) TestLoadConfigUndefinedTokenExpire(c *check.C) {
	tokenExpire = 0
	cost = 0
	key := "auth:token-expire-days"
	oldConfig, err := config.Get(key)
	c.Assert(err, check.IsNil)
	err = config.Unset(key)
	c.Assert(err, check.IsNil)
	defer config.Set(key, oldConfig)
	err = loadConfig()
	c.Assert(err, check.IsNil)
	c.Assert(tokenExpire, check.Equals, defaultExpiration)
}

func (s *S) TestLoadConfigExpireDaysNotInteger(c *check.C) {
	cost = 0
	tokenExpire = 0
	oldValue, err := config.Get("auth:token-expire-days")
	c.Assert(err, check.IsNil)
	config.Set("auth:token-expire-days", "abacaxi")
	defer config.Set("auth:token-expire-days", oldValue)
	err = loadConfig()
	c.Assert(tokenExpire, check.Equals, defaultExpiration)
}

func (s *S) TestLoadConfigCost(c *check.C) {
	key := "auth:hash-cost"
	oldConfig, err := config.Get(key)
	c.Assert(err, check.IsNil)
	config.Set(key, bcrypt.MaxCost)
	defer config.Set(key, oldConfig)
	cost = 0
	tokenExpire = 0
	err = loadConfig()
	c.Assert(err, check.IsNil)
	c.Assert(cost, check.Equals, bcrypt.MaxCost)
}

func (s *S) TestLoadConfigCostUndefined(c *check.C) {
	cost = 0
	tokenExpire = 0
	key := "auth:hash-cost"
	oldConfig, err := config.Get(key)
	c.Assert(err, check.IsNil)
	config.Unset(key)
	defer config.Set(key, oldConfig)
	err = loadConfig()
	c.Assert(err, check.IsNil)
	c.Assert(cost, check.Equals, bcrypt.DefaultCost)
}

func (s *S) TestLoadConfigCostInvalid(c *check.C) {
	values := []int{bcrypt.MinCost - 1, bcrypt.MaxCost + 1}
	key := "auth:hash-cost"
	oldConfig, _ := config.Get(key)
	defer config.Set(key, oldConfig)
	for _, v := range values {
		cost = 0
		tokenExpire = 0
		config.Set(key, v)
		err := loadConfig()
		c.Assert(err, check.NotNil)
		msg := fmt.Sprintf("Invalid value for setting %q: it must be between %d and %d.", key, bcrypt.MinCost, bcrypt.MaxCost)
		c.Assert(err.Error(), check.Equals, msg)
	}
}

func (s *S) TestTokenCannotRepeat(c *check.C) {
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
		c.Check(t, check.Not(check.Equals), reference)
	}
}

func (s *S) TestNewUserToken(c *check.C) {
	u := auth.User{Email: "girl@mj.com"}
	t, err := newUserToken(&u)
	c.Assert(err, check.IsNil)
	c.Assert(t.Expires, check.Equals, tokenExpire)
	c.Assert(t.UserEmail, check.Equals, u.Email)
}

func (s *S) TestNewTokenReturnsErrorWhenUserReferenceDoesNotContainsEmail(c *check.C) {
	u := auth.User{}
	t, err := newUserToken(&u)
	c.Assert(t, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Impossible to generate tokens for users without email$")
}

func (s *S) TestNewTokenReturnsErrorWhenUserIsNil(c *check.C) {
	t, err := newUserToken(nil)
	c.Assert(t, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^User is nil$")
}

func (s *S) TestRemoveOld(c *check.C) {
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
		c.Check(err, check.IsNil)
	}
	err := removeOldTokens(user)
	c.Assert(err, check.IsNil)
	var tokens []Token
	err = s.conn.Tokens().Find(bson.M{"useremail": user}).All(&tokens)
	c.Assert(err, check.IsNil)
	c.Assert(tokens, check.HasLen, 6)
	names := make([]string, len(tokens))
	for i := range tokens {
		names[i] = tokens[i].Token
	}
	expected := []string{
		"blastoise-24", "blastoise-25", "blastoise-26",
		"blastoise-27", "blastoise-28", "blastoise-29",
	}
	c.Assert(names, check.DeepEquals, expected)
}

func (s *S) TestRemoveOldNothingToRemove(c *check.C) {
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
	c.Assert(err, check.IsNil)
	err = removeOldTokens(user)
	c.Assert(err, check.IsNil)
	count, err := s.conn.Tokens().Find(bson.M{"useremail": user}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *S) TestRemoveOldWithoutSetting(c *check.C) {
	err := removeOldTokens("something@tsuru.io")
	c.Assert(err, check.NotNil)
}

func (s *S) TestCreateTokenShouldSaveTheTokenInTheDatabase(c *check.C) {
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer u.Delete()
	_, err = createToken(&u, "123456")
	c.Assert(err, check.IsNil)
	var result Token
	err = s.conn.Tokens().Find(bson.M{"useremail": u.Email}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Token, check.NotNil)
}

func (s *S) TestCreateTokenRemoveOldTokens(c *check.C) {
	config.Set("auth:max-simultaneous-sessions", 2)
	u := auth.User{Email: "para@xmen.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer u.Delete()
	defer s.conn.Tokens().RemoveAll(bson.M{"useremail": u.Email})
	t1, err := newUserToken(&u)
	c.Assert(err, check.IsNil)
	t2 := t1
	t2.Token += "aa"
	err = s.conn.Tokens().Insert(t1, t2)
	_, err = createToken(&u, "123456")
	c.Assert(err, check.IsNil)
	ok := make(chan bool, 1)
	go func() {
		for {
			ct, err := s.conn.Tokens().Find(bson.M{"useremail": u.Email}).Count()
			c.Assert(err, check.IsNil)
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

func (s *S) TestCreateTokenUsesDefaultCostWhenHasCostIsUndefined(c *check.C) {
	err := config.Unset("auth:hash-cost")
	c.Assert(err, check.IsNil)
	defer config.Set("auth:hash-cost", bcrypt.MinCost)
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	_, err = nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer u.Delete()
	cost = 0
	tokenExpire = 0
	_, err = createToken(&u, "123456")
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateTokenShouldReturnErrorIfTheProvidedUserDoesNotHaveEmailDefined(c *check.C) {
	u := auth.User{Password: "123"}
	_, err := createToken(&u, "123")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^User does not have an email$")
}

func (s *S) TestCreateTokenShouldValidateThePassword(c *check.C) {
	u := auth.User{Email: "me@gmail.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer u.Delete()
	_, err = createToken(&u, "123")
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetToken(c *check.C) {
	t, err := getToken("bearer " + s.token.GetValue())
	c.Assert(err, check.IsNil)
	c.Assert(t.Token, check.Equals, s.token.GetValue())
}

func (s *S) TestGetTokenEmptyToken(c *check.C) {
	u, err := getToken("bearer tokenthatdoesnotexist")
	c.Assert(u, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetTokenNotFound(c *check.C) {
	t, err := getToken("bearer invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetTokenInvalid(c *check.C) {
	t, err := getToken("invalid")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetExpiredToken(c *check.C) {
	t, err := createApplicationToken("tsuru-healer")
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": t.Token})
	t.Creation = time.Now().Add(-24 * time.Hour)
	t.Expires = time.Hour
	s.conn.Tokens().Update(bson.M{"token": t.Token}, t)
	t2, err := getToken(t.Token)
	c.Assert(t2, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestDeleteToken(c *check.C) {
	t, err := createApplicationToken("tsuru-healer")
	c.Assert(err, check.IsNil)
	err = deleteToken(t.Token)
	c.Assert(err, check.IsNil)
	_, err = getToken("bearer " + t.Token)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestCreateApplicationToken(c *check.C) {
	t, err := createApplicationToken("tsuru-healer")
	c.Assert(err, check.IsNil)
	c.Assert(t, check.NotNil)
	defer s.conn.Tokens().Remove(bson.M{"token": t.Token})
	n, err := s.conn.Tokens().Find(t).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	c.Assert(t.AppName, check.Equals, "tsuru-healer")
}

func (s *S) TestTokenGetUser(c *check.C) {
	u, err := s.token.User()
	c.Assert(err, check.IsNil)
	c.Assert(u.Email, check.Equals, s.user.Email)
}

func (s *S) TestTokenGetUserUnknownEmail(c *check.C) {
	t := Token{UserEmail: "something@something.com"}
	u, err := t.User()
	c.Assert(u, check.IsNil)
	c.Assert(err, check.NotNil)
}

func (s *S) TestTokenMarshalJSON(c *check.C) {
	valid := time.Now()
	t := Token{
		Token:     "12saii",
		Creation:  valid,
		Expires:   time.Hour,
		UserEmail: "something@something.com",
		AppName:   "myapp",
	}
	b, err := json.Marshal(&t)
	c.Assert(err, check.IsNil)
	want := fmt.Sprintf(`{"token":"12saii","creation":%q,"expires":%d,"email":"something@something.com","app":"myapp"}`,
		valid.Format(time.RFC3339Nano), time.Hour)
	c.Assert(string(b), check.Equals, want)
}

func (s *S) TestTokenIsAppToken(c *check.C) {
	t := Token{AppName: "myapp"}
	isAppToken := t.IsAppToken()
	c.Assert(isAppToken, check.Equals, true)

	t = Token{UserEmail: "something@something.com"}
	isAppToken = t.IsAppToken()
	c.Assert(isAppToken, check.Equals, false)
}

func (s *S) TestUserCheckPasswordUsesBcrypt(c *check.C) {
	u := auth.User{Email: "paradisum", Password: "abcd1234"}
	err := hashPassword(&u)
	c.Assert(err, check.IsNil)
	err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte("abcd1234"))
	c.Assert(err, check.IsNil)
}

func (s *S) TestUserCheckPasswordRightPassword(c *check.C) {
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	err := hashPassword(&u)
	c.Assert(err, check.IsNil)
	err = checkPassword(u.Password, "123456")
	c.Assert(err, check.IsNil)
}

func (s *S) TestUserCheckPasswordChecksBcryptPasswordFirst(c *check.C) {
	passwd, err := bcrypt.GenerateFromPassword([]byte("123456"), cost)
	c.Assert(err, check.IsNil)
	u := auth.User{Email: "wolverine@xmen", Password: string(passwd)}
	err = checkPassword(u.Password, "123456")
	c.Assert(err, check.IsNil)
}

func (s *S) TestUserCheckPasswordReturnsFalseIfThePasswordDoesNotMatch(c *check.C) {
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	err := hashPassword(&u)
	c.Assert(err, check.IsNil)
	err = checkPassword(u.Password, "654321")
	c.Assert(err, check.NotNil)
	_, ok := err.(auth.AuthenticationFailure)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestUserCheckPasswordValidatesThePassword(c *check.C) {
	u := auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	err := hashPassword(&u)
	c.Assert(err, check.IsNil)
	err = checkPassword(u.Password, "123")
	c.Check(err, check.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Check(ok, check.Equals, true)
	c.Check(e.Message, check.Equals, passwordError)
	var p [51]byte
	p[0] = 'a'
	p[50] = 'z'
	err = checkPassword(u.Password, string(p[:]))
	c.Check(err, check.NotNil)
	e, ok = err.(*errors.ValidationError)
	c.Check(ok, check.Equals, true)
	c.Check(e.Message, check.Equals, passwordError)
}

func (s *S) TestDeleteAllTokens(c *check.C) {
	tokens := []Token{
		{Token: "t1", UserEmail: "x@x.com", Creation: time.Now(), Expires: time.Hour},
		{Token: "t2", UserEmail: "x@x.com", Creation: time.Now(), Expires: time.Hour},
		{Token: "t3", UserEmail: "y@y.com", Creation: time.Now(), Expires: time.Hour},
	}
	err := s.conn.Tokens().Insert(tokens[0])
	c.Assert(err, check.IsNil)
	err = s.conn.Tokens().Insert(tokens[1])
	c.Assert(err, check.IsNil)
	err = s.conn.Tokens().Insert(tokens[2])
	c.Assert(err, check.IsNil)
	err = deleteAllTokens("x@x.com")
	c.Assert(err, check.IsNil)
	var tokensDB []Token
	err = s.conn.Tokens().Find(bson.M{"useremail": "x@x.com"}).All(&tokensDB)
	c.Assert(err, check.IsNil)
	c.Assert(len(tokensDB), check.Equals, 0)
	err = s.conn.Tokens().Find(bson.M{"useremail": "y@y.com"}).All(&tokensDB)
	c.Assert(err, check.IsNil)
	c.Assert(len(tokensDB), check.Equals, 1)
}
