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
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/rec/rectest"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

type AuthSuite struct {
	team   *auth.Team
	user   *auth.User
	token  auth.Token
	server *authtest.SMTPServer
}

var _ = gocheck.Suite(&AuthSuite{})

func (s *AuthSuite) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("auth:user-registration", true)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_auth_test")
	config.Set("auth:hash-cost", 4)
	s.createUserAndTeam(c)
	config.Set("admin-team", s.team.Name)
	s.server, err = authtest.NewSMTPServer()
	c.Assert(err, gocheck.IsNil)
	config.Set("smtp:server", s.server.Addr())
	config.Set("smtp:user", "root")
	config.Set("smtp:password", "123456")
	app.Provisioner = provisiontest.NewFakeProvisioner()
	app.AuthScheme = nativeScheme
}

func (s *AuthSuite) TearDownSuite(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	s.server.Stop()
}

func (s *AuthSuite) TearDownTest(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	_, err := conn.Users().RemoveAll(nil)
	c.Assert(err, gocheck.IsNil)
	_, err = conn.Teams().RemoveAll(bson.M{"_id": bson.M{"$ne": s.team.Name}})
	c.Assert(err, gocheck.IsNil)
	s.user.Password = "123456"
	s.user, err = nativeScheme.Create(s.user)
	c.Assert(err, gocheck.IsNil)
}

func (s *AuthSuite) createUserAndTeam(c *gocheck.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err := nativeScheme.Create(s.user)
	c.Assert(err, gocheck.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
}

func (s *AuthSuite) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}

type userPresenceChecker struct{}

func (c *userPresenceChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "ContainsUser", Params: []string{"team", "user"}}
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

var ContainsUser gocheck.Checker = &userPresenceChecker{}

func (s *AuthSuite) TestCreateUserHandlerSavesTheUserInTheDatabase(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.IsNil)
	user, err := auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, gocheck.IsNil)
	action := rectest.Action{
		Action: "create-user",
		User:   "nobody@globo.com",
	}
	c.Assert(action, rectest.IsRecorded)
	c.Assert(user.Quota, gocheck.DeepEquals, quota.Unlimited)
}

func (s *AuthSuite) TestCreateUserQuota(c *gocheck.C) {
	config.Set("quota:apps-per-user", 1)
	defer config.Unset("quota:apps-per-user")
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.IsNil)
	user, err := auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(user.Quota.Limit, gocheck.Equals, 1)
	c.Assert(user.Quota.InUse, gocheck.Equals, 0)
}

func (s *AuthSuite) TestCreateUserUnlimitedQuota(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.IsNil)
	user, err := auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(user.Quota, gocheck.DeepEquals, quota.Unlimited)
}

func (s *AuthSuite) TestCreateUserHandlerReturnsStatus201AfterCreateTheUser(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, 201)
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorIfReadingBodyFails(c *gocheck.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^.*bad file descriptor$")
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorAndBadRequestIfInvalidJSONIsGiven(c *gocheck.C) {
	b := bytes.NewBufferString(`["invalid json":"i'm invalid"]`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^invalid character.*$")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorAndConflictIfItFailsToCreateUser(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "nobody@globo.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "This email is already registered.")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
}

func (s *AuthSuite) TestCreateUserHandlerReturnsBadRequestIfEmailIsNotValid(c *gocheck.C) {
	b := bytes.NewBufferString(`{"email":"nobody","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "Invalid email.")
}

func (s *AuthSuite) TestCreateUserHandlerReturnsBadRequestIfPasswordHasLessThan6CharactersOrMoreThan50Characters(c *gocheck.C) {
	passwords := []string{"123", strings.Join(make([]string, 52), "-")}
	for _, password := range passwords {
		b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"` + password + `"}`)
		request, err := http.NewRequest("POST", "/users", b)
		c.Assert(err, gocheck.IsNil)
		request.Header.Set("Content-type", "application/json")
		recorder := httptest.NewRecorder()
		err = createUser(recorder, request)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e.Message, gocheck.Equals, "Password length should be least 6 characters and at most 50 characters.")
	}
}

func (s *AuthSuite) TestCreateUserCreatesUserInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@me.myself","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": "nobody@me.myself"})
	err = createUser(recorder, request)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url[0], gocheck.Equals, "/user")
	expected := `{"name":"nobody@me.myself","keys":{}}`
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
	c.Assert(h.method[0], gocheck.Equals, "POST")
}

