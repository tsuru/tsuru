// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"golang.org/x/crypto/bcrypt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/repository/repositorytest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"github.com/tsuru/tsuru/types"
	serviceTypes "github.com/tsuru/tsuru/types/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type HandlerSuite struct {
	conn  *db.Storage
	token auth.Token
}

var _ = check.Suite(&HandlerSuite{})

func (s *HandlerSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_handler_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("repo-manager", "fake")
}

func (s *HandlerSuite) SetUpTest(c *check.C) {
	repositorytest.Reset()
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	team := types.Team{Name: "tsuruteam"}
	err = serviceTypes.Team().Insert(team)
	c.Assert(err, check.IsNil)
	app.AuthScheme = nativeScheme
}

func (s *HandlerSuite) TearDownTest(c *check.C) {
	s.conn.Close()
}

func (s *HandlerSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func errorHandler(w http.ResponseWriter, r *http.Request) error {
	return fmt.Errorf("some error")
}

func errorHandlerWriteHeader(w http.ResponseWriter, r *http.Request) error {
	w.WriteHeader(http.StatusBadGateway)
	return errorHandler(w, r)
}

func badRequestHandler(w http.ResponseWriter, r *http.Request) error {
	return &errors.HTTP{Code: http.StatusBadRequest, Message: "some error"}
}

func simpleHandler(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprint(w, "success")
	return nil
}

func outputHandler(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text")
	output := "2012-06-05 17:03:36,887 WARNING ssl-hostname-verification is disabled for this environment"
	fmt.Fprint(w, output)
	return nil
}

func authorizedErrorHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return errorHandler(w, r)
}

func authorizedErrorHandlerWriteHeader(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return errorHandlerWriteHeader(w, r)
}

func authorizedBadRequestHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return badRequestHandler(w, r)
}

func authorizedSimpleHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return simpleHandler(w, r)
}

func authorizedOutputHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return outputHandler(w, r)
}

type recorder struct {
	*httptest.ResponseRecorder
	headerWrites int
}

func (r *recorder) WriteHeader(code int) {
	r.headerWrites++
	r.ResponseRecorder.WriteHeader(code)
}

func (s *HandlerSuite) TestHandlerReturns500WhenInternalHandlerReturnsAnError(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", "GET", Handler(errorHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "some error\n")
}

func (s *HandlerSuite) TestHandlerDontCallWriteHeaderIfItHasAlreadyBeenCalled(c *check.C) {
	rec := recorder{httptest.NewRecorder(), 0}
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", "GET", Handler(errorHandlerWriteHeader))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(&rec, request)
	c.Assert(rec.Code, check.Equals, http.StatusBadGateway)
	c.Assert(rec.Body.String(), check.Equals, "some error\n")
	c.Assert(rec.headerWrites, check.Equals, 1)
}

func (s *HandlerSuite) TestHandlerShouldPassAnHandlerWithoutError(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", "GET", Handler(simpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "success")
}

func (s *HandlerSuite) TestHandlerShouldSetVersionHeaders(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", "GET", Handler(simpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), check.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), check.Equals, craneMin)
	c.Assert(recorder.Header().Get("Supported-Tsuru-Admin"), check.Equals, tsuruAdminMin)
}

func (s *HandlerSuite) TestHandlerShouldSetVersionHeadersEvenOnFail(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", "GET", Handler(errorHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), check.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), check.Equals, craneMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheAuthorizationHeadIsNotPresent(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", "GET", AuthorizationRequiredHandler(authorizedSimpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Header().Get("WWW-Authenticate"), check.Equals, "Bearer realm=\"tsuru\" scope=\"tsuru\"")
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a valid Authorization header\n")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheTokenIsInvalid(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "what the token?!")
	RegisterHandler("/apps", "GET", AuthorizationRequiredHandler(authorizedSimpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Header().Get("WWW-Authenticate"), check.Equals, "Bearer realm=\"tsuru\" scope=\"tsuru\"")
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a valid Authorization header\n")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnTheHandlerResultIfTheTokenIsOk(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	RegisterHandler("/apps", "GET", AuthorizationRequiredHandler(authorizedSimpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "success")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldSetVersionHeaders(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", s.token.GetValue())
	RegisterHandler("/apps", "GET", AuthorizationRequiredHandler(authorizedSimpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), check.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), check.Equals, craneMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldSetVersionHeadersEvenOnError(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "what the token?!")
	RegisterHandler("/apps", "GET", AuthorizationRequiredHandler(authorizedSimpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), check.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), check.Equals, craneMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnTheHandlerErrorIfAnyHappen(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	RegisterHandler("/apps", "GET", AuthorizationRequiredHandler(authorizedErrorHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "some error\n")
}

func (s *HandlerSuite) TestAuthorizetionRequiredHandlerDontCallWriteHeaderIfItHasAlreadyBeenCalled(c *check.C) {
	rec := recorder{httptest.NewRecorder(), 0}
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	RegisterHandler("/apps", "GET", AuthorizationRequiredHandler(authorizedErrorHandlerWriteHeader))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(&rec, request)
	c.Assert(rec.Code, check.Equals, http.StatusBadGateway)
	c.Assert(rec.Body.String(), check.Equals, "some error\n")
	c.Assert(rec.headerWrites, check.Equals, 1)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldRespectTheHandlerStatusCode(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	RegisterHandler("/apps", "GET", AuthorizationRequiredHandler(authorizedBadRequestHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerAppToken(c *check.C) {
	myApp := app.App{
		Name: "my-app",
	}
	err := s.conn.Apps().Insert(myApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	token, err := nativeScheme.AppLogin("my-app")
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps/my-app/", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	RegisterHandler("/apps/{app}/", "GET", AuthorizationRequiredHandler(authorizedOutputHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerWrongApp(c *check.C) {
	token, err := nativeScheme.AppLogin("my-app")
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps/your-app", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	RegisterHandler("/apps/{app}", "GET", AuthorizationRequiredHandler(authorizedOutputHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerAppMissng(c *check.C) {
	token, err := nativeScheme.AppLogin("my-app")
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	RegisterHandler("/apps", "GET", AuthorizationRequiredHandler(authorizedOutputHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *HandlerSuite) TestLocksAppDuringAppRequests(c *check.C) {
	myApp := app.App{
		Name: "my-app",
	}
	err := s.conn.Apps().Insert(myApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/apps/my-app/", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	handler := func(w http.ResponseWriter, r *http.Request, t auth.Token) error {
		a, appErr := app.GetByName(r.URL.Query().Get(":app"))
		c.Assert(appErr, check.IsNil)
		c.Assert(a.Lock.Reason, check.Equals, "POST /apps/my-app/")
		c.Assert(a.Lock.Owner, check.Equals, s.token.GetUserName())
		c.Assert(a.Lock.Locked, check.Equals, true)
		c.Assert(a.Lock.AcquireDate, check.NotNil)
		return nil
	}
	RegisterHandler("/apps/{app}/", "POST", AuthorizationRequiredHandler(handler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	a, err := app.GetByName(request.URL.Query().Get(":app"))
	c.Assert(err, check.IsNil)
	c.Assert(a.Lock.Locked, check.Equals, false)
}
