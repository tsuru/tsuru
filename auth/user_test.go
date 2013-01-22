// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha512"
	"fmt"
	"github.com/globocom/config"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"time"
)

func (s *S) TestCreateUser(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	var result User
	collection := s.conn.Users()
	err = collection.Find(bson.M{"email": u.Email}).One(&result)
	c.Assert(err, IsNil)
	c.Assert(result.Email, Equals, u.Email)
}

func (s *S) TestCreateUserHashesThePasswordUsingPBKDF2SHA512AndSalt(c *C) {
	loadConfig()
	salt := []byte(salt)
	expectedPassword := fmt.Sprintf("%x", pbkdf2.Key([]byte("123456"), salt, 4096, len(salt)*8, sha512.New))
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	var result User
	collection := s.conn.Users()
	err = collection.Find(bson.M{"email": u.Email}).One(&result)
	c.Assert(err, IsNil)
	c.Assert(result.Password, Equals, expectedPassword)
}

func (s *S) TestCreateUserReturnsErrorWhenTryingToCreateAUserWithDuplicatedEmail(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	err = u.Create()
	c.Assert(err, NotNil)
}

func (s *S) TestCreateUserUndefinedSaltPanics(c *C) {
	old, err := config.Get("auth:salt")
	c.Assert(err, IsNil)
	defer config.Set("auth:salt", old)
	err = config.Unset("auth:salt")
	c.Assert(err, IsNil)
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	defer func() {
		r := recover()
		c.Assert(r, NotNil)
	}()
	u.Create()
}

func (s *S) TestGetUserByEmail(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	u = User{Email: "wolverine@xmen.com"}
	err = u.Get()
	c.Assert(err, IsNil)
	c.Assert(u.Email, Equals, "wolverine@xmen.com")
}

func (s *S) TestGetUserReturnsErrorWhenNoUserIsFound(c *C) {
	u := User{Email: "unknown@globo.com"}
	err := u.Get()
	c.Assert(err, NotNil)
}

func (s *S) TestUpdateUser(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	u.Password = "1234"
	err = u.Update()
	c.Assert(err, IsNil)
	err = u.Get()
	c.Assert(u.Password, Equals, "1234")
}

func (s *S) TestUserLoginReturnsTrueIfThePasswordMatches(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	u.HashPassword()
	c.Assert(u.Login("123"), Equals, true)
}

func (s *S) TestUserLoginReturnsFalseIfThePasswordDoesNotMatch(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	u.HashPassword()
	c.Assert(u.Login("1234"), Equals, false)
}

func (s *S) TestNewTokenIsStoredInUser(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	u.Create()
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	t, err := u.CreateToken()
	c.Assert(err, IsNil)
	c.Assert(u.Email, Equals, "wolverine@xmen.com")
	c.Assert(u.Tokens[0].Token, Equals, t.Token)
}

