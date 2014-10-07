// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	tsuruErr "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

type handlerLog struct {
	w        http.ResponseWriter
	r        *http.Request
	called   bool
	sleep    time.Duration
	response int
}

func doHandler() (http.HandlerFunc, *handlerLog) {
	h := &handlerLog{}
	return func(w http.ResponseWriter, r *http.Request) {
		if h.sleep != 0 {
			time.Sleep(h.sleep)
		}
		h.called = true
		h.w = w
		h.r = r
		if h.response != 0 {
			w.WriteHeader(h.response)
		}
	}, h
}

func (s *S) TestContextClearerMiddleware(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	context.AddRequestError(request, errors.New("Some Error"))
	h, log := doHandler()
	contextClearerMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	contErr := context.GetRequestError(request)
	c.Assert(contErr, gocheck.IsNil)
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
	context.AddRequestError(request, errors.New("something"))
	errorHandlingMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	c.Assert(recorder.Code, gocheck.Equals, 500)
}

func (s *S) TestErrorHandlingMiddlewareWithHTTPError(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	context.AddRequestError(request, &tsuruErr.HTTP{Code: 403, Message: "other msg"})
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
	t := context.GetAuthToken(request)
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
	t := context.GetAuthToken(request)
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
	t := context.GetAuthToken(request)
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
	t := context.GetAuthToken(request)
	c.Assert(t, gocheck.IsNil)
	c.Assert(log.called, gocheck.Equals, true)
}

func (s *S) TestAuthTokenMiddlewareWithInvalidToken(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer ifyougotozah'ha'dumyoulldie")
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	t := context.GetAuthToken(request)
	c.Assert(t, gocheck.IsNil)
}

func (s *S) TestAuthTokenMiddlewareUserTokenForApp(c *gocheck.C) {
	a := app.App{Name: "something", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=something", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
	t := context.GetAuthToken(request)
	c.Assert(t.GetValue(), gocheck.Equals, s.token.GetValue())
	c.Assert(t.GetUserName(), gocheck.Equals, s.token.GetUserName())
}

func (s *S) TestAuthTokenMiddlewareUserTokenAppNotFound(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=something", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, false)
	err = context.GetRequestError(request)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*tsuruErr.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusNotFound)
}

func (s *S) TestAuthTokenMiddlewareUserTokenNoAccessToTheApp(c *gocheck.C) {
	a := app.App{Name: "something"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=something", nil)
	c.Assert(err, gocheck.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, false)
	err = context.GetRequestError(request)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*tsuruErr.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusForbidden)
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
	context.SetDelayedHandler(request, h)
	runDelayedHandler(recorder, request)
	c.Assert(log.called, gocheck.Equals, true)
}

func (s *S) TestAppLockMiddlewareDoesNothingWithoutApp(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
}

func (s *S) TestAppLockMiddlewareDoesNothingForGetRequests(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=abc", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
}

func (s *S) TestAppLockMiddlewareReturns404IfNotApp(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=abc", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, false)
	httpErr := context.GetRequestError(request).(*tsuruErr.HTTP)
	c.Assert(httpErr.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(httpErr.Message, gocheck.Equals, "app not found")
	request, err = http.NewRequest("POST", "/?:appname=abc", nil)
	c.Assert(err, gocheck.IsNil)
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, false)
	httpErr = context.GetRequestError(request).(*tsuruErr.HTTP)
	c.Assert(httpErr.Code, gocheck.Equals, http.StatusNotFound)
	c.Assert(httpErr.Message, gocheck.Equals, "app not found")
}

func (s *S) TestAppLockMiddlewareOnLockedApp(c *gocheck.C) {
	myApp := app.App{
		Name: "my-app",
		Lock: app.AppLock{
			Locked:      true,
			Reason:      "/app/my-app/deploy",
			Owner:       "someone",
			AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC),
		},
	}
	err := s.conn.Apps().Insert(myApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=my-app", nil)
	c.Assert(err, gocheck.IsNil)
	h, log := doHandler()
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, false)
	httpErr := context.GetRequestError(request).(*tsuruErr.HTTP)
	c.Assert(httpErr.Code, gocheck.Equals, http.StatusConflict)
	c.Assert(httpErr.Message, gocheck.Matches, "App locked by someone, running /app/my-app/deploy. Acquired in 2048-11-10.*")
}

func (s *S) TestAppLockMiddlewareLocksAndUnlocks(c *gocheck.C) {
	myApp := app.App{
		Name: "my-app",
	}
	err := s.conn.Apps().Insert(myApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=my-app", nil)
	c.Assert(err, gocheck.IsNil)
	called := false
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, func(w http.ResponseWriter, r *http.Request) {
		a, err := app.GetByName(request.URL.Query().Get(":app"))
		c.Assert(err, gocheck.IsNil)
		c.Assert(a.Lock.Locked, gocheck.Equals, true)
		called = true
	})
	c.Assert(called, gocheck.Equals, true)
	a, err := app.GetByName(request.URL.Query().Get(":app"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Lock.Locked, gocheck.Equals, false)
}

func (s *S) TestAppLockMiddlewareWithPreventUnlock(c *gocheck.C) {
	myApp := app.App{
		Name: "my-app",
	}
	err := s.conn.Apps().Insert(myApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=my-app", nil)
	c.Assert(err, gocheck.IsNil)
	called := false
	context.SetPreventUnlock(request)
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, func(w http.ResponseWriter, r *http.Request) {
		a, err := app.GetByName(request.URL.Query().Get(":app"))
		c.Assert(err, gocheck.IsNil)
		c.Assert(a.Lock.Locked, gocheck.Equals, true)
		called = true
	})
	c.Assert(called, gocheck.Equals, true)
	a, err := app.GetByName(request.URL.Query().Get(":app"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Lock.Locked, gocheck.Equals, true)
}

func (s *S) TestAppLockMiddlewareDoesNothingForExcludedHandlers(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=abc", nil)
	c.Assert(err, gocheck.IsNil)
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	context.SetDelayedHandler(request, finalHandler)
	h, log := doHandler()
	m := &appLockMiddleware{
		excludedHandlers: []http.Handler{finalHandler},
	}
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, gocheck.Equals, true)
}

func (s *S) TestLoggerMiddleware(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/my/path", nil)
	c.Assert(err, gocheck.IsNil)
	h, handlerLog := doHandler()
	handlerLog.sleep = 100 * time.Millisecond
	handlerLog.response = http.StatusOK
	var out bytes.Buffer
	middle := loggerMiddleware{
		logger: log.New(&out, "", 0),
	}
	middle.ServeHTTP(negroni.NewResponseWriter(recorder), request, h)
	c.Assert(handlerLog.called, gocheck.Equals, true)
	timePart := time.Now().Format(time.RFC3339Nano)[:19]
	c.Assert(out.String(), gocheck.Matches, fmt.Sprintf(`%s\..+? PUT /my/path 200 in 10\d\.\d+ms`+"\n", timePart))
}
