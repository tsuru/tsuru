// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	stderrors "errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type HandlerSuite struct {
	conn  *db.Storage
	token *auth.Token
}

var _ = gocheck.Suite(&HandlerSuite{})

func (s *HandlerSuite) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_handler_test")
	config.Set("auth:salt", "tsuru-salt")
	config.Set("auth:hash-cost", 4)
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	err = user.Create()
	c.Assert(err, gocheck.IsNil)
	s.token, err = user.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
	team := auth.Team{Name: "tsuruteam", Users: []string{user.Email}}
	conn, _ := db.Conn()
	defer conn.Close()
	err = conn.Teams().Insert(team)
	c.Assert(err, gocheck.IsNil)
	config.Set("admin-team", team.Name)
}

func (s *HandlerSuite) TearDownSuite(c *gocheck.C) {
	s.conn.Apps().Database.DropDatabase()
}

func errorHandler(w http.ResponseWriter, r *http.Request) error {
	return stderrors.New("some error")
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

func authorizedErrorHandler(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	return errorHandler(w, r)
}

func authorizedErrorHandlerWriteHeader(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	return errorHandlerWriteHeader(w, r)
}

func authorizedBadRequestHandler(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	return badRequestHandler(w, r)
}

func authorizedSimpleHandler(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	return simpleHandler(w, r)
}

func authorizedOutputHandler(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
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

func (s *HandlerSuite) TestHandlerReturns500WhenInternalHandlerReturnsAnError(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	Handler(errorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), gocheck.Equals, "some error\n")
}

func (s *HandlerSuite) TestHandlerDontCallWriteHeaderIfItHasAlreadyBeenCalled(c *gocheck.C) {
	recorder := recorder{httptest.NewRecorder(), 0}
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	Handler(errorHandlerWriteHeader).ServeHTTP(&recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.String(), gocheck.Equals, "some error\n")
	c.Assert(recorder.headerWrites, gocheck.Equals, 1)
}

func (s *HandlerSuite) TestHandlerShouldPassAnHandlerWithoutError(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	Handler(simpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), gocheck.Equals, "success")
}

func (s *HandlerSuite) TestHandlerShouldSetVersionHeaders(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	Handler(simpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), gocheck.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), gocheck.Equals, craneMin)
	c.Assert(recorder.Header().Get("Supported-Tsuru-Admin"), gocheck.Equals, tsuruAdminMin)
}

func (s *HandlerSuite) TestHandlerShouldSetVersionHeadersEvenOnFail(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	Handler(errorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), gocheck.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), gocheck.Equals, craneMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheAuthorizationHeadIsNotPresent(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)

	authorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), gocheck.Equals, "You must provide the Authorization header\n")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheTokenIsInvalid(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "what the token?!")
	authorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), gocheck.Equals, "Invalid token\n")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnTheHandlerResultIfTheTokenIsOk(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.Token)
	authorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), gocheck.Equals, "success")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldSetVersionHeaders(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", s.token.Token)
	authorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), gocheck.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), gocheck.Equals, craneMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldSetVersionHeadersEvenOnError(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "what the token?!")
	authorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), gocheck.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), gocheck.Equals, craneMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnTheHandlerErrorIfAnyHappen(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.Token)
	authorizationRequiredHandler(authorizedErrorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), gocheck.Equals, "some error\n")
}

func (s *HandlerSuite) TestAuthorizetionRequiredHandlerDontCallWriteHeaderIfItHasAlreadyBeenCalled(c *gocheck.C) {
	recorder := recorder{httptest.NewRecorder(), 0}
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.Token)
	authorizationRequiredHandler(authorizedErrorHandlerWriteHeader).ServeHTTP(&recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.String(), gocheck.Equals, "some error\n")
	c.Assert(recorder.headerWrites, gocheck.Equals, 1)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldRespectTheHandlerStatusCode(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.Token)
	authorizationRequiredHandler(authorizedBadRequestHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusBadRequest)
}

func (s *HandlerSuite) TestAdminRequiredHandlerShouldReturnUnauthorizedIfTheAuthorizationHeadIsNotPresent(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	adminRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), gocheck.Equals, "You must provide the Authorization header\n")
}

func (s *HandlerSuite) TestAdminRequiredHandlerShouldReturnUnauthorizedIfTheTokenIsInvalid(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "what the token?!")
	adminRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), gocheck.Equals, "Invalid token\n")
}

func (s *HandlerSuite) TestAdminRequiredHandlerShouldReturnForbiddenIfTheUserIsNotAdmin(c *gocheck.C) {
	user := &auth.User{Email: "rain@gotthard.com", Password: "123456"}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	token, err := user.CreateToken("123456")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.Token})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+token.Token)
	adminRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), gocheck.Equals, "Forbidden\n")
}

func (s *HandlerSuite) TestAdminRequiredHandlerShouldReturnTheHandlerResultIfTheTokenIsOk(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.Token)
	adminRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), gocheck.Equals, "success")
}

func (s *HandlerSuite) TestAdminRequiredHandlerShouldSetVersionHeaders(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.Token)
	adminRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), gocheck.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), gocheck.Equals, craneMin)
}

func (s *HandlerSuite) TestAdminRequiredHandlerShouldSetVersionHeadersEvenOnError(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "what the token?!")
	adminRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), gocheck.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), gocheck.Equals, craneMin)
}

func (s *HandlerSuite) TestAdminRequiredHandlerShouldReturnTheHandlerErrorIfAnyHappen(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.Token)
	adminRequiredHandler(authorizedErrorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), gocheck.Equals, "some error\n")
}

func (s *HandlerSuite) TestAdminRequiredHandlerDontCallWriteHeaderIfItHasAlreadyBeenCalled(c *gocheck.C) {
	recorder := recorder{httptest.NewRecorder(), 0}
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.Token)
	adminRequiredHandler(authorizedErrorHandlerWriteHeader).ServeHTTP(&recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.String(), gocheck.Equals, "some error\n")
	c.Assert(recorder.headerWrites, gocheck.Equals, 1)
}

func (s *HandlerSuite) TestAdminRequiredHandlerShouldRespectTheHandlerStatusCode(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.Token)
	adminRequiredHandler(authorizedBadRequestHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusBadRequest)
}

func (s *HandlerSuite) TestSetVersionHeaders(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	setVersionHeaders(recorder)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), gocheck.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), gocheck.Equals, craneMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerAppToken(c *gocheck.C) {
	token, err := auth.CreateApplicationToken("my-app")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.Token})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=my-app", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+token.Token)
	authorizationRequiredHandler(authorizedOutputHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerWrongApp(c *gocheck.C) {
	token, err := auth.CreateApplicationToken("my-app")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.Token})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=your-app", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+token.Token)
	authorizationRequiredHandler(authorizedOutputHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusUnauthorized)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerAppMissng(c *gocheck.C) {
	token, err := auth.CreateApplicationToken("my-app")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.Token})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+token.Token)
	authorizationRequiredHandler(authorizedOutputHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, gocheck.Equals, http.StatusOK)
}
