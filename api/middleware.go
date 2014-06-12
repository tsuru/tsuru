// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"github.com/gorilla/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"net/http"
)

const (
	tsuruMin      = "0.9.0"
	craneMin      = "0.5.1"
	tsuruAdminMin = "0.3.0"
)

const (
	tokenContextKey int = iota
	errorContextKey
	delayedHandlerKey
)

func GetAuthToken(r *http.Request) auth.Token {
	if v := context.Get(r, tokenContextKey); v != nil {
		return v.(auth.Token)
	}
	return nil
}

func SetAuthToken(r *http.Request, t auth.Token) {
	context.Set(r, tokenContextKey, t)
}

func AddRequestError(r *http.Request, err error) {
	if err == nil {
		return
	}
	existingErr := context.Get(r, errorContextKey)
	if existingErr != nil {
		err = &errors.CompositeError{Base: existingErr.(error), Message: err.Error()}
	}
	context.Set(r, errorContextKey, err)
}

func GetRequestError(r *http.Request) error {
	if v := context.Get(r, errorContextKey); v != nil {
		return v.(error)
	}
	return nil
}

func SetDelayedHandler(r *http.Request, h http.Handler) {
	context.Set(r, delayedHandlerKey, h)
}

func validate(token string, r *http.Request) (auth.Token, error) {
	invalid := &errors.HTTP{Message: "Invalid token"}
	t, err := app.AuthScheme.Auth(token)
	if err != nil {
		return nil, invalid
	}
	if t.IsAppToken() {
		if q := r.URL.Query().Get(":app"); q != "" && t.GetAppName() != q {
			return nil, invalid
		}
	}
	return t, nil
}

func contextClearerMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	next(w, r)
	context.Clear(r)
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
	err := GetRequestError(r)
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
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		SetAuthToken(r, t)
	}
	next(w, r)
}

func runDelayedHandler(w http.ResponseWriter, r *http.Request) {
	v := context.Get(r, delayedHandlerKey)
	if v != nil {
		v.(http.Handler).ServeHTTP(w, r)
	}
}
