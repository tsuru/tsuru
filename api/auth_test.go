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
	"sort"
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
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/rec/rectest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/tsurutest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type AuthSuite struct {
	team   *auth.Team
	team2  *auth.Team
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
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *AuthSuite) SetUpTest(c *check.C) {
	routertest.FakeRouter.Reset()
	repositorytest.Reset()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	s.createUserAndTeam(c)
	conn.Platforms().Insert(app.Platform{Name: "python"})
	opts := provision.AddPoolOptions{Name: "test1", Default: true}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
}

func (s *AuthSuite) createUserAndTeam(c *check.C) {
	s.token = customUserWithPermission(c, "super-auth-toremove", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	var err error
	s.user, err = s.token.User()
	s.team = &auth.Team{Name: "tsuruteam"}
	s.team2 = &auth.Team{Name: "tsuruteam2"}
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	err = conn.Teams().Insert(s.team2)
	c.Assert(err, check.IsNil)
}

func (s *AuthSuite) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}

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
	team := auth.Team{Name: "painofsalvation"}
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

func (s *AuthSuite) TestRemoveTeamAsAdmin(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	team := auth.Team{Name: "thegathering"}
	err := conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": team.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s", team.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
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
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermTeamDelete,
		Context: permission.Context(permission.CtxTeam, "other-team"),
	})
	conn, _ := db.Conn()
	defer conn.Close()
	team := auth.Team{Name: "painofsalvation"}
	err := conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": team.Name})
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, `Team "painofsalvation" not found.`)
}

