// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"github.com/gorilla/context"
	tsuruErr "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type handlerLog struct {
	w      http.ResponseWriter
	r      *http.Request
	called bool
}

func doHandler() (http.HandlerFunc, *handlerLog) {
	h := &handlerLog{}
	return func(w http.ResponseWriter, r *http.Request) {
		h.called = true
		h.w = w
		h.r = r
	}, h
}

func (s *S) TestContextClearerMiddleware(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	context.Set(request, "key", "something")
	h, log := doHandler()
	contextClearerMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	val := context.Get(request, "key")
	c.Assert(val, gocheck.IsNil)
}

func (s *S) TestFlushingWriterMiddleware(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	flushingWriterMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	_, ok := log.w.(*io.FlushingWriter)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestSetVersionHeadersMiddleware(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	setVersionHeadersMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), gocheck.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), gocheck.Equals, craneMin)
	c.Assert(recorder.Header().Get("Supported-Tsuru-Admin"), gocheck.Equals, tsuruAdminMin)
}

func (s *S) TestErrorHandlingMiddlewareWithoutError(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	errorHandlingMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	c.Assert(recorder.Code, gocheck.Equals, 200)
}

func (s *S) TestErrorHandlingMiddlewareWithError(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	AddRequestError(request, errors.New("something"))
	errorHandlingMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	c.Assert(recorder.Code, gocheck.Equals, 500)
}

func (s *S) TestErrorHandlingMiddlewareWithHTTPError(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	AddRequestError(request, &tsuruErr.HTTP{Code: 403, Message: "other msg"})
	errorHandlingMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	c.Assert(recorder.Code, gocheck.Equals, 403)
}

func (s *S) TestAuthTokenMiddlewareWithoutToken(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	t := GetAuthToken(request)
	c.Assert(t, gocheck.IsNil)
}

func (s *S) TestAuthTokenMiddlewareWithToken(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	t := GetAuthToken(request)
	c.Assert(t.GetValue(), gocheck.Equals, s.token.GetValue())
	c.Assert(t.GetUserName(), gocheck.Equals, s.token.GetUserName())
}

func (s *S) TestAuthTokenMiddlewareWithAppToken(c *gocheck.C) {
	token, err := nativeScheme.AppLogin("abc")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=abc", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	t := GetAuthToken(request)
	c.Assert(t.GetValue(), gocheck.Equals, token.GetValue())
	c.Assert(t.GetAppName(), gocheck.Equals, "abc")
}

func (s *S) TestAuthTokenMiddlewareWithIncorrectAppToken(c *gocheck.C) {
	token, err := nativeScheme.AppLogin("xyz")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=abc", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, false)
	c.Assert(recorder.Code, gocheck.Equals, 401)
}

func (s *S) TestRunDelayedHandlerWithoutHandler(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	runDelayedHandler(recorder, request)
}

func (s *S) TestRunDelayedHandlerWithHandler(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	SetDelayedHandler(request, h)
	runDelayedHandler(recorder, request)
	c.Assert(log.called, gocheck.Equals, true)
}
