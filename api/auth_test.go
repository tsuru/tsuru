// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/authtest"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/rec/rectest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type AuthSuite struct {
	team   *auth.Team
	user   *auth.User
	token  auth.Token
	server *authtest.SMTPServer
}

var _ = check.Suite(&AuthSuite{})

func (s *AuthSuite) SetUpSuite(c *check.C) {
	var err error
	config.Set("auth:user-registration", true)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_auth_test")
	config.Set("auth:hash-cost", 4)
	config.Set("repo-manager", "fake")
	s.server, err = authtest.NewSMTPServer()
	c.Assert(err, check.IsNil)
	config.Set("smtp:server", s.server.Addr())
	config.Set("smtp:user", "root")
	config.Set("smtp:password", "123456")
	app.Provisioner = provisiontest.NewFakeProvisioner()
	app.AuthScheme = nativeScheme
}

func (s *AuthSuite) TearDownSuite(c *check.C) {
	s.server.Stop()
}

func (s *AuthSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	s.createUserAndTeam(c)
	conn.Platforms().Insert(app.Platform{Name: "python"})
	err = provision.AddPool("test1", false)
	c.Assert(err, check.IsNil)
	config.Set("admin-team", s.team.Name)
}

func (s *AuthSuite) createUserAndTeam(c *check.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, check.IsNil)
	s.team = &auth.Team{
		Name:  "tsuruteam",
		Users: []string{s.user.Email},
	}
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
}

func (s *AuthSuite) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}

type userPresenceChecker struct{}

func (c *userPresenceChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "ContainsUser", Params: []string{"team", "user"}}
}

func (c *userPresenceChecker) Check(params []interface{}, names []string) (bool, string) {
	team, ok := params[0].(*auth.Team)
	if !ok {
		return false, "first parameter should be a pointer to a team instance"
	}

	user, ok := params[1].(*auth.User)
	if !ok {
		return false, "second parameter should be a pointer to a user instance"
	}
	return team.ContainsUser(user), ""
}

type teamLeadPresenceChecker struct{}

func (c *teamLeadPresenceChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "ContainsTeamLead", Params: []string{"team", "user"}}
}

func (c *teamLeadPresenceChecker) Check(params []interface{}, names []string) (bool, string) {
	team, ok := params[0].(*auth.Team)
	if !ok {
		return false, "first parameter should be a pointer to a team instance"
	}

	user, ok := params[1].(*auth.User)
	if !ok {
		return false, "second parameter should be a pointer to a user instance"
	}
	return team.ContainsTeamLead(user), ""
}

var (
	ContainsUser     check.Checker = &userPresenceChecker{}
	ContainsTeamLead check.Checker = &teamLeadPresenceChecker{}
)

func (s *AuthSuite) TestCreateUserHandlerSavesTheUserInTheDatabase(c *check.C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.IsNil)
	user, err := auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, check.IsNil)
	action := rectest.Action{
		Action: "create-user",
		User:   "nobody@globo.com",
	}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(user.Quota, check.DeepEquals, quota.Unlimited)
}

func (s *AuthSuite) TestCreateUserQuota(c *check.C) {
	config.Set("quota:apps-per-user", 1)
	defer config.Unset("quota:apps-per-user")
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.IsNil)
	user, err := auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota.Limit, check.Equals, 1)
	c.Assert(user.Quota.InUse, check.Equals, 0)
}

func (s *AuthSuite) TestCreateUserUnlimitedQuota(c *check.C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.IsNil)
	user, err := auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota, check.DeepEquals, quota.Unlimited)
}

