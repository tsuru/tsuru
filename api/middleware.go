// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"net/http"
	"reflect"
)

const (
	tsuruMin      = "0.9.0"
	craneMin      = "0.5.1"
	tsuruAdminMin = "0.3.0"
)

func validate(token string, r *http.Request) (auth.Token, error) {
	t, err := app.AuthScheme.Auth(token)
	if err != nil {
		return nil, fmt.Errorf("Invalid token")
	}
	if t.IsAppToken() {
		if q := r.URL.Query().Get(":app"); q != "" && t.GetAppName() != q {
			return nil, fmt.Errorf("App token mismatch, token for %q, request for %q", t.GetAppName(), q)
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
		log.Error(err.Error())
	}
}

func authTokenMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	token := r.Header.Get("Authorization")
	if token != "" {
		t, err := validate(token, r)
		if err != nil {
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
	ok, err := app.AcquireApplicationLock(appName, owner, fmt.Sprintf("%s %s", r.Method, r.URL.Path))
	if err != nil {
		context.AddRequestError(r, fmt.Errorf("Error trying to acquire application lock: %s", err))
		return
	}
	if ok {
		if !context.IsPreventUnlock(r) {
			defer app.ReleaseApplicationLock(appName)
		}
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
