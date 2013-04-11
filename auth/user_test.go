// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"code.google.com/p/go.crypto/bcrypt"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/errors"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"time"
)

func (s *S) TestCreateUser(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	var result User
	collection := s.conn.Users()
	err = collection.Find(bson.M{"email": u.Email}).One(&result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result.Email, gocheck.Equals, u.Email)
}

func (s *S) TestCreateUserReturnsErrorWhenTryingToCreateAUserWithDuplicatedEmail(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	err = u.Create()
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestGetUserByEmail(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	u2, err := GetUserByEmail(u.Email)
	c.Assert(err, gocheck.IsNil)
	c.Check(u2.Email, gocheck.Equals, u.Email)
	c.Check(u2.Password, gocheck.Equals, u.Password)
}

func (s *S) TestGetUserByEmailReturnsErrorWhenNoUserIsFound(c *gocheck.C) {
	u, err := GetUserByEmail("unknown@globo.com")
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "User not found")
}

func (s *S) TestGetUserByEmailWithInvalidEmail(c *gocheck.C) {
	u, err := GetUserByEmail("unknown")
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Message, gocheck.Equals, emailError)
}

func (s *S) TestUpdateUser(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	u.Password = "1234"
	err = u.Update()
	c.Assert(err, gocheck.IsNil)
	u2, err := GetUserByEmail("wolverine@xmen.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2.Password, gocheck.Equals, "1234")
}

func (s *S) TestUserCheckPasswordUsesBcrypt(c *gocheck.C) {
	u := User{Email: "paradisum", Password: "abcd1234"}
	u.HashPassword()
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte("abcd1234"))
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUserCheckPasswordRightPassword(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	u.HashPassword()
	err := u.CheckPassword("123456")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUserCheckPasswordChecksBcryptPasswordFirst(c *gocheck.C) {
	passwd, err := bcrypt.GenerateFromPassword([]byte("123456"), cost)
	c.Assert(err, gocheck.IsNil)
	u := User{Email: "wolverine@xmen", Password: string(passwd)}
	err = u.CheckPassword("123456")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUserCheckPasswordRehashesThePassword(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	u.Create()
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	err := u.CheckPassword("123456")
	c.Assert(err, gocheck.IsNil)
	other, err := GetUserByEmail(u.Email)
	c.Assert(err, gocheck.IsNil)
	err = bcrypt.CompareHashAndPassword([]byte(other.Password), []byte("123456"))
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUserCheckPasswordReturnsFalseIfThePasswordDoesNotMatch(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	u.HashPassword()
	err := u.CheckPassword("654321")
	c.Assert(err, gocheck.NotNil)
	_, ok := err.(AuthenticationFailure)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestUserCheckPasswordValidatesThePassword(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	u.HashPassword()
	err := u.CheckPassword("123")
	c.Check(err, gocheck.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Check(ok, gocheck.Equals, true)
	c.Check(e.Message, gocheck.Equals, passwordError)
	var p [51]byte
	p[0] = 'a'
	p[50] = 'z'
	err = u.CheckPassword(string(p[:]))
	c.Check(err, gocheck.NotNil)
	e, ok = err.(*errors.ValidationError)
	c.Check(ok, gocheck.Equals, true)
	c.Check(e.Message, gocheck.Equals, passwordError)
}

func (s *S) TestCreateTokenShouldSaveTheTokenInTheDatabase(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	_, err = u.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
	var result Token
	err = s.conn.Tokens().Find(bson.M{"useremail": u.Email}).One(&result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result.Token, gocheck.NotNil)
}

func (s *S) TestCreateTokenReturnsErrorWhenHashCostIsUndefined(c *gocheck.C) {
	err := config.Unset("auth:hash-cost")
	c.Assert(err, gocheck.IsNil)
	defer config.Set("auth:hash-cost", bcrypt.MinCost)
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err = u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	_, err = u.CreateToken("123456")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestCreateTokenShouldReturnErrorIfTheProvidedUserDoesNotHaveEmailDefined(c *gocheck.C) {
	u := User{Password: "123"}
	_, err := u.CreateToken("123")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^User does not have an email$")
}

func (s *S) TestCreateTokenShouldValidateThePassword(c *gocheck.C) {
	u := User{Email: "me@gmail.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	_, err = u.CreateToken("123")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestAddKeyAddsAKeyToTheUser(c *gocheck.C) {
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.AddKey(Key{Content: "my-key"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(u, HasKey, "my-key")
}

func (s *S) TestRemoveKeyRemovesAKeyFromTheUser(c *gocheck.C) {
	u := &User{Email: "shineon@pinkfloyd.com", Keys: []Key{{Content: "my-key"}}}
	err := u.RemoveKey(Key{Content: "my-key"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(u, gocheck.Not(HasKey), "my-key")
}

func (s *S) TestLoadConfigSalt(c *gocheck.C) {
	configuredSalt, err := config.GetString("auth:salt")
	c.Assert(err, gocheck.IsNil)
	loadConfig()
	c.Assert(salt, gocheck.Equals, configuredSalt)
}

func (s *S) TestLoadConfigUndefinedSalt(c *gocheck.C) {
	key := "auth:salt"
	oldValue, err := config.Get(key)
	c.Assert(err, gocheck.IsNil)
	err = config.Unset(key)
	c.Assert(err, gocheck.IsNil)
	defer config.Set(key, oldValue)
	err = loadConfig()
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Setting "auth:salt" is undefined.`)
	c.Assert(salt, gocheck.Equals, "")
}

func (s *S) TestLoadConfigTokenExpire(c *gocheck.C) {
	configuredToken, err := config.Get("auth:token-expire-days")
	c.Assert(err, gocheck.IsNil)
	expected := time.Duration(int64(configuredToken.(int)) * 24 * int64(time.Hour))
	loadConfig()
	c.Assert(tokenExpire, gocheck.Equals, expected)
}

func (s *S) TestLoadConfigUndefinedTokenExpire(c *gocheck.C) {
	tokenExpire = 0
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

func (s *S) TestLoadConfigShouldPanicIfTheTokenExpireDaysIsNotInteger(c *gocheck.C) {
	oldValue, err := config.Get("auth:token-expire-days")
	c.Assert(err, gocheck.IsNil)
	config.Set("auth:token-expire-days", "abacaxi")
	defer func() {
		config.Set("auth:token-expire-days", oldValue)
		r := recover()
		c.Assert(r, gocheck.NotNil)
	}()
	loadConfig()
}

func (s *S) TestLoadConfigTokenKey(c *gocheck.C) {
	configuredKey, err := config.Get("auth:token-key")
	c.Assert(err, gocheck.IsNil)
	loadConfig()
	c.Assert(tokenKey, gocheck.Equals, configuredKey)
}

func (s *S) TestLoadConfigUndefineTokenKey(c *gocheck.C) {
	key := "auth:token-key"
	oldConfig, err := config.Get(key)
	c.Assert(err, gocheck.IsNil)
	err = config.Unset(key)
	c.Assert(err, gocheck.IsNil)
	defer config.Set(key, oldConfig)
	err = loadConfig()
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Setting "auth:token-key" is undefined.`)
	c.Assert(tokenKey, gocheck.Equals, "")
}

func (s *S) TestLoadConfigDontOverride(c *gocheck.C) {
	tokenKey = "something"
	salt = "salt"
	err := loadConfig()
	c.Assert(err, gocheck.IsNil)
	c.Assert(tokenKey, gocheck.Equals, "something")
	c.Assert(salt, gocheck.Equals, "salt")
}

func (s *S) TestLoadConfigCost(c *gocheck.C) {
	key := "auth:hash-cost"
	oldConfig, err := config.Get(key)
	c.Assert(err, gocheck.IsNil)
	config.Set(key, bcrypt.MaxCost)
	defer config.Set(key, oldConfig)
	salt = ""
	tokenKey = ""
	err = loadConfig()
	c.Assert(err, gocheck.IsNil)
	c.Assert(cost, gocheck.Equals, bcrypt.MaxCost)
}

func (s *S) TestLoadConfigCostUndefined(c *gocheck.C) {
	key := "auth:hash-cost"
	oldConfig, err := config.Get(key)
	c.Assert(err, gocheck.IsNil)
	config.Unset(key)
	defer config.Set(key, oldConfig)
	err = loadConfig()
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestLoadConfigCostInvalid(c *gocheck.C) {
	values := []int{bcrypt.MinCost - 1, bcrypt.MaxCost + 1}
	key := "auth:hash-cost"
	oldConfig, _ := config.Get(key)
	defer config.Set(key, oldConfig)
	for _, v := range values {
		salt = ""
		tokenKey = ""
		config.Set(key, v)
		err := loadConfig()
		c.Assert(err, gocheck.NotNil)
		msg := fmt.Sprintf("Invalid value for setting %q: it must be between %d and %d.", key, bcrypt.MinCost, bcrypt.MaxCost)
		c.Assert(err.Error(), gocheck.Equals, msg)
	}
}

func (s *S) TestTeams(c *gocheck.C) {
	u := User{Email: "me@tsuru.com", Password: "123"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	s.team.AddUser(&u)
	err = s.conn.Teams().Update(bson.M{"_id": s.team.Name}, s.team)
	c.Assert(err, gocheck.IsNil)
	defer func(u *User, t *Team) {
		t.RemoveUser(u)
		s.conn.Teams().Update(bson.M{"_id": t.Name}, t)
	}(&u, s.team)
	t := Team{Name: "abc", Users: []string{u.Email}}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	teams, err := u.Teams()
	c.Assert(err, gocheck.IsNil)
	c.Assert(teams, gocheck.HasLen, 2)
	c.Assert(teams[0].Name, gocheck.Equals, s.team.Name)
	c.Assert(teams[1].Name, gocheck.Equals, t.Name)
}

func (s *S) TestFindKeyReturnsKeyWithNameAndContent(c *gocheck.C) {
	u := User{Email: "me@tsuru.com", Password: "123", Keys: []Key{{Name: "me@tsuru.com-1", Content: "ssh-rsa somekey me@tsuru.com"}}}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	k, index := u.FindKey(Key{Content: u.Keys[0].Content})
	c.Assert(index, gocheck.Equals, 0)
	c.Assert(k.Name, gocheck.Equals, u.Keys[0].Name)
}

func (s *S) TestIsAdminReturnsTrueWhenUserHasATeamNamedWithAdminTeamConf(c *gocheck.C) {
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, gocheck.IsNil)
	t := Team{Name: adminTeamName, Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(&t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().RemoveId(t.Name)
	c.Assert(s.user.IsAdmin(), gocheck.Equals, true)
}

func (s *S) TestIsAdminReturnsFalseWhenUserDoNotHaveATeamNamedWithAdminTeamConf(c *gocheck.C) {
	c.Assert(s.user.IsAdmin(), gocheck.Equals, false)
}

type testApp struct {
	Name  string
	Teams []string
}

func (s *S) TestAllowedAppsShouldReturnAllAppsTheUserHasAccess(c *gocheck.C) {
	team := Team{Name: "teamname", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(&team)
	c.Assert(err, gocheck.IsNil)
	a := testApp{Name: "myapp", Teams: []string{s.team.Name}}
	err = s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	a2 := testApp{Name: "myotherapp", Teams: []string{team.Name}}
	err = s.conn.Apps().Insert(&a2)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": bson.M{"$in": []string{a.Name, a2.Name}}})
		s.conn.Teams().RemoveId(team.Name)
	}()
	aApps, err := s.user.AllowedApps()
	c.Assert(aApps, gocheck.DeepEquals, []string{a.Name, a2.Name})
}

func (s *S) TestAllowedAppsByTeam(c *gocheck.C) {
	team := Team{Name: "teamname", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(&team)
	c.Assert(err, gocheck.IsNil)
	a := testApp{Name: "myapp", Teams: []string{s.team.Name}}
	err = s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	a2 := testApp{Name: "otherapp", Teams: []string{team.Name}}
	err = s.conn.Apps().Insert(&a2)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
		s.conn.Teams().RemoveId(team.Name)
	}()
	alwdApps, err := s.user.AllowedAppsByTeam(team.Name)
	c.Assert(alwdApps, gocheck.DeepEquals, []string{a2.Name})
}
