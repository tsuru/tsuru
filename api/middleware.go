// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	stdLog "log"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
)

const (
	tsuruMin      = "0.18.0"
	craneMin      = "0.7.0"
	tsuruAdminMin = "0.12.0"
)

func validate(token string, r *http.Request) (auth.Token, error) {
	t, err := app.AuthScheme.Auth(token)
	if err != nil {
		t, err = auth.APIAuth(token)
		if err != nil {
			return nil, err
		}
	}
	if t.IsAppToken() {
		if q := r.URL.Query().Get(":app"); q != "" && t.GetAppName() != q {
			return nil, &errors.HTTP{
				Code:    http.StatusForbidden,
				Message: fmt.Sprintf("app token mismatch, token for %q, request for %q", t.GetAppName(), q),
			}
		}
	} else {
		if q := r.URL.Query().Get(":app"); q != "" {
			_, err = getAppFromContext(q, r)
			if err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

func contextClearerMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	defer context.Clear(r)
	next(w, r)
}

func flushingWriterMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	defer func() {
		if r.Body != nil {
			r.Body.Close()
		}
	}()
	fw := io.FlushingWriter{ResponseWriter: w}
	next(&fw, r)
}

func setVersionHeadersMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	w.Header().Set("Supported-Tsuru", tsuruMin)
	w.Header().Set("Supported-Crane", craneMin)
	w.Header().Set("Supported-Tsuru-Admin", tsuruAdminMin)
	next(w, r)
}

func errorHandlingMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	next(w, r)
	err := context.GetRequestError(r)
	if err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*errors.HTTP); ok {
			code = e.Code
		}
		flushing, ok := w.(*io.FlushingWriter)
		if ok && flushing.Wrote() {
			fmt.Fprintln(w, err)
		} else {
			http.Error(w, err.Error(), code)
		}
		log.Errorf("failure running HTTP request %s %s (%d): %s", r.Method, r.URL.Path, code, err)
	}
}

func authTokenMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	token := r.Header.Get("Authorization")
	if token != "" {
		t, err := validate(token, r)
		if err != nil {
			if err != auth.ErrInvalidToken {
				context.AddRequestError(r, err)
				return
			}
			log.Debugf("Ignored invalid token for %s: %s", r.URL.Path, err.Error())
		} else {
			context.SetAuthToken(r, t)
		}
	}
	next(w, r)
}

type appLockMiddleware struct {
	excludedHandlers []http.Handler
}

var lockWaitDuration time.Duration = 10 * time.Second

func (m *appLockMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if r.Method == "GET" {
		next(w, r)
		return
	}
	currentHandler := context.GetDelayedHandler(r)
	if currentHandler != nil {
		currentHandlerPtr := reflect.ValueOf(currentHandler).Pointer()
		for _, h := range m.excludedHandlers {
			if reflect.ValueOf(h).Pointer() == currentHandlerPtr {
				next(w, r)
				return
			}
		}
	}
	appName := r.URL.Query().Get(":app")
	if appName == "" {
		appName = r.URL.Query().Get(":appname")
	}
	if appName == "" {
		next(w, r)
		return
	}
	t := context.GetAuthToken(r)
	var owner string
	if t != nil {
		if t.IsAppToken() {
			owner = t.GetAppName()
		} else {
			owner = t.GetUserName()
		}
	}
	_, err := app.GetByName(appName)
	if err == app.ErrAppNotFound {
		context.AddRequestError(r, &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()})
		return
	}
	ok, err := app.AcquireApplicationLockWait(appName, owner, fmt.Sprintf("%s %s", r.Method, r.URL.Path), lockWaitDuration)
	if err != nil {
		context.AddRequestError(r, fmt.Errorf("Error trying to acquire application lock: %s", err))
		return
	}
	if ok {
		defer func() {
			if !context.IsPreventUnlock(r) {
				app.ReleaseApplicationLock(appName)
			}
		}()
		next(w, r)
		return
	}
	a, err := app.GetByName(appName)
	httpErr := &errors.HTTP{Code: http.StatusInternalServerError}
	if err != nil {
		if err == app.ErrAppNotFound {
			httpErr.Code = http.StatusNotFound
			httpErr.Message = err.Error()
		} else {
			httpErr.Message = fmt.Sprintf("Error to get application: %s", err)
		}
	} else {
		httpErr.Code = http.StatusConflict
		if a.Lock.Locked {
			httpErr.Message = fmt.Sprintf("%s", &a.Lock)
		} else {
			httpErr.Message = "Not locked anymore, please try again."
		}
	}
	context.AddRequestError(r, httpErr)
}

func runDelayedHandler(w http.ResponseWriter, r *http.Request) {
	h := context.GetDelayedHandler(r)
	if h != nil {
		h.ServeHTTP(w, r)
	}
}

type loggerMiddleware struct {
	logger *stdLog.Logger
}

func newLoggerMiddleware() *loggerMiddleware {
	return &loggerMiddleware{
		logger: stdLog.New(os.Stdout, "", 0),
	}
}

func (l *loggerMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	next(rw, r)
	duration := time.Since(start)
	res := rw.(negroni.ResponseWriter)
	nowFormatted := time.Now().Format(time.RFC3339Nano)
	l.logger.Printf("%s %s %s %d in %0.6fms", nowFormatted, r.Method, r.URL.Path, res.Status(), float64(duration)/float64(time.Millisecond))
}
