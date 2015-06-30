// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
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

func (s *S) TestContextClearerMiddleware(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	context.AddRequestError(request, fmt.Errorf("Some Error"))
	h, log := doHandler()
	contextClearerMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	contErr := context.GetRequestError(request)
	c.Assert(contErr, check.IsNil)
}

func (s *S) TestFlushingWriterMiddleware(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	flushingWriterMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	_, ok := log.w.(*io.FlushingWriter)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestSetVersionHeadersMiddleware(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	setVersionHeadersMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	c.Assert(recorder.Header().Get("Supported-Tsuru"), check.Equals, tsuruMin)
	c.Assert(recorder.Header().Get("Supported-Crane"), check.Equals, craneMin)
	c.Assert(recorder.Header().Get("Supported-Tsuru-Admin"), check.Equals, tsuruAdminMin)
}

func (s *S) TestErrorHandlingMiddlewareWithoutError(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	errorHandlingMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	c.Assert(recorder.Code, check.Equals, 200)
}

func (s *S) TestErrorHandlingMiddlewareWithError(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	context.AddRequestError(request, fmt.Errorf("something"))
	errorHandlingMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	c.Assert(recorder.Code, check.Equals, 500)
}

func (s *S) TestErrorHandlingMiddlewareWithHTTPError(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	context.AddRequestError(request, &errors.HTTP{Code: 403, Message: "other msg"})
	errorHandlingMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	c.Assert(recorder.Code, check.Equals, 403)
}

func (s *S) TestAuthTokenMiddlewareWithoutToken(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	t := context.GetAuthToken(request)
	c.Assert(t, check.IsNil)
}

func (s *S) TestAuthTokenMiddlewareWithToken(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	t := context.GetAuthToken(request)
	c.Assert(t.GetValue(), check.Equals, s.token.GetValue())
	c.Assert(t.GetUserName(), check.Equals, s.token.GetUserName())
}

func (s *S) TestAuthTokenMiddlewareWithAPIToken(c *check.C) {
	user := auth.User{Email: "para@xmen.com", APIKey: "347r3487rh3489hr34897rh487hr0377rg308rg32"}
	err := s.conn.Users().Insert(&user)
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+user.APIKey)
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	t := context.GetAuthToken(request)
	c.Assert(t.GetValue(), check.Equals, user.APIKey)
	c.Assert(t.GetUserName(), check.Equals, user.Email)
}

func (s *S) TestAuthTokenMiddlewareWithAppToken(c *check.C) {
	token, err := nativeScheme.AppLogin("abc")
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=abc", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	t := context.GetAuthToken(request)
	c.Assert(t.GetValue(), check.Equals, token.GetValue())
	c.Assert(t.GetAppName(), check.Equals, "abc")
}

