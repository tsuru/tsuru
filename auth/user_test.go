// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"bytes"
	"code.google.com/p/go.crypto/bcrypt"
	stderrors "errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"runtime"
	"strings"
	"sync"
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
	c.Assert(err, gocheck.Equals, ErrUserNotFound)
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

func (s *S) TestUserStartPasswordReset(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	defer s.server.Reset()
	u := User{Email: "thank@alanis.com", Password: "123456"}
	err = u.StartPasswordReset()
	c.Assert(err, gocheck.IsNil)
	defer conn.PasswordTokens().Remove(bson.M{"useremail": u.Email})
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
	defer s.server.Reset()
	u := User{Email: "blues@rush.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	p := u.Password
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	err = u.StartPasswordReset()
	c.Assert(err, gocheck.IsNil)
	time.Sleep(1e6) // Let the email flow
	var token passwordToken
	err = s.conn.PasswordTokens().Find(bson.M{"useremail": u.Email}).One(&token)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.PasswordTokens().Remove(bson.M{"useremail": u.Email})
	err = u.ResetPassword(token.Token)
	c.Assert(err, gocheck.IsNil)
	u2, _ := GetUserByEmail(u.Email)
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
	u := User{Email: "profecia@raul.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	t, err := createPasswordToken(&u)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.PasswordTokens().Remove(bson.M{"_id": t.Token})
	u2 := User{Email: "tsuru@globo.com"}
	err = u2.ResetPassword(t.Token)
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
}

func (s *S) TestResetPasswordEmptyToken(c *gocheck.C) {
	u := User{Email: "presto@rush.com"}
	err := u.ResetPassword("")
	c.Assert(err, gocheck.Equals, ErrInvalidToken)
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

func (s *S) TestCreateTokenRemoveOldTokens(c *gocheck.C) {
	config.Set("auth:max-simultaneous-sessions", 2)
	u := User{Email: "para@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	defer s.conn.Tokens().RemoveAll(bson.M{"useremail": u.Email})
	t1, err := newUserToken(&u)
	c.Assert(err, gocheck.IsNil)
	t2 := t1
	t2.Token += "aa"
	err = s.conn.Tokens().Insert(t1, t2)
	_, err = u.CreateToken("123456")
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
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err = u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	cost = 0
	tokenExpire = 0
	_, err = u.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
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

func (s *S) TestRemoveUnknownKey(c *gocheck.C) {
	u := &User{Email: "shine@pinkfloyd.com", Keys: nil}
	err := u.RemoveKey(Key{Content: "my-key"})
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Key not found")
}

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

func (s *S) TestUserAllowedApps(c *gocheck.C) {
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

func (s *S) TestSendEmail(c *gocheck.C) {
	defer s.server.Reset()
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, gocheck.IsNil)
	s.server.Lock()
	defer s.server.Unlock()
	m := s.server.MailBox[0]
	c.Assert(m.To, gocheck.DeepEquals, []string{"something@tsuru.io"})
	c.Assert(m.From, gocheck.Equals, "root")
	c.Assert(m.Data, gocheck.DeepEquals, []byte("Hello world!\r\n"))
}

func (s *S) TestSendEmailUndefinedSMTPServer(c *gocheck.C) {
	old, _ := config.Get("smtp:server")
	defer config.Set("smtp:server", old)
	config.Unset("smtp:server")
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Setting "smtp:server" is not defined`)
}

func (s *S) TestSendEmailUndefinedUser(c *gocheck.C) {
	old, _ := config.Get("smtp:user")
	defer config.Set("smtp:user", old)
	config.Unset("smtp:user")
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Setting "smtp:user" is not defined`)
}

func (s *S) TestSendEmailUndefinedSMTPPassword(c *gocheck.C) {
	defer s.server.Reset()
	old, _ := config.Get("smtp:password")
	defer config.Set("smtp:password", old)
	config.Unset("smtp:password")
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, gocheck.IsNil)
	s.server.Lock()
	defer s.server.Unlock()
	m := s.server.MailBox[0]
	c.Assert(m.To, gocheck.DeepEquals, []string{"something@tsuru.io"})
	c.Assert(m.From, gocheck.Equals, "root")
	c.Assert(m.Data, gocheck.DeepEquals, []byte("Hello world!\r\n"))
}

func (s *S) TestSMTPServer(c *gocheck.C) {
	var tests = []struct {
		input   string
		output  string
		failure error
	}{
		{"smtp.gmail.com", "smtp.gmail.com:25", nil},
		{"smtp.gmail.com:465", "smtp.gmail.com:465", nil},
		{"", "", stderrors.New(`Setting "smtp:server" is not defined`)},
	}
	old, _ := config.Get("smtp:server")
	defer config.Set("smtp:server", old)
	for _, t := range tests {
		config.Set("smtp:server", t.input)
		server, err := smtpServer()
		c.Check(err, gocheck.DeepEquals, t.failure)
		c.Check(server, gocheck.Equals, t.output)
	}
}

func (s *S) TestGeneratePassword(c *gocheck.C) {
	go runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	passwords := make([]string, 1000)
	var wg sync.WaitGroup
	for i := range passwords {
		wg.Add(1)
		go func(i int) {
			passwords[i] = generatePassword(8)
			wg.Done()
		}(i)
	}
	wg.Wait()
	first := passwords[0]
	for _, p := range passwords[1:] {
		c.Check(p, gocheck.Not(gocheck.Equals), first)
	}
}

func (s *S) TestListKeysShouldCallGandalfAPI(c *gocheck.C) {
	h := testHandler{content: `{"mypckey":"ssh-rsa keystuff keycomment"}`}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	keys, err := u.ListKeys()
	c.Assert(err, gocheck.IsNil)
	expected := map[string]string{"mypckey": "ssh-rsa keystuff keycomment"}
	c.Assert(expected, gocheck.DeepEquals, keys)
	c.Assert(h.url[0], gocheck.Equals, "/user/wolverine@xmen.com/keys")
	c.Assert(h.method[0], gocheck.Equals, "GET")
}

func (s *S) TestListKeysGandalfAPIError(c *gocheck.C) {
	h := testBadHandler{content: "some terrible error"}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	keys, err := u.ListKeys()
	c.Assert(keys, gocheck.DeepEquals, map[string]string(nil))
	c.Assert(err.Error(), gocheck.Equals, "some terrible error\n")
}
