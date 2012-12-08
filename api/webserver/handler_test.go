// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	stderrors "errors"
	"fmt"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	u *auth.User
	t *auth.Token
}

var _ = Suite(&S{})

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

func (s *S) SetUpSuite(c *C) {
	db.Session, _ = db.Open("127.0.0.1:27017", "tsuru_handler_test")
	s.u = &auth.User{Email: "handler@tsuru.globo.com", Password: "123"}
	s.u.Create()
	s.t, _ = s.u.CreateToken()
}

func (s *S) TearDownSuite(c *C) {
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func (s *S) TestHandlerReturns500WhenInternalHandlerReturnsAnError(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(errorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), Equals, "some error\n")
}

func (s *S) TestHandlerDontCallWriteHeaderIfItHasAlreadyBeenCalled(c *C) {
	recorder := recorder{httptest.NewRecorder(), 0}
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(errorHandlerWriteHeader).ServeHTTP(&recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusBadGateway)
	c.Assert(recorder.Body.String(), Equals, "some error\n")
	c.Assert(recorder.headerWrites, Equals, 1)
}

func (s *S) TestHandlerShouldPassAnHandlerWithoutError(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(simpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), Equals, "success")
}

func (s *S) TestHandlerShouldSetVersionHeaders(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(simpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), Equals, craneMin)
}

func (s *S) TestHandlerShouldSetVersionHeadersEvenOnFail(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(errorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), Equals, craneMin)
}

func (s *S) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheAuthorizationHeadIsNotPresent(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)

	AuthorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), Equals, "You must provide the Authorization header\n")
}

func (s *S) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheTokenIsInvalid(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", "what the token?!")
	AuthorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusUnauthorized)
	c.Assert(recorder.Body.String(), Equals, "Invalid token\n")
}

func (s *S) TestAuthorizationRequiredHandlerShouldReturnTheHandlerResultIfTheTokenIsOk(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.t.Token)
	AuthorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), Equals, "success")
}

func (s *S) TestAuthorizationRequiredHandlerShouldSetVersionHeaders(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.t.Token)
	AuthorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), Equals, craneMin)
}

func (s *S) TestAuthorizationRequiredHandlerShouldSetVersionHeadersEvenOnError(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", "what the token?!")
	AuthorizationRequiredHandler(authorizedSimpleHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), Equals, craneMin)
}

func (s *S) TestAuthorizationRequiredHandlerShouldReturnTheHandlerErrorIfAnyHappen(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.t.Token)
	AuthorizationRequiredHandler(authorizedErrorHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), Equals, "some error\n")
}

func (s *S) TestAuthorizationRequiredHandlerShouldRespectTheHandlerStatusCode(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.t.Token)
	AuthorizationRequiredHandler(authorizedBadRequestHandler).ServeHTTP(recorder, request)
	c.Assert(recorder.Code, Equals, http.StatusBadRequest)
}

func (s *S) TestAuthorizationRequiredHandlerShouldFilterOutputFromJuju(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	request.Header.Set("Authorization", s.t.Token)
	AuthorizationRequiredHandler(authorizedOutputHandler).ServeHTTP(recorder, request)
	notExpected := ".*2012-06-05 17:03:36,887 WARNING.*"
	result := strings.Replace(recorder.Body.String(), "\n", "", -1)
	c.Assert(result, Not(Matches), notExpected)
}

func (s *S) TestHandlerShouldFilterOutputFromJuju(c *C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, IsNil)
	Handler(outputHandler).ServeHTTP(recorder, request)
	notExpected := ".*2012-06-05 17:03:36,887 WARNING.*"
	result := strings.Replace(recorder.Body.String(), "\n", "", -1)
	c.Assert(result, Not(Matches), notExpected)
}

func (s *S) TestSetVersionHeaders(c *C) {
	recorder := httptest.NewRecorder()
	setVersionHeaders(recorder)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), Equals, craneMin)
}
