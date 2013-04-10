// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strconv"
	"strings"
)

type AuthSuite struct {
	team *auth.Team
	user *auth.User
}

var _ = gocheck.Suite(&AuthSuite{})

func (s *AuthSuite) SetUpSuite(c *gocheck.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_auth_test")
	config.Set("auth:salt", "tsuru-salt")
	config.Set("auth:token-key", "TSURU-KEY")
	config.Set("auth:hash-cost", 4)
	s.createUserAndTeam(c)
	config.Set("admin-team", s.team.Name)
}

func (s *AuthSuite) TearDownSuite(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *AuthSuite) TearDownTest(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	_, err := conn.Users().RemoveAll(bson.M{"email": bson.M{"$ne": s.user.Email}})
	c.Assert(err, gocheck.IsNil)
	_, err = conn.Teams().RemoveAll(bson.M{"_id": bson.M{"$ne": s.team.Name}})
	c.Assert(err, gocheck.IsNil)
	s.user.Password = "123"
	s.user.HashPassword()
	err = s.user.Update()
	c.Assert(err, gocheck.IsNil)
}

func (s *AuthSuite) createUserAndTeam(c *gocheck.C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123"}
	err := s.user.Create()
	c.Assert(err, gocheck.IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Teams().Insert(s.team)
	c.Assert(err, gocheck.IsNil)
}

// starts a new httptest.Server and returns it
// Also changes git:host, git:port and git:protocol to match the server's url
func (s *AuthSuite) startGandalfTestServer(h http.Handler) *httptest.Server {
	ts := httptest.NewServer(h)
	pieces := strings.Split(ts.URL, "://")
	protocol := pieces[0]
	hostPart := strings.Split(pieces[1], ":")
	port := hostPart[1]
	host := hostPart[0]
	config.Set("git:host", host)
	portInt, _ := strconv.ParseInt(port, 10, 0)
	config.Set("git:port", portInt)
	config.Set("git:protocol", protocol)
	return ts
}

func (s *AuthSuite) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}

type hasKeyChecker struct{}

func (c *hasKeyChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "HasKey", Params: []string{"user", "key"}}
}

func (c *hasKeyChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you should provide two parameters"
	}
	user, ok := params[0].(*auth.User)
	if !ok {
		return false, "first parameter should be a user pointer"
	}
	content, ok := params[1].(string)
	if !ok {
		return false, "second parameter should be a string"
	}
	key := auth.Key{Content: content}
	return user.HasKey(key), ""
}

var HasKey gocheck.Checker = &hasKeyChecker{}

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
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, gocheck.IsNil)
	_, err = auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, gocheck.IsNil)
}

func (s *AuthSuite) TestCreateUserHandlerReturnsStatus201AfterCreateTheUser(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
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
	err = CreateUser(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^.*bad file descriptor$")
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorAndBadRequestIfInvalidJSONIsGiven(c *gocheck.C) {
	b := bytes.NewBufferString(`["invalid json":"i'm invalid"]`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^invalid character.*$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorAndConflictIfItFailsToCreateUser(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	u.Create()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "This email is already registered")
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
}

func (s *AuthSuite) TestCreateUserHandlerReturnsPreconditionFailedIfEmailIsNotValid(c *gocheck.C) {
	b := bytes.NewBufferString(`{"email":"nobody","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, gocheck.Equals, "Invalid email.")
}

func (s *AuthSuite) TestCreateUserHandlerReturnsPreconditionFailedIfPasswordHasLessThan6CharactersOrMoreThan50Characters(c *gocheck.C) {
	passwords := []string{"123", strings.Join(make([]string, 52), "-")}
	for _, password := range passwords {
		b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"` + password + `"}`)
		request, err := http.NewRequest("POST", "/users", b)
		c.Assert(err, gocheck.IsNil)
		request.Header.Set("Content-type", "application/json")
		recorder := httptest.NewRecorder()
		err = CreateUser(recorder, request)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusPreconditionFailed)
		c.Assert(e.Message, gocheck.Equals, "Password length should be least 6 characters and at most 50 characters.")
	}
}

func (s *AuthSuite) TestCreateUserCreatesUserInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@me.myself","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": "nobody@me.myself"})
	err = CreateUser(recorder, request)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url[0], gocheck.Equals, "/user")
	expected := `{"name":"nobody@me.myself","keys":{}}`
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
	c.Assert(h.method[0], gocheck.Equals, "POST")
}