func (s *AuthSuite) TestCreateUserFailWithRegistrationDisabled(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	oldUserRegistration, err := config.GetBool("auth:user-registration")
	c.Assert(err, gocheck.IsNil)
	config.Set("auth:user-registration", false)
	defer config.Set("auth:user-registration", oldUserRegistration)
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.Equals, createDisabledErr)
}

func (s *AuthSuite) TestCreateUserFailWithRegistrationDisabledAndCommonUser(c *gocheck.C) {
	simpleUser := &auth.User{Email: "my@common.user", Password: "123456"}
	_, err := nativeScheme.Create(simpleUser)
	c.Assert(err, gocheck.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": simpleUser.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	oldUserRegistration, err := config.GetBool("auth:user-registration")
	c.Assert(err, gocheck.IsNil)
	config.Set("auth:user-registration", false)
	defer config.Set("auth:user-registration", oldUserRegistration)
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.Equals, createDisabledErr)
}

func (s *AuthSuite) TestCreateUserWorksWithRegistrationDisabledAndAdminUser(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	oldUserRegistration, err := config.GetBool("auth:user-registration")
	c.Assert(err, gocheck.IsNil)
	config.Set("auth:user-registration", false)
	defer config.Set("auth:user-registration", oldUserRegistration)
	recorder := httptest.NewRecorder()
	err = createUser(recorder, request)
	c.Assert(err, gocheck.IsNil)
}

func (s *AuthSuite) TestCreateUserRollsbackAfterGandalfError(c *gocheck.C) {
	h := testHandler{rspCode: http.StatusInternalServerError}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusInternalServerError)
	_, err = auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, gocheck.NotNil)
}

func (s *AuthSuite) TestLoginShouldCreateTokenInTheDatabaseAndReturnItWithinTheResponse(c *gocheck.C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.IsNil)
	var user auth.User
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": "nobody@globo.com"}).One(&user)
	var recorderJSON map[string]string
	r, _ := ioutil.ReadAll(recorder.Body)
	json.Unmarshal(r, &recorderJSON)
	n, err := conn.Tokens().Find(bson.M{"token": recorderJSON["token"]}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
	action := rectest.Action{
		Action: "login",
		User:   u.Email,
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestLoginShouldInformWhenUserIsNotAdmin(c *gocheck.C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.IsNil)
	var user auth.User
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": "nobody@globo.com"}).One(&user)
	var recorderJSON map[string]interface{}
	r, _ := ioutil.ReadAll(recorder.Body)
	json.Unmarshal(r, &recorderJSON)
	c.Assert(recorderJSON["is_admin"], gocheck.Equals, false)
}

func (s *AuthSuite) TestLoginShouldInformWhenUserIsAdmin(c *gocheck.C) {
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/whydidifall@thewho.com/tokens?:email=whydidifall@thewho.com", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.IsNil)
	var user auth.User
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": "whydidifall@thewho.com"}).One(&user)
	var recorderJSON map[string]interface{}
	r, _ := ioutil.ReadAll(recorder.Body)
	json.Unmarshal(r, &recorderJSON)
	c.Assert(recorderJSON["is_admin"], gocheck.Equals, true)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndBadRequestIfItReceivesAnInvalidJSON(c *gocheck.C) {
	b := bytes.NewBufferString(`"invalid":"json"]`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Invalid JSON$")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndBadRequestIfTheJSONDoesNotContainsAPassword(c *gocheck.C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^You must provide a password to login$")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndNotFoundIfTheUserDoesNotExist(c *gocheck.C) {
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^user not found$")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestLoginShouldreturnErrorIfThePasswordDoesNotMatch(c *gocheck.C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	b := bytes.NewBufferString(`{"password":"1234567"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Authentication failed, wrong password.$")
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusUnauthorized)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndInternalServerErrorIfReadAllFails(c *gocheck.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	err := b.Close()
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.NotNil)
}

func (s *AuthSuite) TestLoginShouldReturnBadRequestIfEmailIsNotValid(c *gocheck.C) {
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody/token?:email=nobody", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, native.ErrInvalidEmail.Error())
}

func (s *AuthSuite) TestLoginShouldReturnBadRequestWhenPasswordIsInvalid(c *gocheck.C) {
	passwords := []string{"123", strings.Join(make([]string, 52), "-")}
	u := &auth.User{Email: "me@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, gocheck.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	for _, password := range passwords {
		b := bytes.NewBufferString(`{"password":"` + password + `"}`)
		request, err := http.NewRequest("POST", "/users/me@globo.com/token?:email=me@globo.com", b)
		c.Assert(err, gocheck.IsNil)
		request.Header.Set("Content-type", "application/json")
		recorder := httptest.NewRecorder()
		err = login(recorder, request)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e.Message, gocheck.Matches, "Password.*")
	}
}