func (s *AuthSuite) TestRemoveTeamGives403WhenTeamHasAccessToAnyApp(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	team := auth.Team{Name: "evergrey"}
	err := conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": team.Name})
	a := app.App{Name: "i-should", Platform: "python", TeamOwner: team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer app.Delete(&a, nil)
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	expected := `This team cannot be removed because there are still references to it:
Apps: i-should`
	c.Assert(e.Message, check.Equals, expected)
}

func (s *AuthSuite) TestRemoveTeamGives403WhenTeamHasAccessToAnyServiceInstance(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	team := auth.Team{Name: "evergrey"}
	err := conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer conn.Teams().Remove(bson.M{"_id": team.Name})
	si1 := service.ServiceInstance{Name: "my_nosql", ServiceName: "nosql-service", Teams: []string{team.Name}}
	err = si1.Create()
	c.Assert(err, check.IsNil)
	si2 := service.ServiceInstance{Name: "my_nosql-2", ServiceName: "nosql-service", Teams: []string{team.Name}}
	err = si2.Create()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", fmt.Sprintf("/teams/%s?:name=%s", team.Name, team.Name), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeTeam(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
	expected := `This team cannot be removed because there are still references to it:
Service instances: my_nosql, my_nosql-2`
	c.Assert(e.Message, check.Equals, expected)
}

func (s *AuthSuite) TestListTeamsListsAllTeamsThatTheUserHasAccess(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	request, err := http.NewRequest("GET", "/teams", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	mux := RunServer(true)
	mux.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var m []map[string]interface{}
	err = json.Unmarshal(b, &m)
	c.Assert(err, check.IsNil)
	c.Assert(m, check.HasLen, 1)
	c.Assert(m[0]["name"], check.Equals, s.team.Name)
	c.Assert(m[0]["permissions"], check.DeepEquals, []interface{}{
		"app.create",
	})
	action := rectest.Action{
		Action: "list-teams",
		User:   token.GetUserName(),
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestListTeamsListsShowOnlyParents(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermApp,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	request, err := http.NewRequest("GET", "/teams", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	mux := RunServer(true)
	mux.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var m []map[string]interface{}
	err = json.Unmarshal(b, &m)
	c.Assert(err, check.IsNil)
	c.Assert(m, check.HasLen, 1)
	c.Assert(m[0]["name"], check.Equals, s.team.Name)
	c.Assert(m[0]["permissions"], check.DeepEquals, []interface{}{
		"app",
	})
	action := rectest.Action{
		Action: "list-teams",
		User:   token.GetUserName(),
	}
	c.Assert(action, rectest.IsRecorded)
}

func (s *AuthSuite) TestListTeamsWithAllPoweredUser(c *check.C) {
	request, err := http.NewRequest("GET", "/teams", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	mux := RunServer(true)
	mux.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	b, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, check.IsNil)
	var m []map[string]interface{}
	err = json.Unmarshal(b, &m)
	c.Assert(err, check.IsNil)
	c.Assert(m, check.HasLen, 2)
	names := []string{m[0]["name"].(string), m[1]["name"].(string)}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{s.team.Name, s.team2.Name})
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
	s.user.AddKey(repository.Key{Name: "the-key", Body: "my-key"}, false)
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

func (s *AuthSuite) TestAddKeyForcingUpdate(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	s.user.AddKey(repository.Key{Name: "the-key", Body: "my-key"}, false)
	conn.Users().Update(bson.M{"email": s.user.Email}, s.user)
	defer func() {
		s.user.RemoveKey(repository.Key{Name: "the-key", Body: "my-key"})
		conn.Users().Update(bson.M{"email": s.user.Email}, s.user)
	}()
	b := bytes.NewBufferString(`{"name":"the-key","key":"my-other-key","force":true}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = addKeyToUser(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	keys, err := repository.Manager().(repository.KeyRepositoryManager).ListKeys(s.user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.DeepEquals, []repository.Key{{Name: "the-key", Body: "my-other-key"}})
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
	users := repositorytest.Users()
	sort.Strings(users)
	c.Assert(users, check.DeepEquals, []string{s.user.Email})
}

func (s *AuthSuite) TestRemoveUserProvidingOwnEmail(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "her-voices@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("DELETE", "/users?user="+u.Email, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, check.IsNil)
	n, err := conn.Users().Find(bson.M{"email": u.Email}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	action := rectest.Action{Action: "remove-user", User: u.Email}
	c.Assert(action, rectest.IsRecorded)
	users := repositorytest.Users()
	sort.Strings(users)
	c.Assert(users, check.DeepEquals, []string{s.user.Email})
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
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	n, err := conn.Users().Find(bson.M{"email": u.Email}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	action := rectest.Action{Action: "remove-user", User: u.Email}
	c.Assert(action, rectest.IsRecorded)
	users := repositorytest.Users()
	sort.Strings(users)
	c.Assert(users, check.DeepEquals, []string{s.user.Email})
}

func (s *AuthSuite) TestRemoveAnotherUserNoPermission(c *check.C) {
	token := userWithPermission(c)
	u := auth.User{Email: "her-voices@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/users?user="+s.user.Email, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
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
	err = tsurutest.WaitCondition(time.Second, func() bool {
		s.server.RLock()
		defer s.server.RUnlock()
		return len(s.server.MailBox) == 1
	})
	c.Assert(err, check.IsNil)
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
func (t TestScheme) AppLogout(token string) error {
	return nil
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

func (s *AuthSuite) TestRegenerateAPITokenHandlerOtherUserAndIsAdminUser(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "user@example.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token := s.token
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("POST", "/users/api-key?user=user@example.com", nil)
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

func (s *AuthSuite) TestRegenerateAPITokenHandlerOtherUserAndNotAdminUser(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	u := auth.User{Email: "user@example.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("POST", "/users/api-key?user=myadmin@arrakis.com", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = regenerateAPIToken(recorder, request, token)
	c.Assert(err, check.NotNil)
	c.Assert(err.(*errors.HTTP).Code, check.Equals, http.StatusForbidden)
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

func (s *AuthSuite) TestShowAPITokenOtherUserAndIsAdminUser(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	user := auth.User{
		Email:    "user@example.com",
		Password: "123456",
		APIKey:   "334hd23ubd923hd923j9d23ndibdf",
	}
	_, err := nativeScheme.Create(&user)
	c.Assert(err, check.IsNil)
	defer conn.Users().Remove(bson.M{"email": user.Email})
	token := s.token
	defer conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	request, err := http.NewRequest("GET", "/users/api-key?user=user@example.com", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = showAPIToken(recorder, request, token)
	c.Assert(err, check.IsNil)
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.Equals, "334hd23ubd923hd923j9d23ndibdf")
}

func (s *AuthSuite) TestShowAPITokenOtherUserWithoutPermission(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	users := []auth.User{
		{
			Email:    "user1@example.com",
			Password: "123456",
			APIKey:   "238hd23ubd923hd923j9d23ndibde",
		},
		{
			Email:    "user2@example.com",
			Password: "123456",
			APIKey:   "334hd23ubd923hd923j9d23ndibdf",
		},
	}
	for _, u := range users {
		_, err := nativeScheme.Create(&u)
		c.Assert(err, check.IsNil)
		defer conn.Users().Remove(bson.M{"email": u.Email})
	}
	token := userWithPermission(c)
	request, err := http.NewRequest("GET", "/users/api-key?user=user@example.com", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = showAPIToken(recorder, request, token)
	c.Assert(err, check.NotNil)
	c.Assert(err.(*errors.HTTP).Code, check.Equals, http.StatusForbidden)
}

func (s *AuthSuite) TestListUsers(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	request, err := http.NewRequest("GET", "/users", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(len(users), check.Equals, 2)
	emails := []string{users[0].Email, users[1].Email}
	expected := []string{token.GetUserName(), s.token.GetUserName()}
	sort.Strings(emails)
	sort.Strings(expected)
	c.Assert(emails, check.DeepEquals, expected)
}

func (s *AuthSuite) TestListUsersLimitedUser(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	request, err := http.NewRequest("GET", "/users", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(len(users), check.Equals, 1)
	c.Assert(users[0].Email, check.Equals, token.GetUserName())
}

func (s *AuthSuite) TestUserInfo(c *check.C) {
	request, err := http.NewRequest("GET", "/users/info", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	expected := apiUser{
		Email: s.user.Email,
		Roles: []rolePermissionData{
			{
				Name:         "super-auth-toremove",
				ContextType:  "global",
				ContextValue: "",
			},
		},
		Permissions: []rolePermissionData{
			{
				Name:         "",
				ContextType:  "global",
				ContextValue: "",
			},
		},
	}
	var got apiUser
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
}

type rolePermList []rolePermissionData

func (l rolePermList) Len() int      { return len(l) }
func (l rolePermList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l rolePermList) Less(i, j int) bool {
	return l[i].Name+l[i].ContextValue < l[j].Name+l[j].ContextValue
}

func (s *AuthSuite) TestUserInfoWithRoles(c *check.C) {
	conn, _ := db.Conn()
	defer conn.Close()
	token := userWithPermission(c)
	r, err := permission.NewRole("myrole", "team")
	c.Assert(err, check.IsNil)
	err = r.AddPermissions("app.create", "app.deploy")
	c.Assert(err, check.IsNil)
	u, err := token.User()
	c.Assert(err, check.IsNil)
	err = u.AddRole("myrole", "a")
	c.Assert(err, check.IsNil)
	err = u.AddRole("myrole", "b")
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/users/info", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	handler := RunServer(true)
	handler.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	expected := apiUser{
		Email: u.Email,
		Roles: []rolePermissionData{
			{Name: "myrole", ContextType: "team", ContextValue: "a"},
			{Name: "myrole", ContextType: "team", ContextValue: "b"},
		},
		Permissions: []rolePermissionData{
			{Name: "app.create", ContextType: "team", ContextValue: "a"},
			{Name: "app.create", ContextType: "team", ContextValue: "b"},
			{Name: "app.deploy", ContextType: "team", ContextValue: "a"},
			{Name: "app.deploy", ContextType: "team", ContextValue: "b"},
		},
	}
	var got apiUser
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	sort.Sort(rolePermList(got.Permissions))
	sort.Sort(rolePermList(got.Roles))
	c.Assert(got, check.DeepEquals, expected)
}

func (s *AuthSuite) BenchmarkListUsersManyUsers(c *check.C) {
	c.StopTimer()
	perm := permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}
	expectedNames := []string{}
	nUsers := 100
	for i := 0; i < nUsers; i++ {
		email := fmt.Sprintf("user-%d", i)
		expectedNames = append(expectedNames, email+"@groundcontrol.com")
		t := customUserWithPermission(c, email, perm)
		u, err := t.User()
		c.Assert(err, check.IsNil)
		err = u.AddRole(u.Roles[0].Name, "someothervalue")
		c.Assert(err, check.IsNil)
	}
	request, err := http.NewRequest("GET", "/users", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	c.StartTimer()
	for i := 0; i < c.N; i++ {
		m.ServeHTTP(recorder, request)
	}
	c.StopTimer()
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(len(users), check.Equals, nUsers+1)
	expectedNames = append(expectedNames, s.user.Email)
	names := []string{}
	for _, u := range users {
		names = append(names, u.Email)
	}
	sort.Strings(names)
	sort.Strings(expectedNames)
	c.Assert(names, check.DeepEquals, expectedNames)
}