func (s *AuthSuite) TestLoginShouldCreateTokenInTheDatabaseAndReturnItWithinTheResponse(c *gocheck.C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	u.Create()
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
	var recorderJson map[string]string
	r, _ := ioutil.ReadAll(recorder.Body)
	json.Unmarshal(r, &recorderJson)
	n, err := conn.Tokens().Find(bson.M{"token": recorderJson["token"]}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndBadRequestIfItReceivesAnInvalidJson(c *gocheck.C) {
	b := bytes.NewBufferString(`"invalid":"json"]`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Invalid JSON$")
	e, ok := err.(*errors.Http)
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
	e, ok := err.(*errors.Http)
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
	c.Assert(err, gocheck.ErrorMatches, "^User not found$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestLoginShouldreturnErrorIfThePasswordDoesNotMatch(c *gocheck.C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	u.Create()
	b := bytes.NewBufferString(`{"password":"1234567"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^Authentication failed, wrong password.$")
	e, ok := err.(*errors.Http)
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

func (s *AuthSuite) TestLoginShouldReturnPreconditionFailedIfEmailIsNotValid(c *gocheck.C) {
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody/token?:email=nobody", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = login(recorder, request)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, gocheck.Equals, emailError)
}

func (s *AuthSuite) TestLoginShouldReturnPreconditionFailedWhenPasswordIsInvalid(c *gocheck.C) {
	passwords := []string{"123", strings.Join(make([]string, 52), "-")}
	u := &auth.User{Email: "me@globo.com", Password: "123"}
	err := u.Create()
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
		e, ok := err.(*errors.Http)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusPreconditionFailed)
		c.Assert(e.Message, gocheck.Equals, passwordError)
	}
}

func (s *AuthSuite) TestCreateTeamHandlerSavesTheTeamInTheDatabaseWithTheAuthenticatedUser(c *gocheck.C) {
	b := bytes.NewBufferString(`{"name":"timeredbull"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, gocheck.IsNil)
	t := new(auth.Team)
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Teams().Find(bson.M{"_id": "timeredbull"}).One(t)
	defer conn.Teams().Remove(bson.M{"_id": "timeredbull"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, ContainsUser, s.user)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnsBadRequestIfTheRequestBodyIsAnInvalidJSON(c *gocheck.C) {
	b := bytes.NewBufferString(`{"name"["invalidjson"]}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnsBadRequestIfTheNameIsNotGiven(c *gocheck.C) {
	b := bytes.NewBufferString(`{"genre":"male"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^You must provide the team name$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnsInternalServerErrorIfReadAllFails(c *gocheck.C) {
	b := s.getTestData("bodyToBeClosed.txt")
	err := b.Close()
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
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
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
	c.Assert(e, gocheck.ErrorMatches, "^This team already exists$")
}

func (s *AuthSuite) TestKeyToMap(c *gocheck.C) {
	keys := []auth.Key{{Name: "testkey", Content: "somekey"}}
	kMap := keyToMap(keys)
	c.Assert(kMap, gocheck.DeepEquals, map[string]string{"testkey": "somekey"})
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
	err = RemoveTeam(recorder, request, s.user)
	c.Assert(err, gocheck.IsNil)
	n, err := conn.Teams().Find(bson.M{"name": team.Name}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
}

func (s *AuthSuite) TestRemoveTeamGives404WhenTeamDoesNotExist(c *gocheck.C) {
	request, err := http.NewRequest("DELETE", "/teams/unknown?:name=unknown", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
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
	err = RemoveTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
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
	err = RemoveTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
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
	err = ListTeams(recorder, request, s.user)
	c.Assert(err, gocheck.IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, gocheck.IsNil)
	var m []map[string]string
	err = json.Unmarshal(b, &m)
	c.Assert(err, gocheck.IsNil)
	c.Assert(m, gocheck.DeepEquals, []map[string]string{{"name": s.team.Name}})
}

func (s *AuthSuite) TestListTeamsReturns204IfTheUserHasNoTeam(c *gocheck.C) {
	u := auth.User{Email: "cruiser@gotthard.com", Password: "123"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	request, err := http.NewRequest("GET", "/teams", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = ListTeams(recorder, request, &u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusNoContent)
}

func (s *AuthSuite) TestAddUserToTeam(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	url := "/teams/tsuruteam/wolverine@xmen.com?:team=tsuruteam&:user=wolverine@xmen.com"
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, gocheck.IsNil)
	t := new(auth.Team)
	err = conn.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t, ContainsUser, s.user)
	c.Assert(t, ContainsUser, u)
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnNotFoundIfThereIsNoTeamWithTheGivenName(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("PUT", "/teams/abc/me@me.me?:team=abc&:user=me@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Team not found$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnUnauthorizedIfTheGivenUserIsNotInTheGivenTeam(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "hi@me.me", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	request, err := http.NewRequest("PUT", "/teams/tsuruteam/hi@me.me?:team=tsuruteam&:user=hi@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, u)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusUnauthorized)
	c.Assert(e, gocheck.ErrorMatches, "^You are not authorized to add new users to the team tsuruteam$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnNotFoundIfTheEmailInTheBodyDoesNotExistInTheDatabase(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("PUT", "/teams/tsuruteam/hi2@me.me?:team=tsuruteam&:user=hi2@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^User not found$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnConflictIfTheUserIsAlreadyInTheGroup(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, s.user.Email, s.team.Name, s.user.Email)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
}

func (s *AuthSuite) TestAddUserToTeamShoulGrantAccessInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "marathon@rush.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	a := App{Name: "i-should", Teams: []string{s.team.Name}}
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	err = addKeyToUser("my-key", u)
	c.Assert(err, gocheck.IsNil)
	err = addUserToTeam(u.Email, s.team.Name, s.user)
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

func (s *AuthSuite) TestAddUserToTeamInGandalfShouldCallGandalfApi(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "nonee@me.me", Password: "none"}
	err := addUserToTeamInGandalf("me@gmail.com", &u, s.team)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(h.url), gocheck.Equals, 1)
	c.Assert(h.url[0], gocheck.Equals, "/repository/grant")
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldRemoveAUserFromATeamIfTheTeamExistAndTheUserIsMemberOfTheTeam(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "nonee@me.me", Password: "none"}
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
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, gocheck.IsNil)
	err = conn.Teams().Find(bson.M{"_id": s.team.Name}).One(s.team)
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.team, gocheck.Not(ContainsUser), &u)
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldRemoveOnlyAppsInThatTeamInGandalfWhenUserIsInMoreThanOneTeam(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "nobody@me.me", Password: "none"}
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
	app2 := app.App{Name: "app2", Teams: []string{team2.Name}}
	err = conn.Apps().Insert(&app2)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": app2.Name})
	err = removeUserFromTeam(u.Email, s.team.Name, s.user)
	c.Assert(err, gocheck.IsNil)
	expected := `{"repositories":["app1"],"users":["nobody@me.me"]}`
	c.Assert(len(h.body), gocheck.Equals, 1)
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
	conn.Teams().FindId(s.team.Name).One(s.team)
	c.Assert(s.team, gocheck.Not(ContainsUser), &u) // just in case
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnNotFoundIfTheTeamDoesNotExist(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/none@me.me?:team=unknown&:user=none@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(e, gocheck.ErrorMatches, "^Team not found$")
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnUnauthorizedIfTheGivenUserIsNotMemberOfTheTeam(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/none@me.me?:team=tsuruteam&:user=none@me.me", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, &auth.User{Email: "unknown@gmail.com"})
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusUnauthorized)
	c.Assert(e, gocheck.ErrorMatches, "^You are not authorized to remove a member from the team tsuruteam")
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnNotFoundWhenTheUserIsNotMemberOfTheTeam(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
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
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnForbiddenIfTheUserIsTheLastInTheTeam(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	url := "/teams/tsuruteam/whydidifall@thewho.com?:team=tsuruteam&:user=whydidifall@thewho.com"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(e, gocheck.ErrorMatches, "^You can not remove this user from this team, because it is the last user within the team, and a team can not be orphaned$")
}

func (s *AuthSuite) TestRemoveUserFromTeamRevokesAccessInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "pomar@nando-reis.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	err = addKeyToUser("my-key", u)
	c.Assert(err, gocheck.IsNil)
	err = addUserToTeam("pomar@nando-reis.com", s.team.Name, s.user)
	c.Assert(err, gocheck.IsNil)
	a := struct {
		Name  string
		Teams []string
	}{Name: "myApp", Teams: []string{s.team.Name}}
	err = conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer conn.Apps().Remove(bson.M{"name": a.Name})
	err = removeUserFromTeam("pomar@nando-reis.com", s.team.Name, s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url[2], gocheck.Equals, "/repository/revoke")
	c.Assert(h.method[2], gocheck.Equals, "DELETE")
	expected := `{"repositories":["myApp"],"users":["pomar@nando-reis.com"]}`
	c.Assert(string(h.body[2]), gocheck.Equals, expected)
}

func (s *AuthSuite) TestRemoveUserFromTeamInDatabase(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "nobody@gmail.com", Password: "123456"}
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
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "nobody@gmail.com"}
	err := removeUserFromTeamInGandalf(u, "someteam")
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(h.url), gocheck.Equals, 1)
	c.Assert(h.url[0], gocheck.Equals, "/repository/revoke")
}

func (s *AuthSuite) TestAddKeyToUserAddsAKeyToTheUser(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
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
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, gocheck.IsNil)
	u2, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2, HasKey, "my-key")
}

func (s *AuthSuite) TestAddKeyToUserReturnsErrorIfTheReadingOfTheBodyFails(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := s.getTestData("bodyToBeClosed.txt")
	b.Close()
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
}

func (s *AuthSuite) TestAddKeyToUserReturnsBadRequestIfTheJsonIsInvalid(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`"aaaa}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Invalid JSON$")
}

func (s *AuthSuite) TestAddKeyToUserReturnsBadRequestIfTheKeyIsNotPresent(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Missing key$")
}

func (s *AuthSuite) TestAddKeyToUserReturnsBadRequestIfTheKeyIsEmpty(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"key":""}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Missing key$")
}

func (s *AuthSuite) TestAddKeyToUserReturnsConflictIfTheKeyIsAlreadyPresent(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
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
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
	c.Assert(e.Message, gocheck.Equals, "User already has this key")
}

func (s *AuthSuite) TestAddKeyAddKeyToUserInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "francisco@franciscosouza.net", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	err = addKeyToUser("my-key", u)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		removeKeyFromUser("my-key", u)
		conn.Users().RemoveAll(bson.M{"email": u.Email})
	}()
	c.Assert(u.Keys[0].Name, gocheck.Not(gocheck.Matches), "\\.pub$")
	expectedUrl := fmt.Sprintf("/user/%s/key", u.Email)
	c.Assert(h.url[0], gocheck.Equals, expectedUrl)
	c.Assert(h.method[0], gocheck.Equals, "POST")
	expected := fmt.Sprintf(`{"%s-1":"my-key"}`, u.Email)
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
}

func (s *AuthSuite) TestAddKeyToUserShouldNotInsertKeyInDatabaseWhenGandalfAdditionFails(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	err = addKeyToUser("my-key", u)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to add key to git server: Failed to connect to Gandalf server, it's probably down.")
	defer conn.Users().RemoveAll(bson.M{"email": u.Email})
	u2, err := auth.GetUserByEmail(u.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2.Keys, gocheck.DeepEquals, []auth.Key{})
}

func (s *AuthSuite) TestAddKeyInDatabaseShouldStoreUsersKeyInDB(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	key := auth.Key{Content: "my-ssh-key", Name: "key1"}
	err = addKeyInDatabase(&key, u)
	c.Assert(err, gocheck.IsNil)
	u2, err := auth.GetUserByEmail(u.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2.Keys, gocheck.DeepEquals, []auth.Key{key})
}

func (s *AuthSuite) TestAddKeyInGandalfShouldCallGandalfApi(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	key := auth.Key{Content: "my-ssh-key", Name: "key1"}
	err = addKeyInGandalf(&key, u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(h.url), gocheck.Equals, 1)
	c.Assert(h.url[0], gocheck.Equals, "/user/me@gmail.com/key")
}

func (s *AuthSuite) TestRemoveKeyFromGandalfCallsGandalfApi(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	key := auth.Key{Name: "mykey", Content: "my-ssh-key"}
	err = addKeyInGandalf(&key, u)
	c.Assert(err, gocheck.IsNil)
	err = removeKeyFromGandalf(&key, u)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(h.url), gocheck.Equals, 2) // add and remove
	expected := fmt.Sprintf("/user/me@gmail.com/key/%s", key.Name)
	c.Assert(h.url[1], gocheck.Matches, expected)
}

func (s *AuthSuite) TestRemoveKeyFromDatabase(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@gmail.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	key := auth.Key{Name: "mykey", Content: "my-ssh-key"}
	err = addKeyInDatabase(&key, u)
	c.Assert(err, gocheck.IsNil)
	err = removeKeyFromDatabase(&key, u)
	c.Assert(err, gocheck.IsNil)
	u2, err := auth.GetUserByEmail(u.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2.Keys, gocheck.DeepEquals, []auth.Key{})
}

func (s *AuthSuite) TestRemoveKeyHandlerRemovesTheKeyFromTheUser(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	addKeyToUser("my-key", s.user)
	defer func() {
		if s.user.HasKey(auth.Key{Content: "my-key"}) {
			removeKeyFromUser("my-key", s.user)
		}
	}()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, gocheck.IsNil)
	u2, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2, gocheck.Not(HasKey), "my-key")
}

func (s *AuthSuite) TestRemoveKeyHandlerCallsGandalfRemoveKey(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	err := addKeyToUser("my-key", s.user) //fills the first position in h properties
	c.Assert(err, gocheck.IsNil)
	defer func() {
		if s.user.HasKey(auth.Key{Content: "my-key"}) {
			removeKeyFromUser("my-key", s.user)
		}
	}()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, gocheck.IsNil)
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
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheJSONIsInvalid(c *gocheck.C) {
	b := bytes.NewBufferString(`invalid"json}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Invalid JSON$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheKeyIsNotPresent(c *gocheck.C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Missing key$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheKeyIsEmpty(c *gocheck.C) {
	b := bytes.NewBufferString(`{"key":""}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e, gocheck.ErrorMatches, "^Missing key$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsNotFoundIfTheUserDoesNotHaveTheKey(c *gocheck.C) {
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestRemoveUser(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "her-voices@painofsalvation.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUser(recorder, request, &u)
	c.Assert(err, gocheck.IsNil)
	n, err := conn.Users().Find(bson.M{"email": u.Email}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
}

func (s *AuthSuite) TestRemoveUserWithTheUserBeingLastMemberOfATeam(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	t := auth.Team{Name: "painofsalvation", Users: []string{u.Email}}
	err = conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": t.Name})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUser(recorder, request, &u)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
	expected := `This user is the last member of the team "painofsalvation", so it cannot be removed.

Please remove the team, them remove the user.`
	c.Assert(e.Message, gocheck.Equals, expected)
}

func (s *AuthSuite) TestRemoveUserShouldRemoveTheUserFromAllTeamsThatHeIsMember(c *gocheck.C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	t := auth.Team{Name: "painofsalvation", Users: []string{u.Email, s.user.Email}}
	err = conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": t.Name})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUser(recorder, request, &u)
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
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
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
	err = RemoveUser(recorder, request, &u)
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
	u.Create()
	oldPassword := u.Password
	defer conn.Users().Remove(bson.M{"email": u.Email})
	body := bytes.NewBufferString(`{"old":"123456","new":"654321"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = ChangePassword(recorder, request, u)
	c.Assert(err, gocheck.IsNil)
	otherUser, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, gocheck.IsNil)
	c.Assert(otherUser.Password, gocheck.Not(gocheck.Equals), oldPassword)
}

func (s *AuthSuite) TestChangePasswordReturns412IfNewPasswordIsInvalid(c *gocheck.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := &auth.User{Email: "me@globo.com.com", Password: "123456"}
	u.Create()
	defer conn.Users().Remove(bson.M{"email": u.Email})
	body := bytes.NewBufferString(`{"old":"123456","new":"1234"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = ChangePassword(recorder, request, u)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Check(e.Code, gocheck.Equals, http.StatusPreconditionFailed)
	c.Check(e.Message, gocheck.Equals, "Password length should be least 6 characters and at most 50 characters.")
}

func (s *AuthSuite) TestChangePasswordReturns404IfOldPasswordDidntMatch(c *gocheck.C) {
	body := bytes.NewBufferString(`{"old":"1234","new":"123456"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = ChangePassword(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Check(e.Code, gocheck.Equals, http.StatusForbidden)
	c.Check(e.Message, gocheck.Equals, "The given password didn't match the user's current password.")
}

func (s *AuthSuite) TestChangePasswordReturns400IfRequestBodyIsInvalidJSON(c *gocheck.C) {
	body := bytes.NewBufferString(`{"invalid:"json`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = ChangePassword(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
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
		err = ChangePassword(recorder, request, s.user)
		c.Assert(err, gocheck.NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, gocheck.Equals, true)
		c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
		c.Assert(e.Message, gocheck.Equals, "Both the old and the new passwords are required.")
	}
}

func (s *AuthSuite) TestGenerateApplicationToken(c *gocheck.C) {
	body := bytes.NewBufferString(`{"client":"tsuru-healer"}`)
	request, _ := http.NewRequest("POST", "/tokens", body)
	recorder := httptest.NewRecorder()
	err := generateAppToken(recorder, request, s.user)
	c.Assert(err, gocheck.IsNil)
	var jsonToken map[string]string
	err = json.NewDecoder(recorder.Body).Decode(&jsonToken)
	c.Assert(err, gocheck.IsNil)
	conn, _ := db.Conn()
	defer conn.Close()
	defer conn.Tokens().Remove(bson.M{"token": jsonToken["token"]})
	_, err = auth.CheckApplicationToken(jsonToken["token"])
	c.Assert(err, gocheck.IsNil)
}

func (s *AuthSuite) TestGenerateApplicationTokenInvalidJSON(c *gocheck.C) {
	body := bytes.NewBufferString(`{"client":"tsuru-`)
	request, _ := http.NewRequest("POST", "/tokens", body)
	recorder := httptest.NewRecorder()
	err := generateAppToken(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
}

func (s *AuthSuite) TestGenerateApplicationTokenMissingClient(c *gocheck.C) {
	body := bytes.NewBufferString(`{"client":""}`)
	request, _ := http.NewRequest("POST", "/tokens", body)
	recorder := httptest.NewRecorder()
	err := generateAppToken(recorder, request, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusBadRequest)
	c.Assert(e.Message, gocheck.Equals, "Missing client name in JSON body")
}
