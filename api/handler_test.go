// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	stderrors "errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type HandlerSuite struct {
	conn  *db.Storage
	token *auth.Token
}

var _ = Suite(&HandlerSuite{})

func (s *HandlerSuite) SetUpSuite(c *C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_api_handler_test")
	s.conn, err = db.Conn()
	c.Assert(err, IsNil)
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123"}
	err = user.Create()
	c.Assert(err, IsNil)
	s.token, _ = user.CreateToken()
}

func (s *HandlerSuite) TearDownSuite(c *C) {
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
	return &errors.Http{Code: http.StatusBadRequest, Message: "some error"}
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

func authorizedErrorHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	return errorHandler(w, r)
}

func authorizedErrorHandlerWriteHeader(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	return errorHandlerWriteHeader(w, r)
}

func authorizedBadRequestHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	return badRequestHandler(w, r)
}

func authorizedSimpleHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	return simpleHandler(w, r)
}

func authorizedOutputHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
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

func (s *HandlerSuite) TestHandlerReturns500WhenInternalHandlerReturnsAnError(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(errorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), Equals, "some error\n")
}

func (s *HandlerSuite) TestHandlerDontCallWriteHeaderIfItHasAlreadyBeenCalled(c *C) {
	recorder := recorder{httptest.NewRecorder(), 0}
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(errorHandlerWriteHeader).ServeHTTP(&recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.String(), Equals, "some error\n")
	c.Assert(recorder.headerWrites, Equals, 1)
}

func (s *HandlerSuite) TestHandlerShouldPassAnHandlerWithoutError(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(simpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), Equals, "success")
}

func (s *HandlerSuite) TestHandlerShouldSetVersionHeaders(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(simpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), Equals, craneMin)
}

func (s *HandlerSuite) TestHandlerShouldSetVersionHeadersEvenOnFail(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(errorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), Equals, craneMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheAuthorizationHeadIsNotPresent(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)

	AuthorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), Equals, "You must provide the Authorization header\n")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheTokenIsInvalid(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", "what the token?!")
	AuthorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), Equals, "Invalid token\n")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnTheHandlerResultIfTheTokenIsOk(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.token.Token)
	AuthorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), Equals, "success")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldSetVersionHeaders(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.token.Token)
	AuthorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), Equals, craneMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldSetVersionHeadersEvenOnError(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", "what the token?!")
	AuthorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), Equals, craneMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnTheHandlerErrorIfAnyHappen(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.token.Token)
	AuthorizationRequiredHandler(authorizedErrorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), Equals, "some error\n")
}

func (s *HandlerSuite) TestAuthorizetionRequiredHandlerDontCallWriteHeaderIfItHasAlreadyBeenCalled(c *C) {
	recorder := recorder{httptest.NewRecorder(), 0}
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.token.Token)
	AuthorizationRequiredHandler(authorizedErrorHandlerWriteHeader).ServeHTTP(&recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.String(), Equals, "some error\n")
	c.Assert(recorder.headerWrites, Equals, 1)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldRespectTheHandlerStatusCode(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.token.Token)
	AuthorizationRequiredHandler(authorizedBadRequestHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusBadRequest)
}

func (s *HandlerSuite) TestSetVersionHeaders(c *C) {
	recorder := httptest.NewRecorder()
	setVersionHeaders(recorder)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), Equals, craneMin)
}
