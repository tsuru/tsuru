// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
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

var _ = Suite(&AuthSuite{})

func (s *AuthSuite) SetUpSuite(c *C) {
	var err error
	err = config.ReadConfigFile("../etc/tsuru.conf")
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_api_auth_test")
	c.Assert(err, IsNil)
	s.createUserAndTeam(c)
}

func (s *AuthSuite) TearDownSuite(c *C) {
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func (s *AuthSuite) TearDownTest(c *C) {
	_, err := db.Session.Users().RemoveAll(bson.M{"email": bson.M{"$ne": s.user.Email}})
	c.Assert(err, IsNil)
	_, err = db.Session.Teams().RemoveAll(bson.M{"_id": bson.M{"$ne": s.team.Name}})
	c.Assert(err, IsNil)
	s.user.Password = "123"
	s.user.HashPassword()
	err = s.user.Update()
	c.Assert(err, IsNil)
}

func (s *AuthSuite) createUserAndTeam(c *C) {
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123"}
	err := s.user.Create()
	c.Assert(err, IsNil)
	s.team = &auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	err = db.Session.Teams().Insert(s.team)
	c.Assert(err, IsNil)
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

func (c *hasKeyChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "HasKey", Params: []string{"user", "key"}}
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

var HasKey Checker = &hasKeyChecker{}

type userPresenceChecker struct{}

func (c *userPresenceChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "ContainsUser", Params: []string{"team", "user"}}
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

var ContainsUser Checker = &userPresenceChecker{}

func (s *AuthSuite) TestCreateUserHandlerSavesTheUserInTheDatabase(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, IsNil)
	u := auth.User{Email: "nobody@globo.com"}
	err = u.Get()
	c.Assert(err, IsNil)
}

func (s *AuthSuite) TestCreateUserHandlerReturnsStatus201AfterCreateTheUser(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 201)
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorIfReadingBodyFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^.*bad file descriptor$")
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorAndBadRequestIfInvalidJSONIsGiven(c *C) {
	b := bytes.NewBufferString(`["invalid json":"i'm invalid"]`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^invalid character.*$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateUserHandlerReturnErrorAndConflictIfItFailsToCreateUser(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	u.Create()
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "This email is already registered")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
}

func (s *AuthSuite) TestCreateUserHandlerReturnsPreconditionFailedIfEmailIsNotValid(c *C) {
	b := bytes.NewBufferString(`{"email":"nobody","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, Equals, "Invalid email.")
}

func (s *AuthSuite) TestCreateUserHandlerReturnsPreconditionFailedIfPasswordHasLessThan6CharactersOrMoreThan50Characters(c *C) {
	passwords := []string{"123", strings.Join(make([]string, 52), "-")}
	for _, password := range passwords {
		b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"` + password + `"}`)
		request, err := http.NewRequest("POST", "/users", b)
		c.Assert(err, IsNil)
		request.Header.Set("Content-type", "application/json")
		recorder := httptest.NewRecorder()
		err = CreateUser(recorder, request)
		c.Assert(err, NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, Equals, true)
		c.Assert(e.Code, Equals, http.StatusPreconditionFailed)
		c.Assert(e.Message, Equals, "Password length should be least 6 characters and at most 50 characters.")
	}
}

func (s *AuthSuite) TestCreateUserCreatesUserInGandalf(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"email":"nobody@me.myself","password":"123456"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	defer db.Session.Users().Remove(bson.M{"email": "nobody@me.myself"})
	err = CreateUser(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(h.url[0], Equals, "/user")
	expected := `{"name":"nobody@me.myself","keys":{}}`
	c.Assert(string(h.body[0]), Equals, expected)
	c.Assert(h.method[0], Equals, "POST")
}

func (s *AuthSuite) TestLoginShouldCreateTokenInTheDatabaseAndReturnItWithinTheResponse(c *C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	u.Create()
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, IsNil)
	var user auth.User
	collection := db.Session.Users()
	err = collection.Find(bson.M{"email": "nobody@globo.com"}).One(&user)
	var recorderJson map[string]string
	r, _ := ioutil.ReadAll(recorder.Body)
	json.Unmarshal(r, &recorderJson)
	c.Assert(recorderJson["token"], Equals, user.Tokens[0].Token)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndBadRequestIfItReceivesAnInvalidJson(c *C) {
	b := bytes.NewBufferString(`"invalid":"json"]`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Invalid JSON$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndBadRequestIfTheJSONDoesNotContainsAPassword(c *C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^You must provide a password to login$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndNotFoundIfTheUserDoesNotExist(c *C) {
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User not found$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestLoginShouldreturnErrorIfThePasswordDoesNotMatch(c *C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	u.Create()
	b := bytes.NewBufferString(`{"password":"1234567"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Authentication failed, wrong password$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusUnauthorized)
}

func (s *AuthSuite) TestLoginShouldReturnErrorAndInternalServerErrorIfReadAllFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	err := b.Close()
	c.Assert(err, IsNil)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
}

func (s *AuthSuite) TestLoginShouldReturnPreconditionFailedIfEmailIsNotValid(c *C) {
	b := bytes.NewBufferString(`{"password":"123456"}`)
	request, err := http.NewRequest("POST", "/users/nobody/token?:email=nobody", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, Equals, emailError)
}

func (s *AuthSuite) TestLoginShouldReturnPreconditionFailedIfPasswordIsLessesThan6CharactersOrGreaterThan50Characters(c *C) {
	passwords := []string{"123", strings.Join(make([]string, 52), "-")}
	for _, password := range passwords {
		b := bytes.NewBufferString(`{"password":"` + password + `"}`)
		request, err := http.NewRequest("POST", "/users/nobody@globo.com/token?:email=nobody@globo.com", b)
		c.Assert(err, IsNil)
		request.Header.Set("Content-type", "application/json")
		recorder := httptest.NewRecorder()
		err = Login(recorder, request)
		c.Assert(err, NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, Equals, true)
		c.Assert(e.Code, Equals, http.StatusPreconditionFailed)
		c.Assert(e.Message, Equals, passwordError)
	}
}

func (s *AuthSuite) TestCreateTeamHandlerSavesTheTeamInTheDatabaseWithTheAuthenticatedUser(c *C) {
	b := bytes.NewBufferString(`{"name":"timeredbull"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, IsNil)
	t := new(auth.Team)
	err = db.Session.Teams().Find(bson.M{"_id": "timeredbull"}).One(t)
	defer db.Session.Teams().Remove(bson.M{"_id": "timeredbull"})
	c.Assert(err, IsNil)
	c.Assert(t, ContainsUser, s.user)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnsBadRequestIfTheRequestBodyIsAnInvalidJSON(c *C) {
	b := bytes.NewBufferString(`{"name"["invalidjson"]}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnsBadRequestIfTheNameIsNotGiven(c *C) {
	b := bytes.NewBufferString(`{"genre":"male"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^You must provide the team name$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnsInternalServerErrorIfReadAllFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	err := b.Close()
	c.Assert(err, IsNil)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *AuthSuite) TestCreateTeamHandlerReturnConflictIfTheTeamToBeCreatedAlreadyExists(c *C) {
	err := db.Session.Teams().Insert(bson.M{"_id": "timeredbull"})
	defer db.Session.Teams().Remove(bson.M{"_id": "timeredbull"})
	c.Assert(err, IsNil)
	b := bytes.NewBufferString(`{"name":"timeredbull"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
	c.Assert(e, ErrorMatches, "^This team already exists$")
}

func (s *AuthSuite) TestKeyToMap(c *C) {
	keys := []auth.Key{{Name: "testkey", Content: "somekey"}}
	kMap := keyToMap(keys)
	c.Assert(kMap, DeepEquals, map[string]string{"testkey": "somekey"})
}

func (s *AuthSuite) TestRemoveTeam(c *C) {
	team := auth.Team{Name: "painofsalvation", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(team)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": team.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveTeam(recorder, request, s.user)
	c.Assert(err, IsNil)
	n, err := db.Session.Teams().Find(bson.M{"name": team.Name}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 0)
}

func (s *AuthSuite) TestRemoveTeamGives404WhenTeamDoesNotExist(c *C) {
	request, err := http.NewRequest("DELETE", "/teams/unknown?:name=unknown", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e.Message, Equals, `Team "unknown" not found.`)
}

func (s *AuthSuite) TestRemoveTeamGives404WhenUserDoesNotHaveAccessToTheTeam(c *C) {
	team := auth.Team{Name: "painofsalvation"}
	err := db.Session.Teams().Insert(team)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": team.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e.Message, Equals, `Team "painofsalvation" not found.`)
}

func (s *AuthSuite) TestRemoveTeamGives403WhenTeamHasAccessToAnyApp(c *C) {
	team := auth.Team{Name: "evergrey", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(team)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": team.Name})
	a := App{Name: "i-should", Teams: []string{team.Name}}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	expected := `This team cannot be removed because it have access to apps.

Please remove the apps or revoke these accesses, and try again.`
	c.Assert(e.Message, Equals, expected)
}

func (s *AuthSuite) TestListTeamsListsAllTeamsThatTheUserIsMember(c *C) {
	request, err := http.NewRequest("GET", "/teams", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ListTeams(recorder, request, s.user)
	c.Assert(err, IsNil)
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)
	var m []map[string]string
	err = json.Unmarshal(b, &m)
	c.Assert(err, IsNil)
	c.Assert(m, DeepEquals, []map[string]string{{"name": s.team.Name}})
}

func (s *AuthSuite) TestListTeamsReturns204IfTheUserHasNoTeam(c *C) {
	u := auth.User{Email: "cruiser@gotthard.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	request, err := http.NewRequest("GET", "/teams", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ListTeams(recorder, request, &u)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, http.StatusNoContent)
}

func (s *AuthSuite) TestAddUserToTeamShouldAddAUserToATeamIfTheUserAndTheTeamExistAndTheGivenUserIsMemberOfTheTeam(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)
	url := "/teams/tsuruteam/wolverine@xmen.com?:team=tsuruteam&:user=wolverine@xmen.com"
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, IsNil)
	t := new(auth.Team)
	err = db.Session.Teams().Find(bson.M{"_id": "tsuruteam"}).One(t)
	c.Assert(err, IsNil)
	c.Assert(t, ContainsUser, s.user)
	c.Assert(t, ContainsUser, u)
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnNotFoundIfThereIsNoTeamWithTheGivenName(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("PUT", "/teams/abc/me@me.me?:team=abc&:user=me@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Team not found$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnUnauthorizedIfTheGivenUserIsNotInTheGivenTeam(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "hi@me.me", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)
	request, err := http.NewRequest("PUT", "/teams/tsuruteam/hi@me.me?:team=tsuruteam&:user=hi@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, u)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusUnauthorized)
	c.Assert(e, ErrorMatches, "^You are not authorized to add new users to the team tsuruteam$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnNotFoundIfTheEmailInTheBodyDoesNotExistInTheDatabase(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("PUT", "/teams/tsuruteam/hi2@me.me?:team=tsuruteam&:user=hi2@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^User not found$")
}

func (s *AuthSuite) TestAddUserToTeamShouldReturnConflictIfTheUserIsAlreadyInTheGroup(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, s.user.Email, s.team.Name, s.user.Email)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
}

func (s *AuthSuite) TestAddUserToTeamShoulGrantAccessInGandalf(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "marathon@rush.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	a := App{Name: "i-should", Teams: []string{s.team.Name}}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = addKeyToUser("my-key", u)
	c.Assert(err, IsNil)
	err = u.Get()
	c.Assert(err, IsNil)
	err = addUserToTeam(u.Email, s.team.Name, s.user)
	c.Assert(err, IsNil)
	c.Check(len(h.url), Equals, 2)
	c.Assert(h.url[1], Equals, "/repository/grant")
	c.Assert(h.method[1], Equals, "POST")
	expected := fmt.Sprintf(`{"repositories":["%s"],"users":["marathon@rush.com"]}`, a.Name)
	c.Assert(string(h.body[1]), Equals, expected)
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldRemoveAUserFromATeamIfTheTeamExistAndTheUserIsMemberOfTheTeam(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "nonee@me.me", Password: "none"}
	err := u.Create()
	c.Assert(err, IsNil)
	s.team.AddUser(&u)
	db.Session.Teams().Update(bson.M{"_id": s.team.Name}, s.team)
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/nonee@me.me?:team=tsuruteam&:user=nonee@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, IsNil)
	team := new(auth.Team)
	err = db.Session.Teams().Find(bson.M{"_id": s.team.Name}).One(team)
	c.Assert(err, IsNil)
	c.Assert(team, Not(ContainsUser), &u)
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnNotFoundIfTheTeamDoesNotExist(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/none@me.me?:team=unknown&:user=none@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Team not found$")
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnUnauthorizedIfTheGivenUserIsNotMemberOfTheTeam(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/none@me.me?:team=tsuruteam&:user=none@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, &auth.User{Email: "unknown@gmail.com"})
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusUnauthorized)
	c.Assert(e, ErrorMatches, "^You are not authorized to remove a member from the team tsuruteam")
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnNotFoundWhenTheUserIsNotMemberOfTheTeam(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "nobody@me.me", Password: "132"}
	s.team.AddUser(u)
	db.Session.Teams().Update(bson.M{"_id": s.team.Name}, s.team)
	defer func(t *auth.Team, u *auth.User) {
		s.team.RemoveUser(u)
		db.Session.Teams().Update(bson.M{"_id": t.Name}, t)
	}(s.team, u)
	request, err := http.NewRequest("DELETE", "/teams/tsuruteam/none@me.me?:team=tsuruteam&:user=none@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestRemoveUserFromTeamShouldReturnForbiddenIfTheUserIsTheLastInTheTeam(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	url := "/teams/tsuruteam/whydidifall@thewho.com?:team=tsuruteam&:user=whydidifall@thewho.com"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^You can not remove this user from this team, because it is the last user within the team, and a team can not be orphaned$")
}

func (s *AuthSuite) TestRemoveUserFromTeamRevokesAccessInGandalf(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "pomar@nando-reis.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	err = addKeyToUser("my-key", u)
	c.Assert(err, IsNil)
	err = u.Get()
	c.Assert(err, IsNil)
	err = addUserToTeam("pomar@nando-reis.com", s.team.Name, s.user)
	c.Assert(err, IsNil)
	a := struct {
		Name  string
		Teams []string
	}{Name: "myApp", Teams: []string{s.team.Name}}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = removeUserFromTeam("pomar@nando-reis.com", s.team.Name, s.user)
	c.Assert(err, IsNil)
	c.Assert(h.url[2], Equals, "/repository/revoke")
	c.Assert(h.method[2], Equals, "DELETE")
	expected := `{"repositories":["myApp"],"users":["pomar@nando-reis.com"]}`
	c.Assert(string(h.body[2]), Equals, expected)
}

func (s *AuthSuite) TestAddKeyHandlerAddsAKeyToTheUser(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	defer func() {
		s.user.RemoveKey(auth.Key{Content: "my-key"})
		db.Session.Users().Update(bson.M{"email": s.user.Email}, s.user)
	}()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, IsNil)
	s.user.Get()
	c.Assert(s.user, HasKey, "my-key")
}

func (s *AuthSuite) TestAddKeyHandlerReturnsErrorIfTheReadingOfTheBodyFails(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := s.getTestData("bodyToBeClosed.txt")
	b.Close()
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *AuthSuite) TestAddKeyHandlerReturnsBadRequestIfTheJsonIsInvalid(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`"aaaa}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Invalid JSON$")
}

func (s *AuthSuite) TestAddKeyHandlerReturnsBadRequestIfTheKeyIsNotPresent(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Missing key$")
}

func (s *AuthSuite) TestAddKeyHandlerReturnsBadRequestIfTheKeyIsEmpty(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	b := bytes.NewBufferString(`{"key":""}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Missing key$")
}

func (s *AuthSuite) TestAddKeyHandlerReturnsConflictIfTheKeyIsAlreadyPresent(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	s.user.AddKey(auth.Key{Content: "my-key"})
	db.Session.Users().Update(bson.M{"email": s.user.Email}, s.user)
	defer func() {
		s.user.RemoveKey(auth.Key{Content: "my-key"})
		db.Session.Users().Update(bson.M{"email": s.user.Email}, s.user)
	}()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
}

func (s *AuthSuite) TestAddKeyAddKeyToUserInGandalf(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := &auth.User{Email: "francisco@franciscosouza.net", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)
	err = addKeyToUser("my-key", u)
	c.Assert(err, IsNil)
	defer func() {
		removeKeyFromUser("my-key", u)
		db.Session.Users().RemoveAll(bson.M{"email": u.Email})
	}()
	c.Assert(u.Keys[0].Name, Not(Matches), "\\.pub$")
	expectedUrl := fmt.Sprintf("/user/%s/key", u.Email)
	c.Assert(h.url[0], Equals, expectedUrl)
	c.Assert(h.method[0], Equals, "POST")
	expected := fmt.Sprintf(`{"%s-1":"my-key"}`, u.Email)
	c.Assert(string(h.body[0]), Equals, expected)
}

func (s *AuthSuite) TestRemoveKeyHandlerRemovesTheKeyFromTheUser(c *C) {
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
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, IsNil)
	s.user.Get()
	c.Assert(s.user, Not(HasKey), "my-key")
}

func (s *AuthSuite) TestRemoveKeyHandlerCallsGandalfRemoveKey(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	err := addKeyToUser("my-key", s.user) //fills the first position in h properties
	c.Assert(err, IsNil)
	defer func() {
		if s.user.HasKey(auth.Key{Content: "my-key"}) {
			removeKeyFromUser("my-key", s.user)
		}
	}()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, IsNil)
	c.Assert(h.url[1], Equals, fmt.Sprintf("/user/%s/key/%s-%d", s.user.Email, s.user.Email, len(s.user.Keys)+1))
	c.Assert(h.method[1], Equals, "DELETE")
	c.Assert(string(h.body[1]), Equals, "null")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsErrorInCaseOfAnyIOFailure(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	b.Close()
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheJSONIsInvalid(c *C) {
	b := bytes.NewBufferString(`invalid"json}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Invalid JSON$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheKeyIsNotPresent(c *C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Missing key$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsBadRequestIfTheKeyIsEmpty(c *C) {
	b := bytes.NewBufferString(`{"key":""}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Missing key$")
}

func (s *AuthSuite) TestRemoveKeyHandlerReturnsNotFoundIfTheUserDoesNotHaveTheKey(c *C) {
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestRemoveUser(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "her-voices@painofsalvation.com"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUser(recorder, request, &u)
	c.Assert(err, IsNil)
	n, err := db.Session.Users().Find(bson.M{"email": u.Email}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 0)
}

func (s *AuthSuite) TestRemoveUserWithTheUserBeingLastMemberOfATeam(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	t := auth.Team{Name: "painofsalvation", Users: []string{u.Email}}
	err = db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUser(recorder, request, &u)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	expected := `This user is the last member of the team "painofsalvation", so it cannot be removed.

Please remove the team, them remove the user.`
	c.Assert(e.Message, Equals, expected)
}

func (s *AuthSuite) TestRemoveUserShouldRemoveTheUserFromAllTeamsThatHeIsMember(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	t := auth.Team{Name: "painofsalvation", Users: []string{u.Email, s.user.Email}}
	err = db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUser(recorder, request, &u)
	c.Assert(err, IsNil)
	err = db.Session.Teams().Find(bson.M{"_id": t.Name}).One(&t)
	c.Assert(err, IsNil)
	c.Assert(t.Users, HasLen, 1)
	c.Assert(t.Users[0], Equals, s.user.Email)
}

type App struct {
	Name  string
	Teams []string
}

func (s *AuthSuite) TestRemoveUserRevokesAccessInGandalf(c *C) {
	h := testHandler{}
	ts := s.startGandalfTestServer(&h)
	defer ts.Close()
	u := auth.User{Email: "of-two-beginnings@painofsalvation.com"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	t := auth.Team{Name: "painofsalvation", Users: []string{u.Email, s.user.Email}}
	err = db.Session.Teams().Insert(t)
	c.Assert(err, IsNil)
	defer db.Session.Teams().Remove(bson.M{"_id": t.Name})
	a := struct {
		Name  string
		Teams []string
	}{Name: "myApp", Teams: []string{t.Name}}
	err = db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	request, err := http.NewRequest("DELETE", "/users", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUser(recorder, request, &u)
	c.Assert(err, IsNil)
	c.Assert(h.url[0], Equals, "/repository/revoke")
	c.Assert(h.method[0], Equals, "DELETE")
	expected := `{"repositories":["myApp"],"users":["of-two-beginnings@painofsalvation.com"]}`
	c.Assert(string(h.body[0]), Equals, expected)
}

func (s *AuthSuite) TestChangePasswordHandler(c *C) {
	body := bytes.NewBufferString(`{"old":"123","new":"123456"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ChangePassword(recorder, request, s.user)
	c.Assert(err, IsNil)
	otherUser := *s.user
	err = otherUser.Get()
	c.Assert(err, IsNil)
	hashPassword := func(password string) string {
		salt := []byte("tsuru-salt")
		return fmt.Sprintf("%x", pbkdf2.Key([]byte(password), salt, 4096, len(salt)*8, sha512.New))
	}
	expectedPassword := hashPassword("123456")
	c.Assert(otherUser.Password, Equals, expectedPassword)
}

func (s *AuthSuite) TestChangePasswordReturns412IfNewPasswordIsInvalid(c *C) {
	body := bytes.NewBufferString(`{"old":"123","new":"1234"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ChangePassword(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, Equals, "Password length should be least 6 characters and at most 50 characters.")
}

func (s *AuthSuite) TestChangePasswordReturns404IfOldPasswordDidntMatch(c *C) {
	body := bytes.NewBufferString(`{"old":"1234","new":"123456"}`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ChangePassword(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e.Message, Equals, "The given password didn't match the user's current password.")
}

func (s *AuthSuite) TestChangePasswordReturns400IfRequestBodyIsInvalidJSON(c *C) {
	body := bytes.NewBufferString(`{"invalid:"json`)
	request, err := http.NewRequest("PUT", "/users/password", body)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = ChangePassword(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e.Message, Equals, "Invalid JSON.")
}

func (s *AuthSuite) TestChangePasswordReturns400IfJSONDoesNotIncludeBothOldAndNewPasswords(c *C) {
	bodies := []string{`{"old": "something"}`, `{"new":"something"}`, "{}", "null"}
	for _, body := range bodies {
		b := bytes.NewBufferString(body)
		request, err := http.NewRequest("PUT", "/users/password", b)
		c.Assert(err, IsNil)
		recorder := httptest.NewRecorder()
		err = ChangePassword(recorder, request, s.user)
		c.Assert(err, NotNil)
		e, ok := err.(*errors.Http)
		c.Assert(ok, Equals, true)
		c.Assert(e.Code, Equals, http.StatusBadRequest)
		c.Assert(e.Message, Equals, "Both the old and the new passwords are required.")
	}
}

type testApp struct {
	Name  string
	Teams []string
}

func (s *AuthSuite) TestAllowedAppsShouldReturnAllAppsTheUserHasAccess(c *C) {
	team := auth.Team{Name: "teamname", Users: []string{s.user.Email}}
	err := db.Session.Teams().Insert(&team)
	c.Assert(err, IsNil)
	a := testApp{Name: "myapp", Teams: []string{s.team.Name}}
	err = db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	a2 := testApp{Name: "myotherapp", Teams: []string{team.Name}}
	err = db.Session.Apps().Insert(&a2)
	c.Assert(err, IsNil)
	defer func() {
		db.Session.Apps().Remove(bson.M{"name": bson.M{"$in": []string{a.Name, a2.Name}}})
		db.Session.Teams().RemoveId(team.Name)
	}()
	aApps, err := allowedApps(s.user.Email)
	c.Assert(aApps, DeepEquals, []string{a.Name, a2.Name})
}
