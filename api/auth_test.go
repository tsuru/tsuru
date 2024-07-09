// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/authtest"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"github.com/tsuru/tsuru/tsurutest"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

type AuthSuite struct {
	team            *authTypes.Team
	team2           *authTypes.Team
	user            *auth.User
	token           auth.Token
	server          *authtest.SMTPServer
	testServer      http.Handler
	conn            *db.Storage
	mockTeamService *authTypes.MockTeamService
}

var _ = check.Suite(&AuthSuite{})

func (s *AuthSuite) SetUpSuite(c *check.C) {
	var err error
	config.Set("log:disable-syslog", true)
	config.Set("auth:user-registration", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_api_auth_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("docker:router", "fake")
	config.Set("routers:fake:type", "fake")
	s.server, err = authtest.NewSMTPServer()
	c.Assert(err, check.IsNil)
	config.Set("smtp:server", s.server.Addr())
	config.Set("smtp:user", "root")
	provision.DefaultProvisioner = "fake"
	app.AuthScheme = nativeScheme
	s.testServer = RunServer(true)
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *AuthSuite) TearDownSuite(c *check.C) {
	defer s.conn.Close()
	s.server.Stop()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func (s *AuthSuite) SetUpTest(c *check.C) {
	s.mockTeamService = &authTypes.MockTeamService{}
	servicemanager.Team = s.mockTeamService
	var err error
	servicemanager.AuthGroup, err = auth.GroupService()
	c.Assert(err, check.IsNil)
	provisiontest.ProvisionerInstance.Reset()
	routertest.FakeRouter.Reset()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.createUser(c)
	s.team = &authTypes.Team{Name: "tsuruteam"}
	s.team2 = &authTypes.Team{Name: "tsuruteam2"}
	opts := pool.AddPoolOptions{Name: "test1", Default: true}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
}

func (s *AuthSuite) createUser(c *check.C) {
	_, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "super-auth-toremove", permission.Permission{
		Scheme:  permission.PermAll,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	var err error
	s.user, err = auth.ConvertNewUser(s.token.User())
	c.Assert(err, check.IsNil)
}

func (s *AuthSuite) TestCreateUser(c *check.C) {
	b := strings.NewReader("email=nobody@globo.com&password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	user, err := auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, check.IsNil)
	c.Assert(eventtest.EventDesc{
		Target: userTarget("nobody@globo.com"),
		Owner:  "nobody@globo.com",
		Kind:   "user.create",
	}, eventtest.HasEvent)
	c.Assert(user.Quota, check.DeepEquals, quota.UnlimitedQuota)
}

func (s *AuthSuite) TestCreateUserQuota(c *check.C) {
	config.Set("quota:apps-per-user", 1)
	defer config.Unset("quota:apps-per-user")
	b := strings.NewReader("email=nobody@globo.com&password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	user, err := auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota, check.DeepEquals, quota.Quota{Limit: 1, InUse: 0})
}

func (s *AuthSuite) TestCreateUserUnlimitedQuota(c *check.C) {
	b := strings.NewReader("email=nobody@globo.com&password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	user, err := auth.GetUserByEmail("nobody@globo.com")
	c.Assert(err, check.IsNil)
	c.Assert(user.Quota, check.DeepEquals, quota.UnlimitedQuota)
}

func (s *AuthSuite) TestCreateUserEmailAlreadyExists(c *check.C) {
	u := auth.User{Email: "nobody@globo.com"}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	b := strings.NewReader("email=nobody@globo.com&password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Matches, "this email is already registered\n")
}

func (s *AuthSuite) TestCreateUserEmailIsNotValid(c *check.C) {
	b := strings.NewReader("email=nobody&password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "invalid email\n")
}

func (s *AuthSuite) TestCreateUserPasswordHasLessThan6CharactersOrMoreThan50Characters(c *check.C) {
	passwords := []string{"123", strings.Join(make([]string, 52), "-")}
	for _, password := range passwords {
		b := strings.NewReader("email=nobody@noboy.com&password=" + password)
		request, err := http.NewRequest(http.MethodPost, "/users", b)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		s.testServer.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
		errMsg := "password length should be least 6 characters and at most 50 characters\n"
		c.Assert(recorder.Body.String(), check.Equals, errMsg)
	}
}

func (s *AuthSuite) TestCreateUserFailWithRegistrationDisabled(c *check.C) {
	b := strings.NewReader("email=nobody@globo.com&password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	oldUserRegistration, err := config.GetBool("auth:user-registration")
	c.Assert(err, check.IsNil)
	config.Set("auth:user-registration", false)
	defer config.Set("auth:user-registration", oldUserRegistration)
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), check.Equals, createDisabledErr.Error()+"\n")
}

func (s *AuthSuite) TestCreateUserFailWithRegistrationDisabledAndCommonUser(c *check.C) {
	simpleUser := &auth.User{Email: "my@common.user", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), simpleUser)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": simpleUser.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	b := strings.NewReader("email=nobody@globo.com&password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	oldUserRegistration, err := config.GetBool("auth:user-registration")
	c.Assert(err, check.IsNil)
	config.Set("auth:user-registration", false)
	defer config.Set("auth:user-registration", oldUserRegistration)
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), check.Equals, createDisabledErr.Error()+"\n")
}

func (s *AuthSuite) TestCreateUserWorksWithRegistrationDisabledAndAdminUser(c *check.C) {
	b := strings.NewReader("email=nobody@globo.com&password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	oldUserRegistration, err := config.GetBool("auth:user-registration")
	c.Assert(err, check.IsNil)
	config.Set("auth:user-registration", false)
	defer config.Set("auth:user-registration", oldUserRegistration)
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
}

func (s *AuthSuite) TestLoginShouldCreateTokenInTheDatabaseAndReturnItWithinTheResponse(c *check.C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users/nobody@globo.com/tokens", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var user auth.User
	err = s.conn.Users().Find(bson.M{"email": "nobody@globo.com"}).One(&user)
	c.Assert(err, check.IsNil)
	var recorderJSON map[string]string
	json.Unmarshal(recorder.Body.Bytes(), &recorderJSON)
	n, err := s.conn.Tokens().Find(bson.M{"token": recorderJSON["token"]}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
}

func (s *AuthSuite) TestLoginPasswordMissing(c *check.C) {
	b := strings.NewReader("")
	request, err := http.NewRequest(http.MethodPost, "/users/nobody@globo.com/tokens", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Matches, "^you must provide a password to login\n$")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *AuthSuite) TestLoginUserDoesNotExist(c *check.C) {
	b := strings.NewReader("password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users/nobody@globo.com/tokens", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Matches, "^user not found\n$")
}

func (s *AuthSuite) TestLoginPasswordDoesNotMatch(c *check.C) {
	u := auth.User{Email: "nobody@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("password=1234567")
	request, err := http.NewRequest(http.MethodPost, "/users/nobody@globo.com/tokens", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), check.Matches, "^Authentication failed, wrong password.\n$")
}

func (s *AuthSuite) TestLoginEmailIsNotValid(c *check.C) {
	b := strings.NewReader("password=123456")
	request, err := http.NewRequest(http.MethodPost, "/users/nobody/tokens", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, native.ErrInvalidEmail.Error()+"\n")
}

func (s *AuthSuite) TestLoginPasswordIsInvalid(c *check.C) {
	passwords := []string{"123", strings.Join(make([]string, 52), "-")}
	u := &auth.User{Email: "me@globo.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), u)
	c.Assert(err, check.IsNil)
	for _, password := range passwords {
		b := strings.NewReader("password=" + password)
		request, err := http.NewRequest(http.MethodPost, "/users/me@globo.com/tokens", b)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		s.testServer.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), check.Matches, "Password.*\n")
	}
}

func (s *AuthSuite) TestLogout(c *check.C) {
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": s.user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodDelete, "/users/tokens", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = logout(recorder, request, token)
	c.Assert(err, check.IsNil)
	_, err = nativeScheme.Auth(context.TODO(), token.GetValue())
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *AuthSuite) TestCreateTeam(c *check.C) {
	teamName := "teamredbull"
	s.mockTeamService.OnCreate = func(teamName string, tags []string, _ *authTypes.User) error {
		c.Assert(teamName, check.Equals, teamName)
		c.Assert(tags, check.DeepEquals, []string{"tag1", "tag2"})
		return nil
	}
	b := strings.NewReader("name=" + teamName + "&tag=tag1&tag=tag2")
	request, err := http.NewRequest(http.MethodPost, "/teams", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated, check.Commentf("body: %v", recorder.Body.String()))
	c.Assert(eventtest.EventDesc{
		Target: teamTarget(teamName),
		Owner:  s.token.GetUserName(),
		Kind:   "team.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": teamName},
			{"name": "tag", "value": []string{"tag1", "tag2"}},
		},
	}, eventtest.HasEvent)
}

func (s *AuthSuite) TestCreateTeamInvalidTeamName(c *check.C) {
	s.mockTeamService.OnCreate = func(_ string, _ []string, _ *authTypes.User) error {
		return authTypes.ErrInvalidTeamName
	}
	b := strings.NewReader("ble=bla")
	request, err := http.NewRequest(http.MethodPost, "/teams", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, authTypes.ErrInvalidTeamName.Error()+"\n")
}

func (s *AuthSuite) TestCreateTeamAlreadyExists(c *check.C) {
	s.mockTeamService.OnCreate = func(_ string, _ []string, _ *authTypes.User) error {
		return authTypes.ErrTeamAlreadyExists
	}
	teamName := "timeredbull"
	b := strings.NewReader("name=" + teamName)
	request, err := http.NewRequest(http.MethodPost, "/teams", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, "team already exists\n")
}

func (s *AuthSuite) TestRemoveTeam(c *check.C) {
	teamName := "painofsalvation"
	s.mockTeamService.OnRemove = func(teamName string) error {
		c.Assert(teamName, check.Equals, teamName)
		return nil
	}
	request, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/teams/%s?:name=%s", teamName, teamName), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: teamTarget(teamName),
		Owner:  s.token.GetUserName(),
		Kind:   "team.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": teamName},
		},
	}, eventtest.HasEvent)
}