func (s *S) TestNewTokenReturnsErroWhenUserReferenceDoesNotContainsEmail(c *C) {
	u := User{}
	t, err := newToken(&u)
	c.Assert(t, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Impossible to generate tokens for users without email$")
}

func (s *S) TestNewTokenReturnsErrorWhenUserIsNil(c *C) {
	t, err := newToken(nil)
	c.Assert(t, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User is nil$")
}

func (s *S) TestNewTokenWithoutTokenKey(c *C) {
	old, err := config.Get("auth:token-key")
	c.Assert(err, IsNil)
	defer config.Set("auth:token-key", old)
	err = config.Unset("auth:token-key")
	c.Assert(err, IsNil)
	t, err := newToken(&User{Email: "gopher@golang.org"})
	c.Assert(t, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, `Setting "auth:token-key" is undefined.`)
}

func (s *S) TestCreateTokenShouldSaveTheTokenInUserInTheDatabase(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	err = u.Get()
	c.Assert(err, IsNil)
	_, err = u.CreateToken()
	c.Assert(err, IsNil)
	var result User
	collection := s.conn.Users()
	err = collection.Find(nil).One(&result)
	c.Assert(err, IsNil)
	c.Assert(result.Tokens[0].Token, NotNil)
}

func (s *S) TestCreateTokenShouldReturnErrorIfTheProvidedUserDoesNotHaveEmailDefined(c *C) {
	u := User{Password: "123"}
	_, err := u.CreateToken()
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User does not have an email$")
}

func (s *S) TestGetUserByToken(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	err = u.Get()
	c.Assert(err, IsNil)
	t, err := u.CreateToken()
	c.Assert(err, IsNil)
	user, err := GetUserByToken(t.Token)
	c.Assert(err, IsNil)
	c.Assert(user.Email, Equals, u.Email)
}

func (s *S) TestGetUserByTokenShouldReturnErrorWhenTheGivenTokenDoesNotExist(c *C) {
	user, err := GetUserByToken("i don't exist")
	c.Assert(user, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Token not found$")
}

func (s *S) TestGetUserByTokenShouldReturnErrorWhenTheGivenTokenHasExpired(c *C) {
	collection := s.conn.Users()
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	err = u.Get()
	c.Assert(err, IsNil)
	t, err := u.CreateToken()
	c.Assert(err, IsNil)
	u.Tokens[0].ValidUntil = time.Now().Add(-24 * time.Hour)
	err = collection.Update(bson.M{"email": "wolverine@xmen.com"}, u)
	user, err := GetUserByToken(t.Token)
	c.Assert(user, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Token has expired$")
}

func (s *S) TestGetUserByTokenDoesNotFailWhenTheTokenIsValid(c *C) {
	u := User{
		Email:    "masterof@puppets.com",
		Password: "123",
		Tokens: []Token{
			{
				Token:      "abcd",
				ValidUntil: time.Now().Add(-24 * time.Hour),
			},
		},
	}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	t, err := u.CreateToken()
	c.Assert(err, IsNil)
	user, err := GetUserByToken(t.Token)
	c.Assert(err, IsNil)
	c.Assert(user.Email, Equals, "masterof@puppets.com")
}

func (s *S) TestAddKeyAddsAKeyToTheUser(c *C) {
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.AddKey(Key{Content: "my-key"})
	c.Assert(err, IsNil)
	c.Assert(u, HasKey, "my-key")
}

func (s *S) TestRemoveKeyRemovesAKeyFromTheUser(c *C) {
	u := &User{Email: "shineon@pinkfloyd.com", Keys: []Key{{Content: "my-key"}}}
	err := u.RemoveKey(Key{Content: "my-key"})
	c.Assert(err, IsNil)
	c.Assert(u, Not(HasKey), "my-key")
}

func (s *S) TestCheckTokenReturnErrorIfTheTokenIsOmited(c *C) {
	u, err := CheckToken("")
	c.Assert(u, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^You must provide the token$")
}

func (s *S) TestCheckTokenReturnErrorIfTheTokenIsInvalid(c *C) {
	u, err := CheckToken("invalid")
	c.Assert(u, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Invalid token$")
}

func (s *S) TestCheckTokenReturnTheUserIfTheTokenIsValid(c *C) {
	u, e := CheckToken(s.token.Token)
	c.Assert(e, IsNil)
	c.Assert(u.Email, Equals, s.user.Email)
}

func (s *S) TestLoadConfigSalt(c *C) {
	configuredSalt, err := config.GetString("auth:salt")
	c.Assert(err, IsNil)
	loadConfig()
	c.Assert(salt, Equals, configuredSalt)
}

func (s *S) TestLoadConfigUndefinedSalt(c *C) {
	key := "auth:salt"
	oldValue, err := config.Get(key)
	c.Assert(err, IsNil)
	err = config.Unset(key)
	c.Assert(err, IsNil)
	defer config.Set(key, oldValue)
	err = loadConfig()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, `Setting "auth:salt" is undefined.`)
	c.Assert(salt, Equals, "")
}

func (s *S) TestLoadConfigTokenExpire(c *C) {
	configuredToken, err := config.Get("auth:token-expire-days")
	c.Assert(err, IsNil)
	expected := time.Duration(int64(configuredToken.(int)) * 24 * int64(time.Hour))
	loadConfig()
	c.Assert(tokenExpire, Equals, expected)
}

func (s *S) TestLoadConfigUndefinedTokenExpire(c *C) {
	tokenExpire = 0
	key := "auth:token-expire-days"
	oldConfig, err := config.Get(key)
	c.Assert(err, IsNil)
	err = config.Unset(key)
	c.Assert(err, IsNil)
	defer config.Set(key, oldConfig)
	err = loadConfig()
	c.Assert(err, IsNil)
	c.Assert(tokenExpire, Equals, defaultExpiration)
}

func (s *S) TestLoadConfigShouldPanicIfTheTokenExpireDaysIsNotInteger(c *C) {
	oldValue, err := config.Get("auth:token-expire-days")
	c.Assert(err, IsNil)
	config.Set("auth:token-expire-days", "abacaxi")
	defer func() {
		config.Set("auth:token-expire-days", oldValue)
		r := recover()
		c.Assert(r, NotNil)
	}()
	loadConfig()
}

func (s *S) TestLoadConfigTokenKey(c *C) {
	configuredKey, err := config.Get("auth:token-key")
	c.Assert(err, IsNil)
	loadConfig()
	c.Assert(tokenKey, Equals, configuredKey)
}

func (s *S) TestLoadConfigUndefineTokenKey(c *C) {
	key := "auth:token-key"
	oldConfig, err := config.Get(key)
	c.Assert(err, IsNil)
	err = config.Unset(key)
	c.Assert(err, IsNil)
	defer config.Set(key, oldConfig)
	err = loadConfig()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, `Setting "auth:token-key" is undefined.`)
	c.Assert(tokenKey, Equals, "")
}

func (s *S) TestLoadConfigDontOverride(c *C) {
	tokenKey = "something"
	salt = "salt"
	err := loadConfig()
	c.Assert(err, IsNil)
	c.Assert(tokenKey, Equals, "something")
	c.Assert(salt, Equals, "salt")
}

func (s *S) TestTeams(c *C) {
	u := User{Email: "me@tsuru.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	s.team.AddUser(&u)
	err = s.conn.Teams().Update(bson.M{"_id": s.team.Name}, s.team)
	c.Assert(err, IsNil)
	defer func(u *User, t *Team) {
		t.RemoveUser(u)
		s.conn.Teams().Update(bson.M{"_id": t.Name}, t)
	}(&u, s.team)
	t := Team{Name: "abc", Users: []string{u.Email}}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	teams, err := u.Teams()
	c.Assert(err, IsNil)
	c.Assert(teams, HasLen, 2)
	c.Assert(teams[0].Name, Equals, s.team.Name)
	c.Assert(teams[1].Name, Equals, t.Name)
}

func (s *S) TestFindKeyReturnsKeyWithNameAndContent(c *C) {
	u := User{Email: "me@tsuru.com", Password: "123", Keys: []Key{{Name: "me@tsuru.com-1", Content: "ssh-rsa somekey me@tsuru.com"}}}
	err := u.Create()
	c.Assert(err, IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	k, index := u.FindKey(Key{Content: u.Keys[0].Content})
	c.Assert(index, Equals, 0)
	c.Assert(k.Name, Equals, u.Keys[0].Name)
}

func (s *S) TestIsAdminReturnsTrueWhenUserHasATeamNamedWithAdminTeamConf(c *C) {
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, IsNil)
	t := Team{Name: adminTeamName, Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(&t)
	c.Assert(err, IsNil)
	defer s.conn.Teams().RemoveId(t.Name)
	c.Assert(s.user.IsAdmin(), Equals, true)
}

func (s *S) TestIsAdminReturnsFalseWhenUserDoNotHaveATeamNamedWithAdminTeamConf(c *C) {
	c.Assert(s.user.IsAdmin(), Equals, false)
}