func (s *AuthSuite) TestCreateUserHandlerReturnsStatus201AfterCreateTheUser(c *check.C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, 201)
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorIfReadingBodyFails(c *check.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^.*bad file descriptor$")
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorAndBadRequestIfInvalidJSONIsGiven(c *check.C) {
	b := bytes.NewBufferString(`["invalid json":"i'm invalid"]`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^invalid character.*$")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorAndConflictIfItFailsToCreateUser(c *check.C) {
	u := auth.User{Email: "nobody@globo.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "this email is already registered")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusConflict)
}

func (s *AuthSuite) TestCreateUserHandlerReturnsBadRequestIfEmailIsNotValid(c *check.C) {
	b := bytes.NewBufferString(`{"email":"nobody","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "invalid email")
}

func (s *AuthSuite) TestCreateUserHandlerReturnsBadRequestIfPasswordHasLessThan6CharactersOrMoreThan50Characters(c *check.C) {
	passwords := []string{"123", strings.Join(make([]string, 52), "-")}
	for _, password := range passwords {
		b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"` + password + `"}`)
		request, err := http.NewRequest("POST", "/users", b)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-type", "application/json")
		recorder := httptest.NewRecorder()
		err = createUser(recorder, request)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e.Message, check.Equals, "password length should be least 6 characters and at most 50 characters")
	}
}

func (s *AuthSuite) TestCreateUserCreatesUserInRepository(c *check.C) {
	b := bytes.NewBufferString(`{"email":"nobody@me.myself","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": "nobody@me.myself"})
	err = createUser(recorder, request)
	c.Assert(err, check.IsNil)
	_, err = repository.Manager().(repository.KeyRepositoryManager).ListKeys("nobody@me.myself")
	c.Assert(err, check.IsNil)
}

func (s *AuthSuite) TestCreateUserFailWithRegistrationDisabled(c *check.C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	oldUserRegistration, err := config.GetBool("auth:user-registration")
	c.Assert(err, check.IsNil)
	config.Set("auth:user-registration", false)
	defer config.Set("auth:user-registration", oldUserRegistration)
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.Equals, createDisabledErr)
}

func (s *AuthSuite) TestCreateUserFailWithRegistrationDisabledAndCommonUser(c *check.C) {
	simpleUser := &auth.User{Email: "my@common.user", Password: "123456"}
	_, err := nativeScheme.Create(simpleUser)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": simpleUser.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	oldUserRegistration, err := config.GetBool("auth:user-registration")
	c.Assert(err, check.IsNil)
	config.Set("auth:user-registration", false)
	defer config.Set("auth:user-registration", oldUserRegistration)
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.Equals, createDisabledErr)
}

func (s *AuthSuite) TestCreateUserWorksWithRegistrationDisabledAndAdminUser(c *check.C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	oldUserRegistration, err := config.GetBool("auth:user-registration")
	c.Assert(err, check.IsNil)
	config.Set("auth:user-registration", false)
	defer config.Set("auth:user-registration", oldUserRegistration)
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, check.IsNil)
}

func (s *AuthSuite) TestCreateUserRollsbackAfterRepositoryError(c *check.C) {
	repository.Manager().CreateUser("nobody@globo.com")
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	_, err = auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, check.NotNil)
}

func (s *AuthSuite) TestLoginShouldCreateTokenInTheDatabaseAndReturnItWithinTheResponse(c *check.C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, check.IsNil)
	var user auth.User
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": "nobody@globo.com"}).One(&user)
	var recorderJSON map[string]string
	r, _ := ioutil.ReadAll(recorder.Body)
	json.Unmarshal(r, &recorderJSON)
	n, err := conn.Tokens().Find(bson.M{"token": recorderJSON["token"]}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	action := rectest.Action{
		Action: "login",
		User:   u.Email,
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestLoginShouldInformWhenUserIsNotAdmin(c *check.C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, check.IsNil)
	var user auth.User
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": "nobody@globo.com"}).One(&user)
	var recorderJSON map[string]interface{}
	r, _ := ioutil.ReadAll(recorder.Body)
	json.Unmarshal(r, &recorderJSON)
	c.Assert(recorderJSON["is_admin"], check.Equals, false)
}

func (s *AuthSuite) TestLoginShouldInformWhenUserIsAdmin(c *check.C) {
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/whydidifall@thewho.com/tokens?:email=whydidifall@thewho.com", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, check.IsNil)
	var user auth.User
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": "whydidifall@thewho.com"}).One(&user)
	var recorderJSON map[string]interface{}
	r, _ := ioutil.ReadAll(recorder.Body)
	json.Unmarshal(r, &recorderJSON)
	c.Assert(recorderJSON["is_admin"], check.Equals, true)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndBadRequestIfItReceivesAnInvalidJSON(c *check.C) {
	b := bytes.NewBufferString(`"invalid":"json"]`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Invalid JSON$")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndBadRequestIfTheJSONDoesNotContainsAPassword(c *check.C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^you must provide a password to login$")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndNotFoundIfTheUserDoesNotExist(c *check.C) {
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^user not found$")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestLoginShouldreturnErrorIfThePasswordDoesNotMatch(c *check.C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(`{"password":"1234567"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Authentication failed, wrong password.$")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusUnauthorized)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndInternalServerErrorIfReadAllFails(c *check.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	err := b.Close()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, check.NotNil)
}

func (s *AuthSuite) TestLoginShouldReturnBadRequestIfEmailIsNotValid(c *check.C) {
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody/token?:email=nobody", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, native.ErrInvalidEmail.Error())
}

func (s *AuthSuite) TestLoginShouldReturnBadRequestWhenPasswordIsInvalid(c *check.C) {
	passwords := []string{"123", strings.Join(make([]string, 52), "-")}
	u := &auth.User{Email: "me@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	for _, password := range passwords {
		b := bytes.NewBufferString(`{"password":"` + password + `"}`)
		request, err := http.NewRequest("POST", "/users/me@globo.com/token?:email=me@globo.com", b)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-type", "application/json")
		recorder := httptest.NewRecorder()
		err = login(recorder, request)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e.Message, check.Matches, "Password.*")
	}
}

func (s *AuthSuite) TestLogout(c *check.C) {
	token, err := nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/users/tokens", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = logout(recorder, request, token)
	c.Assert(err, check.IsNil)
	_, err = nativeScheme.Auth(token.GetValue())
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *AuthSuite) TestCreateTeamHandlerSavesTheTeamInTheDatabaseWithTheAuthenticatedUser(c *check.C) {
	b := bytes.NewBufferString(`{"name":"timeredbull"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	t := new(auth.Team)
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Teams().Find(bson.M{"_id": "timeredbull"}).One(t)
	defer conn.Teams().Remove(bson.M{"_id": "timeredbull"})
	c.Assert(err, check.IsNil)
	c.Assert(t, ContainsUser, s.user)
	action := rectest.Action{
		Action: "create-team",
		User:   s.user.Email,
		Extra:  []interface{}{"timeredbull"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnsBadRequestIfTheRequestBodyIsAnInvalidJSON(c *check.C) {
	b := bytes.NewBufferString(`{"name"["invalidjson"]}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnsBadRequestIfTheNameIsNotGiven(c *check.C) {
	b := bytes.NewBufferString(`{"genre":"male"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, auth.ErrInvalidTeamName.Error())
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnConflictIfTheTeamToBeCreatedAlreadyExists(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	err := conn.Teams().Insert(bson.M{"_id": "timeredbull"})
	defer conn.Teams().Remove(bson.M{"_id": "timeredbull"})
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(`{"name":"timeredbull"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusConflict)
	c.Assert(e, check.ErrorMatches, "^team already exists$")
}

func (s *AuthSuite) TestRemoveTeam(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	team := auth.Team{Name: "painofsalvation", Users: []string{s.user.Email}}
	err := conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": team.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	n, err := conn.Teams().Find(bson.M{"name": team.Name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	action := rectest.Action{
		Action: "remove-team",
		User:   s.user.Email,
		Extra:  []interface{}{team.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveTeamGives404WhenTeamDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/teams/unknown?:name=unknown", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, `Team "unknown" not found.`)
}

func (s *AuthSuite) TestRemoveTeamGives404WhenUserDoesNotHaveAccessToTheTeam(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	team := auth.Team{Name: "painofsalvation"}
	err := conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": team.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, `Team "painofsalvation" not found.`)
}

func (s *AuthSuite) TestRemoveTeamGives403WhenTeamHasAccessToAnyApp(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	team := auth.Team{Name: "evergrey", Users: []string{s.user.Email}}
	err := conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": team.Name})
	a := app.App{Name: "i-should", Platform: "python", Teams: []string{team.Name}, TeamOwner: team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	expected := `This team cannot be removed because it have access to apps.

Please remove the apps or revoke these accesses, and try again.`
	c.Assert(e.Message, check.Equals, expected)
}

func (s *AuthSuite) TestListTeamsListsAllTeamsThatTheUserIsMember(c *check.C) {
	request, err := http.NewRequest("GET", "/teams", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = teamList(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var m []map[string]string
	err = json.Unmarshal(b, &m)
	c.Assert(err, check.IsNil)
	c.Assert(m, check.DeepEquals, []map[string]string{{"name": s.team.Name}})
	action := rectest.Action{
		Action: "list-teams",
		User:   s.user.Email,
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestListTeamsReturns204IfTheUserHasNoTeam(c *check.C) {
	u := auth.User{Email: "cruiser@gotthard.com", Password: "234567"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "234567"})
	c.Assert(err, check.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("GET", "/teams", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = teamList(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *AuthSuite) TestAddUserToTeam(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	url := "/teams/tsuruteam/wolverine@xmen.com?:team=tsuruteam&:user=wolverine@xmen.com"
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	t := new(auth.Team)
	err = conn.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, check.IsNil)
	c.Assert(t, ContainsUser, s.user)
	c.Assert(t, ContainsUser, u)
	action := rectest.Action{
		Action: "add-user-to-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=" + u.Email},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnNotFoundIfThereIsNoTeamWithTheGivenName(c *check.C) {
	request, err := http.NewRequest("PUT", "/teams/abc/me@me.me?:team=abc&:user=me@me.me", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^Team not found$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnForbiddenIfTheGivenUserIsNotInTheGivenTeam(c *check.C) {
	u := &auth.User{Email: "hi@me.me", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("PUT", "/teams/tsuruteam/hi@me.me?:team=tsuruteam&:user=hi@me.me", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addUserToTeam(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^You are not authorized to add new users to the team tsuruteam$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnNotFoundIfTheEmailInTheBodyDoesNotExistInTheDatabase(c *check.C) {
	request, err := http.NewRequest("PUT", "/teams/tsuruteam/hi2@me.me?:team=tsuruteam&:user=hi2@me.me", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^User not found$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnConflictIfTheUserIsAlreadyInTheGroup(c *check.C) {
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, s.user.Email, s.team.Name, s.user.Email)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusConflict)
}

func (s *AuthSuite) TestAddUserToTeamShoulGrantAccessInRepository(c *check.C) {
	u := &auth.User{Email: "marathon@rush.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	t, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer nativeScheme.Logout(t.GetValue())
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	a := app.App{Name: "i-should", Platform: "python", Teams: []string{s.team.Name}}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	b := bytes.NewBufferString(`{"name":"the-key","key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, t)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, u.Email, s.team.Name, u.Email)
	request, err = http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder = httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	grants, err := repositorytest.Granted(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{s.user.Email, "marathon@rush.com"})
}

func (s *AuthSuite) TestAddUserToTeamInDatabase(c *check.C) {
	user := &auth.User{Email: "nobody@gmail.com", Password: "123456"}
	team := &auth.Team{Name: "myteam"}
	conn, _ := db.Conn()
	defer conn.Close()
	err := conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer conn.Teams().RemoveId(team.Name)
	err = addUserToTeamInDatabase(user, team)
	c.Assert(err, check.IsNil)
	conn.Teams().FindId(team.Name).One(team)
	c.Assert(team.Users, check.DeepEquals, []string{user.Email})
}

func (s *AuthSuite) TestAddLeadToTeamAsAdmin(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	s.team.AddUser(u)
	err := conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	_, err = nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	url := "/teams/tsuruteam/leads/wolverine@xmen.com?:team=tsuruteam&:user=wolverine@xmen.com"
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addLeadToTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	t := new(auth.Team)
	err = conn.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, check.IsNil)
	c.Assert(t, check.Not(ContainsTeamLead), s.user)
	c.Assert(t, ContainsTeamLead, u)
	action := rectest.Action{
		Action: "add-lead-to-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=" + u.Email},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestAddLeadToTeamAsTeamLead(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	config.Set("admin-team", "admin-team")
	s.team.AddTeamLead(s.user)
	err := conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	u := &auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	s.team.AddUser(u)
	err = conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	_, err = nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	url := "/teams/tsuruteam/leads/wolverine@xmen.com?:team=tsuruteam&:user=wolverine@xmen.com"
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addLeadToTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	t := new(auth.Team)
	err = conn.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, check.IsNil)
	c.Assert(t, ContainsTeamLead, s.user)
	c.Assert(t, ContainsTeamLead, u)
	action := rectest.Action{
		Action: "add-lead-to-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=" + u.Email},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestAddLeadToTeamShouldReturnErrorWhenNotAdminAndNotTeamLead(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	config.Set("admin-team", "admin-team")
	u := &auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	s.team.AddUser(u)
	err := conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	_, err = nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	url := "/teams/tsuruteam/leads/wolverine@xmen.com?:team=tsuruteam&:user=wolverine@xmen.com"
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addLeadToTeam(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "^You are not authorized to add new leads to the team tsuruteam$")
	t := new(auth.Team)
	err = conn.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, check.IsNil)
	c.Assert(t, check.Not(ContainsTeamLead), s.user)
	c.Assert(t, check.Not(ContainsTeamLead), u)
	action := rectest.Action{
		Action: "add-lead-to-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=" + u.Email},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestAddLeadToTeamInDatabase(c *check.C) {
	user := &auth.User{Email: "nobody@gmail.com", Password: "123456"}
	team := &auth.Team{Name: "myteam"}
	conn, _ := db.Conn()
	defer conn.Close()
	err := conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	team.AddUser(user)
	err = conn.Teams().UpdateId(team.Name, team)
	c.Assert(err, check.IsNil)
	defer conn.Teams().RemoveId(team.Name)
	team.AddUser(user)
	err = conn.Teams().UpdateId(team.Name, team)
	c.Assert(err, check.IsNil)
	err = addLeadToTeamInDatabase(user, team)
	c.Assert(err, check.IsNil)
	conn.Teams().FindId(team.Name).One(team)
	c.Assert(team.TeamLeads, check.DeepEquals, []string{user.Email})
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldRemoveAUserFromATeamIfTheTeamExistAndTheUserIsMemberOfTheTeam(c *check.C) {
	u := auth.User{Email: "nonee@me.me"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	s.team.AddUser(&u)
	conn.Teams().Update(bson.M{"_id": s.team.Name}, s.team)
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/nonee@me.me?:team=tsuruteam&:user=nonee@me.me", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	err = conn.Teams().Find(bson.M{"_id": s.team.Name}).One(s.team)
	c.Assert(err, check.IsNil)
	c.Assert(s.team, check.Not(ContainsUser), &u)
	action := rectest.Action{
		Action: "remove-user-from-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=" + u.Email},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldRemoveOnlyAppsInThatTeamInRepositoryWhenUserIsInMoreThanOneTeam(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "nobody@me.me"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	s.team.AddUser(&u)
	conn.Teams().UpdateId(s.team.Name, s.team)
	team2 := auth.Team{Name: "team2", Users: []string{u.Email}}
	err = conn.Teams().Insert(&team2)
	c.Assert(err, check.IsNil)
	defer conn.Teams().RemoveId(team2.Name)
	app1 := app.App{Name: "app1", Platform: "python", Teams: []string{s.team.Name}}
	err = app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&app1)
	app2 := app.App{Name: "app2", Platform: "python", Teams: []string{s.team.Name, team2.Name}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&app2)
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, u.Email, s.team.Name, u.Email)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	grants, err := repositorytest.Granted("app1")
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{s.user.Email})
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnNotFoundIfTheTeamDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/none@me.me?:team=unknown&:user=none@me.me", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^Team not found$")
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnUnauthorizedIfTheGivenUserIsNotMemberOfTheTeam(c *check.C) {
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/none@me.me?:team=tsuruteam&:user=none@me.me", nil)
	c.Assert(err, check.IsNil)
	u := &auth.User{Email: "unknown@gmail.com", Password: "123456"}
	_, err = nativeScheme.Create(u)
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusUnauthorized)
	c.Assert(e, check.ErrorMatches, "^You are not authorized to remove a member from the team tsuruteam")
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnNotFoundWhenTheUserIsNotMemberOfTheTeam(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "nobody@me.me", Password: "132"}
	s.team.AddUser(u)
	conn.Teams().Update(bson.M{"_id": s.team.Name}, s.team)
	defer func(t *auth.Team, u *auth.User) {
		s.team.RemoveUser(u)
		conn.Teams().Update(bson.M{"_id": t.Name}, t)
	}(s.team, u)
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/none@me.me?:team=tsuruteam&:user=none@me.me", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnForbiddenIfTheUserIsTheLastInTheTeam(c *check.C) {
	url := "/teams/tsuruteam/whydidifall@thewho.com?:team=tsuruteam&:user=whydidifall@thewho.com"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e, check.ErrorMatches, "^You can not remove this user from this team, because it is the last user within the team, and a team can not be orphaned$")
}

func (s *AuthSuite) TestRemoveUserFromTeamRevokesAccessInRepository(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "pomar@nando-reis.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	t, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer nativeScheme.Logout(t.GetValue())
	defer conn.Users().Remove(bson.M{"email": u.Email})
	b := bytes.NewBufferString(`{"name":"the-key","key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, t)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, u.Email, s.team.Name, u.Email)
	request, err = http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder = httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myapp", Platform: "python", Teams: []string{s.team.Name}}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a)
	url = fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, u.Email, s.team.Name, u.Email)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder = httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	grants, err := repositorytest.Granted("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{s.user.Email})
}

func (s *AuthSuite) TestRemoveUserFromTeamInDatabase(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "nobody@gmail.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	s.team.AddUser(u)
	err = conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	err = removeUserFromTeamInDatabase(u, s.team)
	c.Assert(err, check.IsNil)
	err = conn.Teams().FindId(s.team.Name).One(s.team)
	c.Assert(err, check.IsNil)
	c.Assert(s.team, check.Not(ContainsUser), u)
}

func (s *AuthSuite) TestRemoveLeadFromTeamAsAdmin(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	s.team.AddTeamLead(s.user)
	err := conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	u := &auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	s.team.AddUser(u)
	s.team.AddTeamLead(u)
	err = conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	_, err = nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	url := "/teams/tsuruteam/leads/wolverine@xmen.com?:team=tsuruteam&:user=wolverine@xmen.com"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeLeadFromTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	t := new(auth.Team)
	err = conn.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, check.IsNil)
	c.Assert(t, ContainsTeamLead, s.user)
	c.Assert(t, check.Not(ContainsTeamLead), u)
	action := rectest.Action{
		Action: "remove-lead-from-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=" + u.Email},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveLeadFromTeamAsTeamLead(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	config.Set("admin-team", "admin-team")
	s.team.AddTeamLead(s.user)
	err := conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	u := &auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	s.team.AddUser(u)
	s.team.AddTeamLead(u)
	err = conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	_, err = nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	url := "/teams/tsuruteam/leads/wolverine@xmen.com?:team=tsuruteam&:user=wolverine@xmen.com"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeLeadFromTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	t := new(auth.Team)
	err = conn.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, check.IsNil)
	c.Assert(t, ContainsTeamLead, s.user)
	c.Assert(t, check.Not(ContainsTeamLead), u)
	action := rectest.Action{
		Action: "remove-lead-from-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=" + u.Email},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveLeadFromTeamShouldReturnErrorWhenNotAdminAndNotTeamLead(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	config.Set("admin-team", "admin-team")
	err := conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	url := "/teams/tsuruteam/leads/whydidifall@thewho.com?:team=tsuruteam&:user=whydidifall@thewho.com"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeLeadFromTeam(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "^You are not authorized to remove a lead from the team tsuruteam")
	t := new(auth.Team)
	err = conn.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, check.IsNil)
	action := rectest.Action{
		Action: "remove-lead-from-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=whydidifall@thewho.com"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveLeadFromTeamShouldReturnErrorWhenItIsLastTeamLead(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	s.team.AddTeamLead(s.user)
	err := conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	url := "/teams/tsuruteam/leads/whydidifall@thewho.com?:team=tsuruteam&:user=whydidifall@thewho.com"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeLeadFromTeam(recorder, request, s.token)
	c.Assert(err, check.ErrorMatches, "^You can not remove this lead from this team, because it is the last team lead")
	t := new(auth.Team)
	err = conn.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, check.IsNil)
	c.Assert(t, ContainsTeamLead, s.user)
	action := rectest.Action{
		Action: "remove-lead-from-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=whydidifall@thewho.com"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveLeadFromTeamInDatabase(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "nobody@gmail.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	s.team.AddUser(u)
	s.team.AddTeamLead(u)
	err = conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, check.IsNil)
	err = removeLeadFromTeamInDatabase(u, s.team)
	c.Assert(err, check.IsNil)
	err = conn.Teams().FindId(s.team.Name).One(s.team)
	c.Assert(err, check.IsNil)
	c.Assert(s.team, check.Not(ContainsTeamLead), u)
}

func (s *AuthSuite) TestGetTeam(c *check.C) {
	team, err := auth.GetTeam(s.team.Name)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = getTeam(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got auth.Team
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, *team)
	action := rectest.Action{
		User:   s.user.Email,
		Action: "get-team",
		Extra:  []interface{}{team.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestGetTeamNotFound(c *check.C) {
	url := "/teams/unknown?:name=unknown"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = getTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, "Team not found")
}

func (s *AuthSuite) TestGetTeamForbidden(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	team := auth.Team{Name: "paradisum", Users: []string{"someuser@me.com"}}
	conn.Teams().Insert(team)
	defer conn.Teams().RemoveId(team.Name)
	url := fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = getTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e.Message, check.Equals, "User is not member of this team")
}

func (s *AuthSuite) TestAddKeyToUser(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	defer func() {
		s.user.RemoveKey(repository.Key{Name: "the-key"})
		conn.Users().Update(bson.M{"email": s.user.Email}, s.user)
	}()
	b := bytes.NewBufferString(`{"name":"the-key","key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	s.user, err = auth.GetUserByEmail(s.user.Email)
	c.Assert(err, check.IsNil)
	action := rectest.Action{
		Action: "add-key",
		User:   s.user.Email,
		Extra:  []interface{}{"the-key", "my-key"},
	}
	c.Assert(action, rectest.IsRecorded)
	keys, err := repository.Manager().(repository.KeyRepositoryManager).ListKeys(s.user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.DeepEquals, []repository.Key{{Name: "the-key", Body: "my-key"}})
}

func (s *AuthSuite) TestAddKeyToUserReturnsErrorIfTheReadingOfTheBodyFails(c *check.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	b.Close()
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
}

func (s *AuthSuite) TestAddKeyToUserReturnsBadRequestIfTheJSONIsInvalid(c *check.C) {
	b := bytes.NewBufferString(`"aaaa}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e, check.ErrorMatches, "^Invalid JSON$")
}

func (s *AuthSuite) TestAddKeyToUserReturnsBadRequestIfTheKeyIsNotPresent(c *check.C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e, check.ErrorMatches, "^Missing key content$")
}

func (s *AuthSuite) TestAddKeyToUserReturnsBadRequestIfTheKeyIsEmpty(c *check.C) {
	b := bytes.NewBufferString(`{"key":""}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e, check.ErrorMatches, "^Missing key content$")
}

func (s *AuthSuite) TestAddKeyToUserKeyManagerDisabled(c *check.C) {
	config.Set("repo-manager", "none")
	defer config.Set("repo-manager", "fake")
	conn, _ := db.Conn()
	defer conn.Close()
	b := bytes.NewBufferString(`{"name":"the-key","key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "key management is disabled")
}

func (s *AuthSuite) TestAddKeyToUserReturnsConflictIfTheKeyIsAlreadyPresent(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	s.user.AddKey(repository.Key{Name: "the-key", Body: "my-key"})
	conn.Users().Update(bson.M{"email": s.user.Email}, s.user)
	defer func() {
		s.user.RemoveKey(repository.Key{Name: "the-key", Body: "my-key"})
		conn.Users().Update(bson.M{"email": s.user.Email}, s.user)
	}()
	b := bytes.NewBufferString(`{"name":"the-key","key":"your-key"}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusConflict)
	c.Assert(e.Message, check.Equals, "user already have this key")
}

func (s *AuthSuite) TestAddKeyToUserFailure(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	t, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer nativeScheme.Logout(t.GetValue())
	b := bytes.NewBufferString(`{"name":"the-key","key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, check.IsNil)
	repository.Manager().RemoveUser(u.Email)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, t)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "user not found")
}

func (s *AuthSuite) TestRemoveKeyHandler(c *check.C) {
	b := bytes.NewBufferString(`{"name":"the-key","key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	addKeyToUser(recorder, request, s.token)
	b = bytes.NewBufferString(`{"name":"the-key","key":"my-key"}`)
	request, err = http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder = httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	action := rectest.Action{
		Action: "remove-key",
		User:   s.user.Email,
		Extra:  []interface{}{"the-key", "my-key"},
	}
	c.Assert(action, rectest.IsRecorded)
	keys, err := repository.Manager().(repository.KeyRepositoryManager).ListKeys(s.user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.HasLen, 0)
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsErrorInCaseOfAnyIOFailure(c *check.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	b.Close()
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheJSONIsInvalid(c *check.C) {
	b := bytes.NewBufferString(`invalid"json}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e, check.ErrorMatches, "^Invalid JSON$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheKeyIsNotPresent(c *check.C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e, check.ErrorMatches, "^Either the content or the name of the key must be provided$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheKeyIsEmpty(c *check.C) {
	b := bytes.NewBufferString(`{"name":""}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e, check.ErrorMatches, "^Either the content or the name of the key must be provided$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsNotFoundIfTheUserDoesNotHaveTheKey(c *check.C) {
	b := bytes.NewBufferString(`{"name":"the-key"}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestRemoveKeyFromUserKeyManagerDisabled(c *check.C) {
	config.Set("repo-manager", "none")
	defer config.Set("repo-manager", "fake")
	conn, _ := db.Conn()
	defer conn.Close()
	b := bytes.NewBufferString(`{"name":"the-key","key":"my-key"}`)
	request, err := http.NewRequest("DELETE", "/users/keys", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "key management is disabled")
}

func (s *AuthSuite) TestListKeysHandler(c *check.C) {
	keys := []repository.Key{
		{Name: "homekey", Body: "lol somekey somecomment"},
		{Name: "workkey", Body: "lol someotherkey someothercomment"},
	}
	repository.Manager().(repository.KeyRepositoryManager).AddKey(s.user.Email, keys[0])
	repository.Manager().(repository.KeyRepositoryManager).AddKey(s.user.Email, keys[1])
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/users/keys", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	got := map[string]string{}
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	expected := map[string]string{
		"homekey": "lol somekey somecomment",
		"workkey": "lol someotherkey someothercomment",
	}
	c.Assert(expected, check.DeepEquals, got)
}

func (s *AuthSuite) TestListKeysKeyManagerDisabled(c *check.C) {
	config.Set("repo-manager", "none")
	defer config.Set("repo-manager", "fake")
	conn, _ := db.Conn()
	defer conn.Close()
	request, err := http.NewRequest("GET", "/users/keys", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = listKeys(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "key management is disabled")
}

func (s *AuthSuite) TestRemoveUser(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "her-voices@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, check.IsNil)
	n, err := conn.Users().Find(bson.M{"email": u.Email}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	action := rectest.Action{Action: "remove-user", User: u.Email}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(repositorytest.Users(), check.DeepEquals, []string{"whydidifall@thewho.com"})
}

func (s *AuthSuite) TestRemoveAnotherUser(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "her-voices@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	request, err := http.NewRequest("DELETE", "/users?user="+u.Email, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	n, err := conn.Users().Find(bson.M{"email": u.Email}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	action := rectest.Action{Action: "remove-user", User: u.Email}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(repositorytest.Users(), check.DeepEquals, []string{"whydidifall@thewho.com"})
}

func (s *AuthSuite) TestRemoveAnotherUserNonAdmin(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "her-voices@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("DELETE", "/users?user="+s.user.Email, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	c.Assert(e.Message, check.Equals, "you're not allowed to remove this user")
}

func (s *AuthSuite) TestRemoveUserNonSpecifiedUserAdmin(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "please specify the user you want to remove")
}

func (s *AuthSuite) TestRemoveUserWithTheUserBeingLastMemberOfATeam(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	t := auth.Team{Name: "painofsalvation", Users: []string{u.Email}}
	err = conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": t.Name})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	expected := `This user is the last member of the team "painofsalvation", so it cannot be removed.

Please remove the team, then remove the user.`
	c.Assert(e.Message, check.Equals, expected)
}

func (s *AuthSuite) TestRemoveUserShouldRemoveTheUserFromAllTeamsThatHeIsMember(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	t := auth.Team{Name: "painofsalvation", Users: []string{u.Email, s.user.Email}}
	err = conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": t.Name})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, check.IsNil)
	err = conn.Teams().Find(bson.M{"_id": t.Name}).One(&t)
	c.Assert(err, check.IsNil)
	c.Assert(t.Users, check.HasLen, 1)
	c.Assert(t.Users[0], check.Equals, s.user.Email)
}

func (s *AuthSuite) TestChangePasswordHandler(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@globo.com.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	oldPassword := u.Password
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	body := bytes.NewBufferString(`{"old":"123456","new":"654321"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = changePassword(recorder, request, token)
	c.Assert(err, check.IsNil)
	otherUser, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(otherUser.Password, check.Not(check.Equals), oldPassword)
	action := rectest.Action{
		Action: "change-password",
		User:   u.Email,
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestChangePasswordReturns412IfNewPasswordIsInvalid(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@globo.com.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	body := bytes.NewBufferString(`{"old":"123456","new":"1234"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = changePassword(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Check(e.Code, check.Equals, http.StatusBadRequest)
	c.Check(e.Message, check.Equals, "password length should be least 6 characters and at most 50 characters")
}

func (s *AuthSuite) TestChangePasswordReturns404IfOldPasswordDidntMatch(c *check.C) {
	body := bytes.NewBufferString(`{"old":"1234","new":"123456"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = changePassword(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Check(e.Code, check.Equals, http.StatusForbidden)
	c.Check(e.Message, check.Equals, "the given password didn't match the user's current password")
}

func (s *AuthSuite) TestChangePasswordReturns400IfRequestBodyIsInvalidJSON(c *check.C) {
	body := bytes.NewBufferString(`{"invalid:"json`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = changePassword(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "Invalid JSON.")
}

func (s *AuthSuite) TestChangePasswordReturns400IfJSONDoesNotIncludeBothOldAndNewPasswords(c *check.C) {
	bodies := []string{`{"old": "something"}`, `{"new":"something"}`, "{}", "null"}
	for _, body := range bodies {
		b := bytes.NewBufferString(body)
		request, err := http.NewRequest("PUT", "/users/password", b)
		c.Assert(err, check.IsNil)
		recorder := httptest.NewRecorder()
		err = changePassword(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e.Message, check.Equals, "Both the old and the new passwords are required.")
	}
}

func (s *AuthSuite) TestResetPasswordStep1(c *check.C) {
	defer s.server.Reset()
	oldPassword := s.user.Password
	url := fmt.Sprintf("/users/%s/password?:email=%s", s.user.Email, s.user.Email)
	request, _ := http.NewRequest("POST", url, nil)
	recorder := httptest.NewRecorder()
	err := resetPassword(recorder, request)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	var m map[string]interface{}
	err = conn.PasswordTokens().Find(bson.M{"useremail": s.user.Email}).One(&m)
	c.Assert(err, check.IsNil)
	defer conn.PasswordTokens().RemoveId(m["_id"])
	time.Sleep(1e9)
	s.server.RLock()
	defer s.server.RUnlock()
	c.Assert(s.server.MailBox, check.HasLen, 1)
	u, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(u.Password, check.Equals, oldPassword)
	action := rectest.Action{
		Action: "reset-password-gen-token",
		User:   s.user.Email,
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestResetPasswordUserNotFound(c *check.C) {
	url := "/users/unknown@tsuru.io/password?:email=unknown@tsuru.io"
	request, _ := http.NewRequest("POST", url, nil)
	recorder := httptest.NewRecorder()
	err := resetPassword(recorder, request)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, "user not found")
}

func (s *AuthSuite) TestResetPasswordInvalidEmail(c *check.C) {
	url := "/users/unknown/password?:email=unknown"
	request, _ := http.NewRequest("POST", url, nil)
	recorder := httptest.NewRecorder()
	err := resetPassword(recorder, request)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "invalid email")
}

func (s *AuthSuite) TestResetPasswordStep2(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	user := auth.User{Email: "uns@alanis.com", Password: "145678"}
	err = user.Create()
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	oldPassword := user.Password
	err = nativeScheme.StartPasswordReset(&user)
	c.Assert(err, check.IsNil)
	var t map[string]interface{}
	err = conn.PasswordTokens().Find(bson.M{"useremail": user.Email}).One(&t)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/users/%s/password?:email=%s&token=%s", user.Email, user.Email, t["_id"])
	request, _ := http.NewRequest("POST", url, nil)
	recorder := httptest.NewRecorder()
	err = resetPassword(recorder, request)
	c.Assert(err, check.IsNil)
	u2, err := auth.GetUserByEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(u2.Password, check.Not(check.Equals), oldPassword)
	action := rectest.Action{
		Action: "reset-password",
		User:   user.Email,
	}
	c.Assert(action, rectest.IsRecorded)
}

type TestScheme native.NativeScheme

func (t TestScheme) AppLogin(appName string) (auth.Token, error) {
	return nil, nil
}
func (t TestScheme) Login(params map[string]string) (auth.Token, error) {
	return nil, nil
}
func (t TestScheme) Logout(token string) error {
	return nil
}
func (t TestScheme) Auth(token string) (auth.Token, error) {
	return nil, nil
}
func (t TestScheme) Info() (auth.SchemeInfo, error) {
	return auth.SchemeInfo{"foo": "bar", "foo2": "bar2"}, nil
}
func (t TestScheme) Name() string {
	return "test"
}
func (t TestScheme) Create(u *auth.User) (*auth.User, error) {
	return nil, nil
}
func (t TestScheme) Remove(u *auth.User) error {
	return nil
}

func (s *AuthSuite) TestAuthScheme(c *check.C) {
	oldScheme := app.AuthScheme
	defer func() { app.AuthScheme = oldScheme }()
	app.AuthScheme = TestScheme{}
	request, _ := http.NewRequest("GET", "/auth/scheme", nil)
	recorder := httptest.NewRecorder()
	err := authScheme(recorder, request)
	c.Assert(err, check.IsNil)
	var parsed map[string]interface{}
	err = json.NewDecoder(recorder.Body).Decode(&parsed)
	c.Assert(err, check.IsNil)
	c.Assert(parsed["name"], check.Equals, "test")
	c.Assert(parsed["data"], check.DeepEquals, map[string]interface{}{"foo": "bar", "foo2": "bar2"})
}

func (s *AuthSuite) TestRegenerateAPITokenHandler(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "zobomafoo@zimbabue.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("POST", "/users/api-key", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = regenerateAPIToken(recorder, request, token)
	c.Assert(err, check.IsNil)
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	count, err := conn.Users().Find(bson.M{"apikey": got}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *AuthSuite) TestShowAPITokenForUserWithNoToken(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "zobomafoo@zimbabue.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("GET", "/users/api-key", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = showAPIToken(recorder, request, token)
	c.Assert(err, check.IsNil)
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	count, err := conn.Users().Find(bson.M{"apikey": got}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *AuthSuite) TestShowAPITokenForUserWithToken(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "zobomafoo@zimbabue.com", Password: "123456", APIKey: "238hd23ubd923hd923j9d23ndibde"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("GET", "/users/api-key", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = showAPIToken(recorder, request, token)
	c.Assert(err, check.IsNil)
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.Equals, "238hd23ubd923hd923j9d23ndibde")
}

func (s *AuthSuite) TestListUsers(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	token, err := nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("GET", "/users", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = listUsers(recorder, request, token)
	c.Assert(err, check.IsNil)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(len(users), check.Equals, 1)
	c.Assert(users[0].Email, check.Equals, s.user.Email)
	c.Assert(users[0].Teams, check.DeepEquals, []string{s.team.Name})
}

func (s *AuthSuite) TestUserInfo(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	token, err := nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("GET", "/users/info", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	expected := apiUser{Email: s.user.Email, Teams: []string{s.team.Name}}
	var got apiUser
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
}