func (s *AuthSuite) TestRemoveTeamGives404WhenTeamDoesNotExist(c *check.C) {
	s.mockTeamService.OnRemove = func(_ string) error {
		return authTypes.ErrTeamNotFound
	}
	request, err := http.NewRequest(http.MethodDelete, "/teams/unknown?:name=unknown", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Team \"unknown\" not found.\n")
}

func (s *AuthSuite) TestRemoveTeamGives404WhenUserDoesNotHaveAccessToTheTeam(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermTeamDelete,
		Context: permission.Context(permTypes.CtxTeam, "other-team"),
	})
	teamName := "painofsalvation"
	request, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/teams/%s?:name=%s", teamName, teamName), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Team \"painofsalvation\" not found.\n")
}

func (s *AuthSuite) TestRemoveTeamGives403WhenTeamHasAccessToAnyApp(c *check.C) {
	s.mockTeamService.OnRemove = func(_ string) error {
		return &authTypes.ErrTeamStillUsed{Apps: []string{"i-should"}}
	}
	teamName := "evergrey"
	request, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/teams/%s?:name=%s", teamName, teamName), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	expected := `This team cannot be removed because there are still references to it:
Apps: i-should
`
	c.Assert(recorder.Body.String(), check.Equals, expected)
}

func (s *AuthSuite) TestRemoveTeamGives403WhenTeamHasAccessToAnyServiceInstance(c *check.C) {
	s.mockTeamService.OnRemove = func(_ string) error {
		return &authTypes.ErrTeamStillUsed{ServiceInstances: []string{"my_nosql", "my_nosql-2"}}
	}
	teamName := "evergrey"
	request, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/teams/%s?:name=%s", teamName, teamName), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	expected := `This team cannot be removed because there are still references to it:
Service instances: my_nosql, my_nosql-2
`
	c.Assert(recorder.Body.String(), check.Equals, expected)
}

func (s *AuthSuite) TestListTeamsListsAllTeamsThatTheUserHasAccess(c *check.C) {
	s.mockTeamService.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request, err := http.NewRequest(http.MethodGet, "/teams", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var m []map[string]interface{}
	err = json.Unmarshal(recorder.Body.Bytes(), &m)
	c.Assert(err, check.IsNil)
	c.Assert(m, check.HasLen, 1)
	c.Assert(m[0]["name"], check.Equals, s.team.Name)
	c.Assert(m[0]["permissions"], check.DeepEquals, []interface{}{
		"app.create",
	})
}

func (s *AuthSuite) TestListTeamsListsShowOnlyParents(c *check.C) {
	s.mockTeamService.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermApp,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request, err := http.NewRequest(http.MethodGet, "/teams", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var m []map[string]interface{}
	err = json.Unmarshal(recorder.Body.Bytes(), &m)
	c.Assert(err, check.IsNil)
	c.Assert(m, check.HasLen, 1)
	c.Assert(m[0]["name"], check.Equals, s.team.Name)
	c.Assert(m[0]["permissions"], check.DeepEquals, []interface{}{
		"app",
	})
}

func (s *AuthSuite) TestListTeamsWithAllPoweredUser(c *check.C) {
	s.mockTeamService.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}, {Name: s.team2.Name}}, nil
	}
	request, err := http.NewRequest(http.MethodGet, "/teams", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var m []map[string]interface{}
	err = json.Unmarshal(recorder.Body.Bytes(), &m)
	c.Assert(err, check.IsNil)
	c.Assert(m, check.HasLen, 2)
	names := []string{m[0]["name"].(string), m[1]["name"].(string)}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{s.team.Name, s.team2.Name})
}

func (s *AuthSuite) TestListTeamsReturns204IfTheUserHasNoTeam(c *check.C) {
	s.mockTeamService.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}, {Name: s.team2.Name}}, nil
	}
	u := auth.User{Email: "cruiser@gotthard.com", Password: "234567"}
	_, err := nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "234567"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodGet, "/teams", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *AuthSuite) TestTeamInfoReturns404TeamNotFound(c *check.C) {
	teamName := "team-test"
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, teamName)
		return nil, authTypes.ErrTeamNotFound
	}
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/teams/%v", teamName), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *AuthSuite) TestTeamInfoReturns200Success(c *check.C) {
	teamName := "team-test"
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, teamName)
		return &authTypes.Team{Name: name}, nil
	}
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/teams/%v", teamName), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *AuthSuite) TestTeamInfoReturnsUsers(c *check.C) {
	teamName := "team-test"
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, teamName)
		return &authTypes.Team{Name: name}, nil
	}

	u1 := auth.User{Email: "myuser1@example.com", Roles: []authTypes.RoleInstance{{Name: "team-member", ContextValue: teamName}}}
	err := u1.Create(context.TODO())
	c.Assert(err, check.IsNil)

	u2 := auth.User{Email: "myuser2@example.com", Roles: []authTypes.RoleInstance{{Name: "god"}, {Name: "team-member", ContextValue: "other-team"}}}
	err = u2.Create(context.TODO())
	c.Assert(err, check.IsNil)

	role, err := permission.NewRole("team-member", "team", "")
	c.Assert(err, check.IsNil)

	err = role.AddPermissions(context.TODO(), "app")
	c.Assert(err, check.IsNil)

	role, err = permission.NewRole("god", "global", "")
	c.Assert(err, check.IsNil)

	err = role.AddPermissions(context.TODO(), "app")
	c.Assert(err, check.IsNil)

	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/teams/%v", teamName), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	type result struct {
		Users []apiUser
	}
	r := &result{}
	json.Unmarshal(recorder.Body.Bytes(), r)
	c.Assert(r.Users, check.HasLen, 1)
	c.Assert(u1.Email, check.Equals, r.Users[0].Email)
}

func (s *AuthSuite) TestRemoveUser(c *check.C) {
	u := auth.User{Email: "her-voices@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodDelete, "/users", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, check.IsNil)
	n, err := s.conn.Users().Find(bson.M{"email": u.Email}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: userTarget(token.GetUserName()),
		Owner:  token.GetUserName(),
		Kind:   "user.delete",
	}, eventtest.HasEvent)
}

func (s *AuthSuite) TestRemoveUserProvidingOwnEmail(c *check.C) {
	u := auth.User{Email: "her-voices@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodDelete, "/users?user="+u.Email, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, check.IsNil)
	n, err := s.conn.Users().Find(bson.M{"email": u.Email}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: userTarget(u.Email),
		Owner:  token.GetUserName(),
		Kind:   "user.delete",
		StartCustomData: []map[string]interface{}{
			{"name": "user", "value": u.Email},
		},
	}, eventtest.HasEvent)

}

func (s *AuthSuite) TestRemoveAnotherUser(c *check.C) {
	u := auth.User{Email: "her-voices@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodDelete, "/users?user="+u.Email, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	n, err := s.conn.Users().Find(bson.M{"email": u.Email}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: userTarget(u.Email),
		Owner:  s.token.GetUserName(),
		Kind:   "user.delete",
		StartCustomData: []map[string]interface{}{
			{"name": "user", "value": u.Email},
		},
	}, eventtest.HasEvent)
}

func (s *AuthSuite) TestRemoveAnotherUserNoPermission(c *check.C) {
	token := userWithPermission(c)
	u := auth.User{Email: "her-voices@painofsalvation.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodDelete, "/users?user="+s.user.Email, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUser(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *AuthSuite) TestChangePassword(c *check.C) {
	u := &auth.User{Email: "me@globo.com.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), u)
	c.Assert(err, check.IsNil)
	oldPassword := u.Password
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("old=123456&new=654321&confirm=654321")
	request, err := http.NewRequest(http.MethodPut, "/users/password", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	otherUser, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(otherUser.Password, check.Not(check.Equals), oldPassword)
	c.Assert(eventtest.EventDesc{
		Target: userTarget(token.GetUserName()),
		Owner:  token.GetUserName(),
		Kind:   "user.update.password",
	}, eventtest.HasEvent)
}

func (s *AuthSuite) TestChangePasswordReturns412IfNewPasswordIsInvalid(c *check.C) {
	u := &auth.User{Email: "me@globo.com.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("old=123456&new=1234&confirm=1234")
	request, err := http.NewRequest(http.MethodPut, "/users/password", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	msg := "password length should be least 6 characters and at most 50 characters\n"
	c.Check(recorder.Body.String(), check.Equals, msg)
}

func (s *AuthSuite) TestChangePasswordReturns412IfNewPasswordAndConfirmPasswordDidntMatch(c *check.C) {
	u := &auth.User{Email: "me@globo.com.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("old=123456&new=12345678&confirm=1234567810")
	request, err := http.NewRequest(http.MethodPut, "/users/password", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	msg := "New password and password confirmation didn't match.\n"
	c.Check(recorder.Body.String(), check.Equals, msg)
}

func (s *AuthSuite) TestChangePasswordReturns404IfOldPasswordDidntMatch(c *check.C) {
	body := strings.NewReader("old=1234&new=123456&confirm=123456")
	request, err := http.NewRequest(http.MethodPut, "/users/password", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	msg := "the given password didn't match the user's current password\n"
	c.Assert(recorder.Body.String(), check.Equals, msg)
}

func (s *AuthSuite) TestChangePasswordInvalidPasswords(c *check.C) {
	bodies := []string{"old=something", "new=something", "{}", "null"}
	for _, body := range bodies {
		b := strings.NewReader(body)
		request, err := http.NewRequest(http.MethodPut, "/users/password", b)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "bearer "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		s.testServer.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), check.Equals, "Both the old and the new passwords are required.\n")
	}
}

func (s *AuthSuite) TestResetPasswordStep1(c *check.C) {
	defer s.server.Reset()
	oldPassword := s.user.Password
	url := fmt.Sprintf("/users/%s/password?:email=%s", s.user.Email, s.user.Email)
	request, _ := http.NewRequest(http.MethodPost, url, nil)
	recorder := httptest.NewRecorder()
	err := resetPassword(recorder, request)
	c.Assert(err, check.IsNil)
	var m map[string]interface{}
	err = s.conn.PasswordTokens().Find(bson.M{"useremail": s.user.Email}).One(&m)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(time.Second, func() bool {
		s.server.RLock()
		defer s.server.RUnlock()
		return len(s.server.MailBox) == 1
	})
	c.Assert(err, check.IsNil)
	u, err := auth.GetUserByEmail(s.user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(u.Password, check.Equals, oldPassword)
	c.Assert(eventtest.EventDesc{
		Target: userTarget(s.token.GetUserName()),
		Owner:  s.token.GetUserName(),
		Kind:   "user.update.reset",
		StartCustomData: []map[string]interface{}{
			{"name": ":email", "value": s.token.GetUserName()},
		},
	}, eventtest.HasEvent)
}

func (s *AuthSuite) TestResetPasswordUserNotFound(c *check.C) {
	url := "/users/unknown@tsuru.io/password?:email=unknown@tsuru.io"
	request, _ := http.NewRequest(http.MethodPost, url, nil)
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
	request, _ := http.NewRequest(http.MethodPost, url, nil)
	recorder := httptest.NewRecorder()
	err := resetPassword(recorder, request)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Message, check.Equals, "invalid email")
}

func (s *AuthSuite) TestResetPasswordStep2(c *check.C) {
	user := auth.User{Email: "uns@alanis.com", Password: "145678"}
	err := user.Create(context.TODO())
	c.Assert(err, check.IsNil)
	oldPassword := user.Password
	err = nativeScheme.StartPasswordReset(context.TODO(), &user)
	c.Assert(err, check.IsNil)
	var t map[string]interface{}
	err = s.conn.PasswordTokens().Find(bson.M{"useremail": user.Email}).One(&t)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/users/%s/password?:email=%s&token=%s", user.Email, user.Email, t["_id"])
	request, _ := http.NewRequest(http.MethodPost, url, nil)
	recorder := httptest.NewRecorder()
	err = resetPassword(recorder, request)
	c.Assert(err, check.IsNil)
	u2, err := auth.GetUserByEmail(user.Email)
	c.Assert(err, check.IsNil)
	c.Assert(u2.Password, check.Not(check.Equals), oldPassword)
	c.Assert(eventtest.EventDesc{
		Target: userTarget(user.Email),
		Owner:  user.Email,
		Kind:   "user.update.reset",
		StartCustomData: []map[string]interface{}{
			{"name": ":email", "value": user.Email},
			{"name": "token", "value": t["_id"]},
		},
	}, eventtest.HasEvent)
}

type TestScheme native.NativeScheme

var (
	_ auth.Scheme = &TestScheme{}
)

func (t TestScheme) AppLogin(ctx context.Context, appName string) (auth.Token, error) {
	return nil, nil
}
func (t TestScheme) AppLogout(ctx context.Context, token string) error {
	return nil
}
func (t TestScheme) Login(ctx context.Context, params map[string]string) (auth.Token, error) {
	return nil, nil
}
func (t TestScheme) Logout(ctx context.Context, token string) error {
	return nil
}
func (t TestScheme) Auth(ctx context.Context, token string) (auth.Token, error) {
	return nil, nil
}
func (t TestScheme) Info(ctx context.Context) (*authTypes.SchemeInfo, error) {
	return &authTypes.SchemeInfo{Name: "test", Data: authTypes.SchemeData{AuthorizeURL: "http://foo/bar"}}, nil
}
func (t TestScheme) Create(ctx context.Context, u *auth.User) (*auth.User, error) {
	return nil, nil
}
func (t TestScheme) Remove(ctx context.Context, u *auth.User) error {
	return nil
}

func (s *AuthSuite) TestAuthScheme(c *check.C) {
	oldScheme := app.AuthScheme
	defer func() { app.AuthScheme = oldScheme }()
	app.AuthScheme = TestScheme{}
	request, err := http.NewRequest(http.MethodGet, "/auth/scheme", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var parsed map[string]interface{}
	err = json.NewDecoder(recorder.Body).Decode(&parsed)
	c.Assert(err, check.IsNil)
	c.Assert(parsed["name"], check.Equals, "test")
	c.Assert(parsed["data"], check.DeepEquals, map[string]interface{}{"authorizeUrl": "http://foo/bar"})
}

func (s *AuthSuite) TestRegenerateAPITokenHandler(c *check.C) {
	r, err := permission.NewRole("myrole", "global", "")
	c.Assert(err, check.IsNil)
	err = r.AddPermissions(context.TODO(), "apikey.update")
	c.Assert(err, check.IsNil)

	u := auth.User{Email: "zobomafoo@zimbabue.com", Password: "123456", Roles: []authTypes.RoleInstance{
		{Name: r.Name},
	}}
	_, err = nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodPost, "/users/api-key", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	count, err := s.conn.Users().Find(bson.M{"apikey": got}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
	c.Assert(eventtest.EventDesc{
		Target: userTarget(u.Email),
		Owner:  u.Email,
		Kind:   "apikey.update",
	}, eventtest.HasEvent)
}

func (s *AuthSuite) TestRegenerateAPITokenHandlerOtherUserAndIsAdminUser(c *check.C) {
	u := auth.User{Email: "leto@arrakis.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	token := s.token
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodPost, "/users/api-key?user=leto@arrakis.com", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = regenerateAPIToken(recorder, request, token)
	c.Assert(err, check.IsNil)
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	count, err := s.conn.Users().Find(bson.M{"apikey": got}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
	c.Assert(eventtest.EventDesc{
		Target: userTarget("leto@arrakis.com"),
		Owner:  token.GetUserName(),
		Kind:   "apikey.update",
		StartCustomData: []map[string]interface{}{
			{"name": "user", "value": "leto@arrakis.com"},
		},
	}, eventtest.HasEvent)
}

func (s *AuthSuite) TestRegenerateAPITokenHandlerOtherUserAndNotAdminUser(c *check.C) {
	u := auth.User{Email: "user@example.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodPost, "/users/api-key?user=myadmin@arrakis.com", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = regenerateAPIToken(recorder, request, token)
	c.Assert(err, check.NotNil)
	c.Assert(err.(*errors.HTTP).Code, check.Equals, http.StatusForbidden)
}

func (s *AuthSuite) TestShowAPITokenForUserWithNoPermission(c *check.C) {
	u := auth.User{Email: "zobomafoo@zimbabue.com", Password: "123456"}
	_, err := nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodGet, "/users/api-key", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = showAPIToken(recorder, request, token)
	c.Assert(err, check.Equals, permission.ErrUnauthorized)
}

func (s *AuthSuite) TestShowAPITokenForUserWithNoToken(c *check.C) {
	r, err := permission.NewRole("myrole", "global", "")
	c.Assert(err, check.IsNil)
	err = r.AddPermissions(context.TODO(), "apikey.read")
	c.Assert(err, check.IsNil)

	u := auth.User{Email: "zobomafoo@zimbabue.com", Password: "123456", Roles: []authTypes.RoleInstance{
		{Name: r.Name},
	}}
	_, err = nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodGet, "/users/api-key", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = showAPIToken(recorder, request, token)
	c.Assert(err, check.IsNil)
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	count, err := s.conn.Users().Find(bson.M{"apikey": got}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *AuthSuite) TestShowAPITokenForUserWithToken(c *check.C) {
	r, err := permission.NewRole("myrole", "global", "")
	c.Assert(err, check.IsNil)
	err = r.AddPermissions(context.TODO(), "apikey.read")
	c.Assert(err, check.IsNil)

	u := auth.User{Email: "zobomafoo@zimbabue.com", Password: "123456", APIKey: "238hd23ubd923hd923j9d23ndibde", Roles: []authTypes.RoleInstance{
		{Name: r.Name},
	}}

	_, err = nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodGet, "/users/api-key", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var got string
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.Equals, "238hd23ubd923hd923j9d23ndibde")
}

func (s *AuthSuite) TestShowAPITokenOtherUserAndIsAdminUser(c *check.C) {
	user := auth.User{
		Email:    "user@example.com",
		Password: "123456",
		APIKey:   "334hd23ubd923hd923j9d23ndibdf",
	}
	_, err := nativeScheme.Create(context.TODO(), &user)
	c.Assert(err, check.IsNil)
	token := s.token
	request, err := http.NewRequest(http.MethodGet, "/users/api-key?user=user@example.com", nil)
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
		_, err := nativeScheme.Create(context.TODO(), &u)
		c.Assert(err, check.IsNil)
	}
	token := userWithPermission(c)
	request, err := http.NewRequest(http.MethodGet, "/users/api-key?user=user@example.com", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = showAPIToken(recorder, request, token)
	c.Assert(err, check.NotNil)
	c.Assert(err.(*errors.HTTP).Code, check.Equals, http.StatusForbidden)
}

func (s *AuthSuite) TestListUsers(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request, err := http.NewRequest(http.MethodGet, "/users", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 2)
	emails := []string{users[0].Email, users[1].Email}
	expected := []string{token.GetUserName(), s.token.GetUserName()}
	sort.Strings(emails)
	sort.Strings(expected)
	c.Assert(emails, check.DeepEquals, expected)
}

func (s *AuthSuite) TestListUsersFilterByUserEmail(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	expected := token.GetUserName()
	url := fmt.Sprintf("/users?userEmail=%s", expected)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
	email := users[0].Email
	c.Check(email, check.DeepEquals, expected)
}

func (s *AuthSuite) TestListUsersFilterByRole(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	expectedUser, err := token.User()
	c.Assert(err, check.IsNil)
	userRoles := expectedUser.Roles
	expectedRole := userRoles[0].Name
	url := fmt.Sprintf("/users?role=%s", expectedRole)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
	receivedUser := users[0]
	c.Assert(expectedUser.Email, check.Equals, receivedUser.Email)
}

func (s *AuthSuite) TestListUsersFilterByRoleAndContext(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team2.Name),
	})
	expectedUser, err := token.User()
	c.Assert(err, check.IsNil)
	userRoles := expectedUser.Roles
	expectedRole := userRoles[1].Name
	otherUser := &auth.User{Email: "groundcontrol@majortom.com", Password: "123456", Quota: quota.UnlimitedQuota}
	_, err = nativeScheme.Create(context.TODO(), otherUser)
	c.Assert(err, check.IsNil)
	otherUser.AddRole(context.TODO(), expectedRole, s.team.Name)
	url := fmt.Sprintf("/users?role=%s&context=%s", expectedRole, s.team2.Name)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
	receivedUser := users[0]
	c.Assert(expectedUser.Email, check.Equals, receivedUser.Email)
}

func (s *AuthSuite) TestListUsersFilterByRoleAndInvalidContext(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team2.Name),
	})
	expectedUser, err := token.User()
	c.Assert(err, check.IsNil)
	userRoles := expectedUser.Roles
	expectedRole := userRoles[1].Name
	otherUser := &auth.User{Email: "groundcontrol@majortom.com", Password: "123456", Quota: quota.UnlimitedQuota}
	_, err = nativeScheme.Create(context.TODO(), otherUser)
	c.Assert(err, check.IsNil)
	otherUser.AddRole(context.TODO(), expectedRole, s.team.Name)
	url := fmt.Sprintf("/users?role=%s&context=%s", expectedRole, "blablabla")
	request, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	var users []apiUser
	c.Assert(recorder.Body.String(), check.Equals, "Wrong context being passed.\n")
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.NotNil)
	c.Assert(users, check.HasLen, 0)
}

func (s *AuthSuite) TestListUsersLimitedUser(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request, err := http.NewRequest(http.MethodGet, "/users", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
	c.Assert(users[0].Email, check.Equals, token.GetUserName())
}

func (s *AuthSuite) TestListUsersLimitedUserWithMoreRoles(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	_, token2 := permissiontest.CustomUserWithPermission(c, nativeScheme, "jerry", permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "another-team"),
	})
	request, err := http.NewRequest(http.MethodGet, "/users", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 2)
	emails := []string{users[0].Email, users[1].Email}
	expected := []string{token.GetUserName(), token2.GetUserName()}
	sort.Strings(emails)
	sort.Strings(expected)
	c.Assert(emails, check.DeepEquals, expected)
}

func (s *AuthSuite) TestUserInfo(c *check.C) {
	request, err := http.NewRequest(http.MethodGet, "/users/info", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
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

func (s *AuthSuite) TestUserInfoWithoutRoles(c *check.C) {
	token := userWithPermission(c)
	u, err := token.User()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodGet, "/users/info", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	expected := apiUser{
		Email: u.Email,
		Roles: []rolePermissionData{},
	}
	var got apiUser
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	sort.Sort(rolePermList(got.Permissions))
	sort.Sort(rolePermList(got.Roles))
	c.Assert(got, check.DeepEquals, expected)
}

type rolePermList []rolePermissionData

func (l rolePermList) Len() int      { return len(l) }
func (l rolePermList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l rolePermList) Less(i, j int) bool {
	return l[i].Name+l[i].ContextValue < l[j].Name+l[j].ContextValue
}

func (s *AuthSuite) TestUserInfoWithRoles(c *check.C) {
	token := userWithPermission(c)
	r, err := permission.NewRole("myrole", "team", "")
	c.Assert(err, check.IsNil)
	err = r.AddPermissions(context.TODO(), "app.create", "app.deploy")
	c.Assert(err, check.IsNil)
	u, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "myrole", "a")
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "myrole", "b")
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodGet, "/users/info", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
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

func (s *AuthSuite) TestUserInfoWithRolesFromGroups(c *check.C) {
	token := userWithPermission(c)
	r, err := permission.NewRole("myrole", "team", "")
	c.Assert(err, check.IsNil)
	err = r.AddPermissions(context.TODO(), "app.create", "app.deploy")
	c.Assert(err, check.IsNil)
	u, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	err = u.AddRole(context.TODO(), "myrole", "a")
	c.Assert(err, check.IsNil)
	u.Groups = []string{"grp1", "grp2"}
	err = u.Update()
	c.Assert(err, check.IsNil)
	err = servicemanager.AuthGroup.AddRole(context.TODO(), "grp2", "myrole", "b")
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodGet, "/users/info", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	expected := apiUser{
		Email: u.Email,
		Roles: []rolePermissionData{
			{Name: "myrole", ContextType: "team", ContextValue: "a"},
			{Name: "myrole", ContextType: "team", ContextValue: "b", Group: "grp2"},
		},
		Permissions: []rolePermissionData{
			{Name: "app.create", ContextType: "team", ContextValue: "a"},
			{Name: "app.create", ContextType: "team", ContextValue: "b", Group: "grp2"},
			{Name: "app.deploy", ContextType: "team", ContextValue: "a"},
			{Name: "app.deploy", ContextType: "team", ContextValue: "b", Group: "grp2"},
		},
		Groups: []string{"grp1", "grp2"},
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
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}
	expectedNames := []string{}
	nUsers := 100
	for i := 0; i < nUsers; i++ {
		email := fmt.Sprintf("user-%d", i)
		expectedNames = append(expectedNames, email+"@groundcontrol.com")
		_, t := permissiontest.CustomUserWithPermission(c, nativeScheme, email, perm)
		u, err := auth.ConvertNewUser(t.User())
		c.Assert(err, check.IsNil)
		err = u.AddRole(context.TODO(), u.Roles[0].Name, "someothervalue")
		c.Assert(err, check.IsNil)
	}
	request, err := http.NewRequest(http.MethodGet, "/users", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	c.StartTimer()
	for i := 0; i < c.N; i++ {
		s.testServer.ServeHTTP(recorder, request)
	}
	c.StopTimer()
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, nUsers+1)
	expectedNames = append(expectedNames, s.user.Email)
	names := []string{}
	for _, u := range users {
		names = append(names, u.Email)
	}
	sort.Strings(names)
	sort.Strings(expectedNames)
	c.Assert(names, check.DeepEquals, expectedNames)
}

func (s *AuthSuite) TestUserListWithoutPermission(c *check.C) {
	perm1 := permission.Permission{
		Scheme:  permission.PermRoleUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}
	perm2 := permission.Permission{
		Scheme:  permission.PermTeamCreate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	}
	token := userWithPermission(c, perm1, perm2)
	request, err := http.NewRequest(http.MethodGet, "/users", nil)
	c.Assert(err, check.IsNil)
	request.Header.Add("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var users []apiUser
	err = json.NewDecoder(recorder.Body).Decode(&users)
	c.Assert(err, check.IsNil)
	c.Assert(users, check.HasLen, 1)
	emails := []string{users[0].Email}
	expected := []string{token.GetUserName()}
	c.Assert(emails, check.DeepEquals, expected)
}

func (s *AuthSuite) TestUpdateTeam(c *check.C) {
	oldTeamName := "team1"
	newTeamName := "team9000"
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, oldTeamName)
		return &authTypes.Team{Name: name}, nil
	}
	s.mockTeamService.OnCreate = func(name string, _ []string, _ *authTypes.User) error {
		c.Assert(name, check.Equals, newTeamName)
		return nil
	}
	s.mockTeamService.OnRemove = func(name string) error {
		c.Assert(name, check.Equals, oldTeamName)
		return nil
	}
	body := strings.NewReader("newname=" + newTeamName)
	request, err := http.NewRequest(http.MethodPost, "/teams/"+oldTeamName, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *AuthSuite) TestUpdateTeamNotFound(c *check.C) {
	s.mockTeamService.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return nil, authTypes.ErrTeamNotFound
	}
	body := strings.NewReader("newname=team9000")
	request, err := http.NewRequest(http.MethodPost, "/teams/team1", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, authTypes.ErrTeamNotFound.Error()+"\n")
}

func (s *AuthSuite) TestUpdateTeamTags(c *check.C) {
	var updated bool
	s.mockTeamService.OnUpdate = func(name string, tags []string) error {
		c.Assert(name, check.DeepEquals, "team1")
		c.Assert(tags, check.DeepEquals, []string{"tag1", "tag2"})
		updated = true
		return nil
	}
	body := strings.NewReader("tag=tag2&tags.0=tag1")
	request, err := http.NewRequest(http.MethodPut, "/teams/team1", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(updated, check.DeepEquals, true)
}

func (s *AuthSuite) TestUpdateTeamCallFnsAndRollback(c *check.C) {
	s.mockTeamService.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &authTypes.Team{}, nil
	}
	oldTeamRenameFns := teamRenameFns
	defer func() { teamRenameFns = oldTeamRenameFns }()
	var calls1, calls2 [][]string
	teamRenameFns = []func(ctx context.Context, oldName, newName string) error{
		func(ctx context.Context, oldName, newName string) error {
			calls1 = append(calls1, []string{oldName, newName})
			return nil
		},
		func(ctx context.Context, oldName, newName string) error {
			calls2 = append(calls2, []string{oldName, newName})
			return fmt.Errorf("error in %q -> %q", oldName, newName)
		},
	}
	body := strings.NewReader("newname=team9000")
	request, err := http.NewRequest(http.MethodPost, "/teams/team1", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "error in \"team1\" -> \"team9000\"\n")
	c.Assert(calls1, check.DeepEquals, [][]string{
		{"team1", "team9000"},
		{"team9000", "team1"},
	})
	c.Assert(calls2, check.DeepEquals, [][]string{
		{"team1", "team9000"},
	})
}

func (s *AuthSuite) TestUpdateTeamErrorInRollback(c *check.C) {
	s.mockTeamService.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &authTypes.Team{}, nil
	}
	oldTeamRenameFns := teamRenameFns
	defer func() { teamRenameFns = oldTeamRenameFns }()
	var calls1, calls2 [][]string
	teamRenameFns = []func(ctx context.Context, oldName, newName string) error{
		func(ctx context.Context, oldName, newName string) error {
			calls1 = append(calls1, []string{oldName, newName})
			if len(calls1) == 2 {
				return fmt.Errorf("error in rollback")
			}
			return nil
		},
		func(ctx context.Context, oldName, newName string) error {
			calls2 = append(calls2, []string{oldName, newName})
			return fmt.Errorf("error in %q -> %q", oldName, newName)
		},
	}
	body := strings.NewReader("newname=team9000")
	request, err := http.NewRequest(http.MethodPost, "/teams/team1", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	buf := bytes.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(buf, true))
	defer log.SetLogger(nil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "error in \"team1\" -> \"team9000\"\n")
	c.Assert(calls1, check.DeepEquals, [][]string{
		{"team1", "team9000"},
		{"team9000", "team1"},
	})
	c.Assert(calls2, check.DeepEquals, [][]string{
		{"team1", "team9000"},
	})
	c.Assert(buf.String(), check.Matches, "(?s).*error rolling back team name change in.*TestUpdateTeamErrorInRollback.*from \"team1\" to \"team9000\".*")
}

func (s *AuthSuite) TestTeamUsersList(c *check.C) {
	teamName := "team-test"
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, teamName)
		return &authTypes.Team{Name: name}, nil
	}

	u1 := auth.User{Email: "myuser1@example.com", Roles: []authTypes.RoleInstance{{Name: "team-member", ContextValue: teamName}}}
	err := u1.Create(context.TODO())
	c.Assert(err, check.IsNil)

	u2 := auth.User{Email: "myuser2@example.com", Roles: []authTypes.RoleInstance{{Name: "god"}, {Name: "team-member", ContextValue: "other-team"}}}
	err = u2.Create(context.TODO())
	c.Assert(err, check.IsNil)

	role1, err := permission.NewRole("team-member", "team", "")
	c.Assert(err, check.IsNil)

	err = role1.AddPermissions(context.TODO(), "app")
	c.Assert(err, check.IsNil)

	role2, err := permission.NewRole("god", "global", "")
	c.Assert(err, check.IsNil)

	err = role2.AddPermissions(context.TODO(), "app")
	c.Assert(err, check.IsNil)

	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/teams/%v/users", teamName), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	r := []teamUserItem{}
	json.NewDecoder(recorder.Body).Decode(&r)
	c.Assert(r, check.HasLen, 1)
	c.Assert(r[0].Email, check.Equals, u1.Email)
	c.Assert(r[0].Roles, check.DeepEquals, []string{role1.Name})
}

func (s *AuthSuite) TestTeamUsersListNoPerm(c *check.C) {
	teamName := "team-test"
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, teamName)
		return &authTypes.Team{Name: name}, nil
	}

	token := userWithPermission(c)

	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/teams/%s/users?:name=%s", teamName, teamName), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = teamUserList(recorder, request, token)
	c.Assert(err, check.Equals, permission.ErrUnauthorized)
}

func (s *AuthSuite) TestTeamUsersListNoTeam(c *check.C) {
	teamName := "team-test"
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		return nil, authTypes.ErrTeamNotFound
	}

	token := userWithPermission(c)

	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/teams/%s/users?:name=%s", teamName, teamName), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = teamUserList(recorder, request, token)
	c.Assert(err, check.DeepEquals, &errors.HTTP{Code: http.StatusNotFound, Message: "team not found"})
}

func (s *AuthSuite) TestTeamGroupsList(c *check.C) {
	teamName := "team-test"
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, teamName)
		return &authTypes.Team{Name: name}, nil
	}

	role1, err := permission.NewRole("team-member", "team", "")
	c.Assert(err, check.IsNil)
	err = role1.AddPermissions(context.TODO(), "app")
	c.Assert(err, check.IsNil)

	err = servicemanager.AuthGroup.AddRole(context.TODO(), "group1", "team-member", teamName)
	c.Assert(err, check.IsNil)

	err = servicemanager.AuthGroup.AddRole(context.TODO(), "group2", "team-member", "other-team")
	c.Assert(err, check.IsNil)

	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/teams/%v/groups", teamName), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)

	r := []teamGroupItem{}
	json.NewDecoder(recorder.Body).Decode(&r)
	c.Assert(r, check.HasLen, 1)
	c.Assert(r[0].Group, check.Equals, "group1")
	c.Assert(r[0].Roles, check.DeepEquals, []string{role1.Name})
}

func (s *AuthSuite) TestTeamGroupsListNoPerm(c *check.C) {
	teamName := "team-test"
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, teamName)
		return &authTypes.Team{Name: name}, nil
	}

	token := userWithPermission(c)

	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/teams/%s/groups?:name=%s", teamName, teamName), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = teamGroupList(recorder, request, token)
	c.Assert(err, check.Equals, permission.ErrUnauthorized)
}

func (s *AuthSuite) TestTeamGroupsListNoTeam(c *check.C) {
	teamName := "team-test"
	s.mockTeamService.OnFindByName = func(name string) (*authTypes.Team, error) {
		return nil, authTypes.ErrTeamNotFound
	}

	token := userWithPermission(c)

	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/teams/%s/users?:name=%s", teamName, teamName), nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = teamGroupList(recorder, request, token)
	c.Assert(err, check.DeepEquals, &errors.HTTP{Code: http.StatusNotFound, Message: "team not found"})
}