func (s *S) TestAuthTokenMiddlewareWithIncorrectAppToken(c *check.C) {
	token, err := nativeScheme.AppLogin("xyz")
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=abc", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	t := context.GetAuthToken(request)
	c.Assert(t, check.IsNil)
	c.Assert(log.called, check.Equals, false)
	err = context.GetRequestError(request)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAuthTokenMiddlewareWithInvalidToken(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer ifyougotozah'ha'dumyoulldie")
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	t := context.GetAuthToken(request)
	c.Assert(t, check.IsNil)
}

func (s *S) TestAuthTokenMiddlewareWithInvalidAPIToken(c *check.C) {
	user := auth.User{Email: "para@xmen.com", APIKey: "347r3487rh3489hr34897rh487hr0377rg308rg32"}
	err := s.conn.Users().Insert(&user)
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer 12eh923d8ydh238eun`1po2ueh1`p2")
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	t := context.GetAuthToken(request)
	c.Assert(t, check.IsNil)
}

func (s *S) TestAuthTokenMiddlewareUserTokenForApp(c *check.C) {
	a := app.App{Name: "something", Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=something", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
	t := context.GetAuthToken(request)
	c.Assert(t.GetValue(), check.Equals, s.token.GetValue())
	c.Assert(t.GetUserName(), check.Equals, s.token.GetUserName())
}

func (s *S) TestAuthTokenMiddlewareUserTokenAppNotFound(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=something", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, false)
	err = context.GetRequestError(request)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestAuthTokenMiddlewareUserTokenNoAccessToTheApp(c *check.C) {
	a := app.App{Name: "something"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=something", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	h, log := doHandler()
	authTokenMiddleware(recorder, request, h)
	c.Assert(log.called, check.Equals, false)
	err = context.GetRequestError(request)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRunDelayedHandlerWithoutHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	runDelayedHandler(recorder, request)
}

func (s *S) TestRunDelayedHandlerWithHandler(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	context.SetDelayedHandler(request, h)
	runDelayedHandler(recorder, request)
	c.Assert(log.called, check.Equals, true)
}

func (s *S) TestAppLockMiddlewareDoesNothingWithoutApp(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
}

func (s *S) TestAppLockMiddlewareDoesNothingForGetRequests(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/?:app=abc", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
}

func (s *S) TestAppLockMiddlewareReturns404IfNotApp(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=abc", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, check.Equals, false)
	httpErr := context.GetRequestError(request).(*errors.HTTP)
	c.Assert(httpErr.Code, check.Equals, http.StatusNotFound)
	c.Assert(httpErr.Message, check.Equals, "App not found.")
	request, err = http.NewRequest("POST", "/?:appname=abc", nil)
	c.Assert(err, check.IsNil)
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, check.Equals, false)
	httpErr = context.GetRequestError(request).(*errors.HTTP)
	c.Assert(httpErr.Code, check.Equals, http.StatusNotFound)
	c.Assert(httpErr.Message, check.Equals, "App not found.")
}

func (s *S) TestAppLockMiddlewareOnLockedApp(c *check.C) {
	oldDuration := lockWaitDuration
	lockWaitDuration = 1 * time.Second
	defer func() { lockWaitDuration = oldDuration }()
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
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=my-app", nil)
	c.Assert(err, check.IsNil)
	h, log := doHandler()
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, check.Equals, false)
	httpErr := context.GetRequestError(request).(*errors.HTTP)
	c.Assert(httpErr.Code, check.Equals, http.StatusConflict)
	c.Assert(httpErr.Message, check.Matches, "App locked by someone, running /app/my-app/deploy. Acquired in 2048-11-10.*")
}

func (s *S) TestAppLockMiddlewareLocksAndUnlocks(c *check.C) {
	myApp := app.App{
		Name: "my-app",
	}
	err := s.conn.Apps().Insert(myApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=my-app", nil)
	c.Assert(err, check.IsNil)
	called := false
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, func(w http.ResponseWriter, r *http.Request) {
		a, err := app.GetByName(request.URL.Query().Get(":app"))
		c.Assert(err, check.IsNil)
		c.Assert(a.Lock.Locked, check.Equals, true)
		called = true
	})
	c.Assert(called, check.Equals, true)
	a, err := app.GetByName(request.URL.Query().Get(":app"))
	c.Assert(err, check.IsNil)
	c.Assert(a.Lock.Locked, check.Equals, false)
}

func (s *S) TestAppLockMiddlewareWithPreventUnlock(c *check.C) {
	myApp := app.App{
		Name: "my-app",
	}
	err := s.conn.Apps().Insert(myApp)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=my-app", nil)
	c.Assert(err, check.IsNil)
	called := false
	context.SetPreventUnlock(request)
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, func(w http.ResponseWriter, r *http.Request) {
		a, err := app.GetByName(request.URL.Query().Get(":app"))
		c.Assert(err, check.IsNil)
		c.Assert(a.Lock.Locked, check.Equals, true)
		called = true
	})
	c.Assert(called, check.Equals, true)
	a, err := app.GetByName(request.URL.Query().Get(":app"))
	c.Assert(err, check.IsNil)
	c.Assert(a.Lock.Locked, check.Equals, true)
}

func (s *S) TestAppLockMiddlewareDoesNothingForExcludedHandlers(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=abc", nil)
	c.Assert(err, check.IsNil)
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	context.SetDelayedHandler(request, finalHandler)
	h, log := doHandler()
	m := &appLockMiddleware{
		excludedHandlers: []http.Handler{finalHandler},
	}
	m.ServeHTTP(recorder, request, h)
	c.Assert(log.called, check.Equals, true)
}

func (s *S) TestAppLockMiddlewareWaitForLock(c *check.C) {
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
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": myApp.Name})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/?:app=my-app", nil)
	c.Assert(err, check.IsNil)
	called := false
	wg := sync.WaitGroup{}
	wg.Add(1)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		time.Sleep(time.Second)
		app.ReleaseApplicationLock(myApp.Name)
	}()
	m := &appLockMiddleware{}
	m.ServeHTTP(recorder, request, func(w http.ResponseWriter, r *http.Request) {
		a, err := app.GetByName(request.URL.Query().Get(":app"))
		c.Assert(err, check.IsNil)
		c.Assert(a.Lock.Locked, check.Equals, true)
		called = true
	})
	c.Assert(called, check.Equals, true)
	a, err := app.GetByName(request.URL.Query().Get(":app"))
	c.Assert(err, check.IsNil)
	c.Assert(a.Lock.Locked, check.Equals, false)
}

func (s *S) TestLoggerMiddleware(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/my/path", nil)
	c.Assert(err, check.IsNil)
	h, handlerLog := doHandler()
	handlerLog.sleep = 100 * time.Millisecond
	handlerLog.response = http.StatusOK
	var out bytes.Buffer
	middle := loggerMiddleware{
		logger: log.New(&out, "", 0),
	}
	middle.ServeHTTP(negroni.NewResponseWriter(recorder), request, h)
	c.Assert(handlerLog.called, check.Equals, true)
	timePart := time.Now().Format(time.RFC3339Nano)[:19]
	c.Assert(out.String(), check.Matches, fmt.Sprintf(`%s\..+? PUT /my/path 200 in 10\d\.\d+ms`+"\n", timePart))
}
