// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/errors"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

type HandlerSuite struct {
	token auth.Token
}

var _ = check.Suite(&HandlerSuite{})

func (s *HandlerSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_api_handler_test")
	config.Set("auth:hash-cost", bcrypt.MinCost)

	storagev2.Reset()
}

func (s *HandlerSuite) SetUpTest(c *check.C) {
	var err error
	storagev2.ClearAllCollections(nil)
	user := &auth.User{Email: "whydidifall@thewho.com", Password: "123456"}
	_, err = nativeScheme.Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	s.token, err = nativeScheme.Login(context.TODO(), map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	app.AuthScheme = nativeScheme
}

func (s *HandlerSuite) TearDownTest(c *check.C) {
}

func (s *HandlerSuite) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func errorHandler(w http.ResponseWriter, r *http.Request) error {
	return fmt.Errorf("some error")
}

func errorHandlerWriteHeader(w http.ResponseWriter, r *http.Request) error {
	w.WriteHeader(http.StatusBadGateway)
	return errorHandler(w, r)
}

func simpleHandler(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprint(w, "success")
	return nil
}

func authorizedErrorHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return errorHandler(w, r)
}

func authorizedErrorHandlerWriteHeader(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return errorHandlerWriteHeader(w, r)
}

func authorizedBadRequestHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return &errors.HTTP{Code: http.StatusBadRequest, Message: "some error"}
}

func authorizedSimpleHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return simpleHandler(w, r)
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
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", http.MethodGet, Handler(errorHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "some error\n")
}

func (s *HandlerSuite) TestHandlerDontCallWriteHeaderIfItHasAlreadyBeenCalled(c *check.C) {
	rec := recorder{httptest.NewRecorder(), 0}
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", http.MethodGet, Handler(errorHandlerWriteHeader))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(&rec, request)
	c.Assert(rec.Code, check.Equals, http.StatusBadGateway)
	c.Assert(rec.Body.String(), check.Equals, "some error\n")
	c.Assert(rec.headerWrites, check.Equals, 1)
}

func (s *HandlerSuite) TestHandlerShouldPassAnHandlerWithoutError(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", http.MethodGet, Handler(simpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "success")
}

func (s *HandlerSuite) TestHandlerShouldSetVersionHeaders(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", http.MethodGet, Handler(simpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), check.Equals, tsuruMin)
}

func (s *HandlerSuite) TestHandlerShouldSetVersionHeadersEvenOnFail(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", http.MethodGet, Handler(errorHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), check.Equals, tsuruMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheAuthorizationHeadIsNotPresent(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	RegisterHandler("/apps", http.MethodGet, AuthorizationRequiredHandler(authorizedSimpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Header().Get("WWW-Authenticate"), check.Equals, "Bearer realm=\"tsuru\" scope=\"tsuru\"")
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a valid Authorization header\n")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnUnauthorizedIfTheTokenIsInvalid(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "what the token?!")
	RegisterHandler("/apps", http.MethodGet, AuthorizationRequiredHandler(authorizedSimpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusUnauthorized)
	c.Assert(recorder.Header().Get("WWW-Authenticate"), check.Equals, "Bearer realm=\"tsuru\" scope=\"tsuru\"")
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a valid Authorization header\n")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnTheHandlerResultIfTheTokenIsOk(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	RegisterHandler("/apps", http.MethodGet, AuthorizationRequiredHandler(authorizedSimpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "success")
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldSetVersionHeaders(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", s.token.GetValue())
	RegisterHandler("/apps", http.MethodGet, AuthorizationRequiredHandler(authorizedSimpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), check.Equals, tsuruMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldSetVersionHeadersEvenOnError(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "what the token?!")
	RegisterHandler("/apps", http.MethodGet, AuthorizationRequiredHandler(authorizedSimpleHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), check.Equals, tsuruMin)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldReturnTheHandlerErrorIfAnyHappen(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	RegisterHandler("/apps", http.MethodGet, AuthorizationRequiredHandler(authorizedErrorHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "some error\n")
}

func (s *HandlerSuite) TestAuthorizetionRequiredHandlerDontCallWriteHeaderIfItHasAlreadyBeenCalled(c *check.C) {
	rec := recorder{httptest.NewRecorder(), 0}
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	RegisterHandler("/apps", http.MethodGet, AuthorizationRequiredHandler(authorizedErrorHandlerWriteHeader))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(&rec, request)
	c.Assert(rec.Code, check.Equals, http.StatusBadGateway)
	c.Assert(rec.Body.String(), check.Equals, "some error\n")
	c.Assert(rec.headerWrites, check.Equals, 1)
}

func (s *HandlerSuite) TestAuthorizationRequiredHandlerShouldRespectTheHandlerStatusCode(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	RegisterHandler("/apps", http.MethodGet, AuthorizationRequiredHandler(authorizedBadRequestHandler))
	defer resetHandlers()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}