func (s *AuthSuite) TestLogout(c *gocheck.C) {
	token, err := nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("DELETE", "/users/tokens", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = logout(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	_, err = nativeScheme.Auth(token.GetValue())
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *AuthSuite) TestCreateTeamHandlerSavesTheTeamInTheDatabaseWithTheAuthenticatedUser(c *gocheck.C) {
	b := bytes.NewBufferString(`{"name":"timeredbull"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createTeam(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	t := new(auth.Team)
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Teams().Find(bson.M{"_id": "timeredbull"}).One(t)
	defer conn.Teams().Remove(bson.M{"_id": "timeredbull"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, ContainsUser, s.user)
	action := rectest.Action{
		Action: "create-team",
		User:   s.user.Email,
		Extra:  []interface{}{"timeredbull"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnsBadRequestIfTheRequestBodyIsAnInvalidJSON(c *gocheck.C) {
	b := bytes.NewBufferString(`{"name"["invalidjson"]}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnsBadRequestIfTheNameIsNotGiven(c *gocheck.C) {
	b := bytes.NewBufferString(`{"genre":"male"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, auth.ErrInvalidTeamName.Error())
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnConflictIfTheTeamToBeCreatedAlreadyExists(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	err := conn.Teams().Insert(bson.M{"_id": "timeredbull"})
	defer conn.Teams().Remove(bson.M{"_id": "timeredbull"})
	c.Assert(err, gocheck.IsNil)
	b := bytes.NewBufferString(`{"name":"timeredbull"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = createTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
	c.Assert(e, gocheck.ErrorMatches, "^team already exists$")
}

func (s *AuthSuite) TestRemoveTeam(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	team := auth.Team{Name: "painofsalvation", Users: []string{s.user.Email}}
	err := conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": team.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	n, err := conn.Teams().Find(bson.M{"name": team.Name}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
	action := rectest.Action{
		Action: "remove-team",
		User:   s.user.Email,
		Extra:  []interface{}{team.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveTeamGives404WhenTeamDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("DELETE", "/teams/unknown?:name=unknown", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e.Message, gocheck.Equals, `Team "unknown" not found.`)
}

func (s *AuthSuite) TestRemoveTeamGives404WhenUserDoesNotHaveAccessToTheTeam(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	team := auth.Team{Name: "painofsalvation"}
	err := conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": team.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e.Message, gocheck.Equals, `Team "painofsalvation" not found.`)
}

func (s *AuthSuite) TestRemoveTeamGives403WhenTeamHasAccessToAnyApp(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	team := auth.Team{Name: "evergrey", Users: []string{s.user.Email}}
	err := conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": team.Name})
	a := App{Name: "i-should", Teams: []string{team.Name}}
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	expected := `This team cannot be removed because it have access to apps.

Please remove the apps or revoke these accesses, and try again.`
	c.Assert(e.Message, gocheck.Equals, expected)
}

func (s *AuthSuite) TestListTeamsListsAllTeamsThatTheUserIsMember(c *gocheck.C) {
	request, err := http.NewRequest("GET", "/teams", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = teamList(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "application/json")
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var m []map[string]string
	err = json.Unmarshal(b, &m)
	c.Assert(err, gocheck.IsNil)
	c.Assert(m, gocheck.DeepEquals, []map[string]string{{"name": s.team.Name}})
	action := rectest.Action{
		Action: "list-teams",
		User:   s.user.Email,
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestListTeamsReturns204IfTheUserHasNoTeam(c *gocheck.C) {
	u := auth.User{Email: "cruiser@gotthard.com", Password: "234567"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "234567"})
	c.Assert(err, gocheck.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("GET", "/teams", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = teamList(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNoContent)
}

func (s *AuthSuite) TestAddUserToTeam(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, gocheck.IsNil)
	url := "/teams/tsuruteam/wolverine@xmen.com?:team=tsuruteam&:user=wolverine@xmen.com"
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	t := new(auth.Team)
	err = conn.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, ContainsUser, s.user)
	c.Assert(t, ContainsUser, u)
	action := rectest.Action{
		Action: "add-user-to-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=" + u.Email},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnNotFoundIfThereIsNoTeamWithTheGivenName(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("PUT", "/teams/abc/me@me.me?:team=abc&:user=me@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Team not found$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnForbiddenIfTheGivenUserIsNotInTheGivenTeam(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "hi@me.me", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, gocheck.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("PUT", "/teams/tsuruteam/hi@me.me?:team=tsuruteam&:user=hi@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addUserToTeam(recorder, request, token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^You are not authorized to add new users to the team tsuruteam$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnNotFoundIfTheEmailInTheBodyDoesNotExistInTheDatabase(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("PUT", "/teams/tsuruteam/hi2@me.me?:team=tsuruteam&:user=hi2@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^User not found$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnConflictIfTheUserIsAlreadyInTheGroup(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, s.user.Email, s.team.Name, s.user.Email)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
}

func (s *AuthSuite) TestAddUserToTeamShoulGrantAccessInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "marathon@rush.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, gocheck.IsNil)
	t, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer nativeScheme.Logout(t.GetValue())
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	a := App{Name: "i-should", Teams: []string{s.team.Name}}
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, t)
	c.Assert(err, gocheck.IsNil)
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, u.Email, s.team.Name, u.Email)
	request, err = http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Check(len(h.url), gocheck.Equals, 2)
	c.Assert(h.url[1], gocheck.Equals, "/repository/grant")
	c.Assert(h.method[1], gocheck.Equals, "POST")
	expected := fmt.Sprintf(`{"repositories":["%s"],"users":["marathon@rush.com"]}`, a.Name)
	c.Assert(string(h.body[1]), gocheck.Equals, expected)
}

func (s *AuthSuite) TestAddUserToTeamInDatabase(c *gocheck.C) {
	user := &auth.User{Email: "nobody@gmail.com", Password: "123456"}
	team := &auth.Team{Name: "myteam"}
	conn, _ := db.Conn()
	defer conn.Close()
	err := conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	defer conn.Teams().RemoveId(team.Name)
	err = addUserToTeamInDatabase(user, team)
	c.Assert(err, gocheck.IsNil)
	conn.Teams().FindId(team.Name).One(team)
	c.Assert(team.Users, gocheck.DeepEquals, []string{user.Email})
}

func (s *AuthSuite) TestAddUserToTeamInGandalfShouldCallGandalfAPI(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "nonee@me.me", Password: "none"}
	err := addUserToTeamInGandalf(&u, s.team)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(h.url), gocheck.Equals, 1)
	c.Assert(h.url[0], gocheck.Equals, "/repository/grant")
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldRemoveAUserFromATeamIfTheTeamExistAndTheUserIsMemberOfTheTeam(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "nonee@me.me"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	s.team.AddUser(&u)
	conn.Teams().Update(bson.M{"_id": s.team.Name}, s.team)
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/nonee@me.me?:team=tsuruteam&:user=nonee@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	err = conn.Teams().Find(bson.M{"_id": s.team.Name}).One(s.team)
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.team, gocheck.Not(ContainsUser), &u)
	action := rectest.Action{
		Action: "remove-user-from-team",
		User:   s.user.Email,
		Extra:  []interface{}{"team=tsuruteam", "user=" + u.Email},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldRemoveOnlyAppsInThatTeamInGandalfWhenUserIsInMoreThanOneTeam(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "nobody@me.me"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	s.team.AddUser(&u)
	conn.Teams().UpdateId(s.team.Name, s.team)
	team2 := auth.Team{Name: "team2", Users: []string{u.Email}}
	err = conn.Teams().Insert(&team2)
	c.Assert(err, gocheck.IsNil)
	defer conn.Teams().RemoveId(team2.Name)
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}}
	err = conn.Apps().Insert(&app1)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": app1.Name})
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name, team2.Name}}
	err = conn.Apps().Insert(&app2)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": app2.Name})
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, u.Email, s.team.Name, u.Email)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	expected := `{"repositories":["app1"],"users":["nobody@me.me"]}`
	c.Assert(len(h.body), gocheck.Equals, 1)
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
	conn.Teams().FindId(s.team.Name).One(s.team)
	c.Assert(s.team, gocheck.Not(ContainsUser), &u) // just in case
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnNotFoundIfTheTeamDoesNotExist(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/none@me.me?:team=unknown&:user=none@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Team not found$")
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnUnauthorizedIfTheGivenUserIsNotMemberOfTheTeam(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/none@me.me?:team=tsuruteam&:user=none@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	u := &auth.User{Email: "unknown@gmail.com", Password: "123456"}
	_, err = nativeScheme.Create(u)
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusUnauthorized)
	c.Assert(e, gocheck.ErrorMatches, "^You are not authorized to remove a member from the team tsuruteam")
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnNotFoundWhenTheUserIsNotMemberOfTheTeam(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
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
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnForbiddenIfTheUserIsTheLastInTheTeam(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	url := "/teams/tsuruteam/whydidifall@thewho.com?:team=tsuruteam&:user=whydidifall@thewho.com"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^You can not remove this user from this team, because it is the last user within the team, and a team can not be orphaned$")
}

func (s *AuthSuite) TestRemoveUserFromTeamRevokesAccessInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "pomar@nando-reis.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, gocheck.IsNil)
	t, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer nativeScheme.Logout(t.GetValue())
	defer conn.Users().Remove(bson.M{"email": u.Email})
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, t)
	c.Assert(err, gocheck.IsNil)
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, u.Email, s.team.Name, u.Email)
	request, err = http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	err = addUserToTeam(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	a := struct {
		Name  string
		Teams []string
	}{Name: "myApp", Teams: []string{s.team.Name}}
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	url = fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, u.Email, s.team.Name, u.Email)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	err = removeUserFromTeam(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url[2], gocheck.Equals, "/repository/revoke")
	c.Assert(h.method[2], gocheck.Equals, "DELETE")
	expected := `{"repositories":["myApp"],"users":["pomar@nando-reis.com"]}`
	c.Assert(string(h.body[2]), gocheck.Equals, expected)
}

func (s *AuthSuite) TestRemoveUserFromTeamInDatabase(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "nobody@gmail.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	s.team.AddUser(u)
	err = conn.Teams().UpdateId(s.team.Name, s.team)
	c.Assert(err, gocheck.IsNil)
	err = removeUserFromTeamInDatabase(u, s.team)
	c.Assert(err, gocheck.IsNil)
	err = conn.Teams().FindId(s.team.Name).One(s.team)
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.team, gocheck.Not(ContainsUser), u)
}

func (s *AuthSuite) TestRemoveUserFromTeamInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "nobody@gmail.com"}
	err := removeUserFromTeamInGandalf(u, &auth.Team{Name: "someteam"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(h.url), gocheck.Equals, 1)
	c.Assert(h.url[0], gocheck.Equals, "/repository/revoke")
}

func (s *AuthSuite) TestGetTeam(c *gocheck.C) {
	team, err := auth.GetTeam(s.team.Name)
	c.Assert(err, gocheck.IsNil)
	url := fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = getTeam(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "application/json")
	var got auth.Team
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, *team)
	action := rectest.Action{
		User:   s.user.Email,
		Action: "get-team",
		Extra:  []interface{}{team.Name},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestGetTeamNotFound(c *gocheck.C) {
	url := "/teams/unknown?:name=unknown"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = getTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e.Message, gocheck.Equals, "Team not found")
}

func (s *AuthSuite) TestGetTeamForbidden(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	team := auth.Team{Name: "paradisum", Users: []string{"someuser@me.com"}}
	conn.Teams().Insert(team)
	defer conn.Teams().RemoveId(team.Name)
	url := fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = getTeam(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e.Message, gocheck.Equals, "User is not member of this team")
}

func (s *AuthSuite) TestAddKeyToUserAddsAKeyToTheUser(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	defer func() {
		s.user.RemoveKey(auth.Key{Content: "my-key"})
		conn.Users().Update(bson.M{"email": s.user.Email}, s.user)
	}()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	s.user, err = auth.GetUserByEmail(s.user.Email)
	c.Assert(err, gocheck.IsNil)
	key := auth.Key{Name: s.user.Email + "-1"}
	gotKey, _ := s.user.FindKey(key)
	key.Content = "my-key"
	c.Assert(gotKey, gocheck.DeepEquals, key)
	action := rectest.Action{
		Action: "add-key",
		User:   s.user.Email,
		Extra:  []interface{}{"", "my-key"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestAddKeyToUserAcceptsTheNameOfTheKey(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	defer func() {
		s.user.RemoveKey(auth.Key{Content: "my-key"})
		conn.Users().Update(bson.M{"email": s.user.Email}, s.user)
	}()
	b := bytes.NewBufferString(`{"key":"my-key","name":"super-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	s.user, err = auth.GetUserByEmail(s.user.Email)
	c.Assert(err, gocheck.IsNil)
	key := auth.Key{Name: "super-key"}
	gotKey, _ := s.user.FindKey(key)
	key.Content = "my-key"
	c.Assert(gotKey, gocheck.DeepEquals, key)
	action := rectest.Action{
		Action: "add-key",
		User:   s.user.Email,
		Extra:  []interface{}{"super-key", "my-key"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestAddKeyToUserReturnsErrorIfTheReadingOfTheBodyFails(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := s.getTestData("bodyToBeClosed.txt")
	b.Close()
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *AuthSuite) TestAddKeyToUserReturnsBadRequestIfTheJSONIsInvalid(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`"aaaa}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Invalid JSON$")
}

func (s *AuthSuite) TestAddKeyToUserReturnsBadRequestIfTheKeyIsNotPresent(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Missing key content$")
}

func (s *AuthSuite) TestAddKeyToUserReturnsBadRequestIfTheKeyIsEmpty(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"key":""}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Missing key content$")
}

func (s *AuthSuite) TestAddKeyToUserReturnsConflictIfTheKeyIsAlreadyPresent(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	s.user.AddKey(auth.Key{Content: "my-key"})
	conn.Users().Update(bson.M{"email": s.user.Email}, s.user)
	defer func() {
		s.user.RemoveKey(auth.Key{Content: "my-key"})
		conn.Users().Update(bson.M{"email": s.user.Email}, s.user)
	}()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
	c.Assert(e.Message, gocheck.Equals, "user already has this key")
}

func (s *AuthSuite) TestAddKeyAddKeyToUserInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "francisco@franciscosouza.net", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, gocheck.IsNil)
	t, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer nativeScheme.Logout(t.GetValue())
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, t)
	c.Assert(err, gocheck.IsNil)
	u, err = auth.GetUserByEmail(u.Email)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		u.RemoveKey(u.Keys[0])
		conn.Users().RemoveAll(bson.M{"email": u.Email})
	}()
	c.Assert(u.Keys[0].Name, gocheck.Not(gocheck.Matches), "\\.pub$")
	expectedURL := fmt.Sprintf("/user/%s/key", u.Email)
	c.Assert(h.url[0], gocheck.Equals, expectedURL)
	c.Assert(h.method[0], gocheck.Equals, "POST")
	expected := fmt.Sprintf(`{"%s-1":"my-key"}`, u.Email)
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
}

func (s *AuthSuite) TestAddKeyToUserShouldNotInsertKeyInDatabaseWhenGandalfAdditionFails(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, gocheck.IsNil)
	t, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer nativeScheme.Logout(t.GetValue())
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, t)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to add key to git server: Failed to connect to Gandalf server, it's probably down.")
	defer conn.Users().RemoveAll(bson.M{"email": u.Email})
	u2, err := auth.GetUserByEmail(u.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2.Keys, gocheck.DeepEquals, []auth.Key{})
}

func (s *AuthSuite) TestRemoveKeyHandlerRemovesTheKeyFromTheUser(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	addKeyToUser(recorder, request, s.token)
	b = bytes.NewBufferString(`{"key":"my-key"}`)
	request, err = http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	u2, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2.HasKey(auth.Key{Content: "my-key"}), gocheck.Equals, false)
	action := rectest.Action{
		Action: "remove-key",
		User:   s.user.Email,
		Extra:  []interface{}{"", "my-key"},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveKeyHandlerCanRemoveByName(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"key":"my-key","name":"key-name"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	addKeyToUser(recorder, request, s.token)
	b = bytes.NewBufferString(`{"name":"key-name"}`)
	request, err = http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	u2, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2.HasKey(auth.Key{Content: "my-key"}), gocheck.Equals, false)
	action := rectest.Action{
		Action: "remove-key",
		User:   s.user.Email,
		Extra:  []interface{}{"key-name", ""},
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveKeyHandlerCallsGandalfRemoveKey(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	b = bytes.NewBufferString(`{"key":"my-key"}`)
	request, err = http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder = httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	s.user, _ = auth.GetUserByEmail(s.user.Email)
	c.Assert(h.url[1], gocheck.Equals, fmt.Sprintf("/user/%s/key/%s-%d", s.user.Email, s.user.Email, len(s.user.Keys)+1))
	c.Assert(h.method[1], gocheck.Equals, "DELETE")
	c.Assert(string(h.body[1]), gocheck.Equals, "null")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsErrorInCaseOfAnyIOFailure(c *gocheck.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	b.Close()
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheJSONIsInvalid(c *gocheck.C) {
	b := bytes.NewBufferString(`invalid"json}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Invalid JSON$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheKeyIsNotPresent(c *gocheck.C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Either the content or the name of the key must be provided$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheKeyIsEmpty(c *gocheck.C) {
	b := bytes.NewBufferString(`{"key":""}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Either the content or the name of the key must be provided$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsNotFoundIfTheUserDoesNotHaveTheKey(c *gocheck.C) {
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeKeyFromUser(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestListKeysHandler(c *gocheck.C) {
	h := testHandler{
		content: `{"homekey": "lol somekey somecomment", "workkey": "lol someotherkey someothercomment"}`,
	}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/users/cartman@south.park/keys?email=cartman@south.park", nil)
	c.Assert(err, gocheck.IsNil)
	err = listKeys(recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
	got := map[string]string{}
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, gocheck.IsNil)
	expected := map[string]string{
		"homekey": "lol somekey somecomment",
		"workkey": "lol someotherkey someothercomment",
	}
	c.Assert(expected, gocheck.DeepEquals, got)
}

func (s *AuthSuite) TestListKeysRepassesGandalfsErrors(c *gocheck.C) {
	h := testBadHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/users/cartman@south.park/keys?email=cartman@south.park", nil)
	err = listKeys(recorder, request, s.token)
	c.Assert(err.Error(), gocheck.Equals, "some error\n")
}

func (s *AuthSuite) TestRemoveUser(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "her-voices@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	n, err := conn.Users().Find(bson.M{"email": u.Email}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
	action := rectest.Action{Action: "remove-user", User: u.Email}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestRemoveUserWithTheUserBeingLastMemberOfATeam(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	t := auth.Team{Name: "painofsalvation", Users: []string{u.Email}}
	err = conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": t.Name})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	expected := `This user is the last member of the team "painofsalvation", so it cannot be removed.

Please remove the team, then remove the user.`
	c.Assert(e.Message, gocheck.Equals, expected)
}

func (s *AuthSuite) TestRemoveUserShouldRemoveTheUserFromAllTeamsThatHeIsMember(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	t := auth.Team{Name: "painofsalvation", Users: []string{u.Email, s.user.Email}}
	err = conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": t.Name})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	err = conn.Teams().Find(bson.M{"_id": t.Name}).One(&t)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.Users, gocheck.HasLen, 1)
	c.Assert(t.Users[0], gocheck.Equals, s.user.Email)
}

type App struct {
	Name  string
	Teams []string
}

func (s *AuthSuite) TestRemoveUserRevokesAccessInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	t := auth.Team{Name: "painofsalvation", Users: []string{u.Email, s.user.Email}}
	err = conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": t.Name})
	a := struct {
		Name  string
		Teams []string
	}{Name: "myApp", Teams: []string{t.Name}}
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url[0], gocheck.Equals, "/repository/revoke")
	c.Assert(h.method[0], gocheck.Equals, "DELETE")
	expected := `{"repositories":["myApp"],"users":["of-two-beginnings@painofsalvation.com"]}`
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
}

func (s *AuthSuite) TestChangePasswordHandler(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@globo.com.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, gocheck.IsNil)
	oldPassword := u.Password
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	body := bytes.NewBufferString(`{"old":"123456","new":"654321"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = changePassword(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	otherUser, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(otherUser.Password, gocheck.Not(gocheck.Equals), oldPassword)
	action := rectest.Action{
		Action: "change-password",
		User:   u.Email,
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestChangePasswordReturns412IfNewPasswordIsInvalid(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@globo.com.com", Password: "123456"}
	_, err := nativeScheme.Create(u)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	body := bytes.NewBufferString(`{"old":"123456","new":"1234"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = changePassword(recorder, request, token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Check(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Check(e.Message, gocheck.Equals, "Password length should be least 6 characters and at most 50 characters.")
}

func (s *AuthSuite) TestChangePasswordReturns404IfOldPasswordDidntMatch(c *gocheck.C) {
	body := bytes.NewBufferString(`{"old":"1234","new":"123456"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = changePassword(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Check(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Check(e.Message, gocheck.Equals, "The given password didn't match the user's current password.")
}

func (s *AuthSuite) TestChangePasswordReturns400IfRequestBodyIsInvalidJSON(c *gocheck.C) {
	body := bytes.NewBufferString(`{"invalid:"json`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = changePassword(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "Invalid JSON.")
}

func (s *AuthSuite) TestChangePasswordReturns400IfJSONDoesNotIncludeBothOldAndNewPasswords(c *gocheck.C) {
	bodies := []string{`{"old": "something"}`, `{"new":"something"}`, "{}", "null"}
	for _, body := range bodies {
		b := bytes.NewBufferString(body)
		request, err := http.NewRequest("PUT", "/users/password", b)
		c.Assert(err, gocheck.IsNil)
		recorder := httptest.NewRecorder()
		err = changePassword(recorder, request, s.token)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e.Message, gocheck.Equals, "Both the old and the new passwords are required.")
	}
}

func (s *AuthSuite) TestResetPasswordStep1(c *gocheck.C) {
	defer s.server.Reset()
	oldPassword := s.user.Password
	url := fmt.Sprintf("/users/%s/password?:email=%s", s.user.Email, s.user.Email)
	request, _ := http.NewRequest("POST", url, nil)
	recorder := httptest.NewRecorder()
	err := resetPassword(recorder, request)
	c.Assert(err, gocheck.IsNil)
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	var m map[string]interface{}
	err = conn.PasswordTokens().Find(bson.M{"useremail": s.user.Email}).One(&m)
	c.Assert(err, gocheck.IsNil)
	defer conn.PasswordTokens().RemoveId(m["_id"])
	time.Sleep(1e9)
	s.server.RLock()
	defer s.server.RUnlock()
	c.Assert(s.server.MailBox, gocheck.HasLen, 1)
	u, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u.Password, gocheck.Equals, oldPassword)
	action := rectest.Action{
		Action: "reset-password-gen-token",
		User:   s.user.Email,
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestResetPasswordUserNotFound(c *gocheck.C) {
	url := "/users/unknown@tsuru.io/password?:email=unknown@tsuru.io"
	request, _ := http.NewRequest("POST", url, nil)
	recorder := httptest.NewRecorder()
	err := resetPassword(recorder, request)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e.Message, gocheck.Equals, "user not found")
}

func (s *AuthSuite) TestResetPasswordInvalidEmail(c *gocheck.C) {
	url := "/users/unknown/password?:email=unknown"
	request, _ := http.NewRequest("POST", url, nil)
	recorder := httptest.NewRecorder()
	err := resetPassword(recorder, request)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "Invalid email.")
}

func (s *AuthSuite) TestResetPasswordStep2(c *gocheck.C) {
	conn, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	defer conn.Close()
	user := auth.User{Email: "uns@alanis.com", Password: "145678"}
	err = user.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	oldPassword := user.Password
	err = nativeScheme.StartPasswordReset(&user)
	c.Assert(err, gocheck.IsNil)
	var t map[string]interface{}
	err = conn.PasswordTokens().Find(bson.M{"useremail": user.Email}).One(&t)
	c.Assert(err, gocheck.IsNil)
	url := fmt.Sprintf("/users/%s/password?:email=%s&token=%s", user.Email, user.Email, t["_id"])
	request, _ := http.NewRequest("POST", url, nil)
	recorder := httptest.NewRecorder()
	err = resetPassword(recorder, request)
	c.Assert(err, gocheck.IsNil)
	u2, err := auth.GetUserByEmail(user.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2.Password, gocheck.Not(gocheck.Equals), oldPassword)
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

func (s *AuthSuite) TestAuthScheme(c *gocheck.C) {
	oldScheme := app.AuthScheme
	defer func() { app.AuthScheme = oldScheme }()
	app.AuthScheme = TestScheme{}
	request, _ := http.NewRequest("GET", "/auth/scheme", nil)
	recorder := httptest.NewRecorder()
	err := authScheme(recorder, request)
	c.Assert(err, gocheck.IsNil)
	var parsed map[string]interface{}
	err = json.NewDecoder(recorder.Body).Decode(&parsed)
	c.Assert(err, gocheck.IsNil)
	c.Assert(parsed["name"], gocheck.Equals, "test")
	c.Assert(parsed["data"], gocheck.DeepEquals, map[string]interface{}{"foo": "bar", "foo2": "bar2"})
}

func (s *AuthSuite) TestRegenerateAPITokenHandler(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "zobomafoo@zimbabue.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("POST", "/users/api-key", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = regenerateAPIToken(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, gocheck.IsNil)
	count, err := conn.Users().Find(bson.M{"apikey": got}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 1)
}

func (s *AuthSuite) TestShowAPITokenForUserWithNoToken(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "zobomafoo@zimbabue.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("GET", "/users/api-key", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = showAPIToken(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, gocheck.IsNil)
	count, err := conn.Users().Find(bson.M{"apikey": got}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 1)
}

func (s *AuthSuite) TestShowAPITokenForUserWithToken(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "zobomafoo@zimbabue.com", Password: "123456", APIKey: "238hd23ubd923hd923j9d23ndibde"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("GET", "/users/api-key", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = showAPIToken(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.Equals, "238hd23ubd923hd923j9d23ndibde")
}

func (s *AuthSuite) TestListUsers(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	token, err := nativeScheme.Login(map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, gocheck.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("GET", "/users", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = listUsers(recorder, request, token)
	c.Assert(err, gocheck.IsNil)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(users), gocheck.Equals, 1)
	c.Assert(users[0].Email, gocheck.Equals, s.user.Email)
	c.Assert(users[0].Teams, gocheck.DeepEquals, []string{s.team.Name})
}
