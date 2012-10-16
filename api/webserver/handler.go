// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/api/app"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"net/http"
)

type Handler func(http.ResponseWriter, *http.Request) error

func (fn Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			r.Body.Close()
		}
	}()
	w = &app.FilteredWriter{w}
	if err := fn(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err.Error())
	}
}

type AuthorizationRequiredHandler func(http.ResponseWriter, *http.Request, *auth.User) error

func (fn AuthorizationRequiredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			r.Body.Close()
		}
	}()
	w = &app.FilteredWriter{w}
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "You must provide the Authorization header", http.StatusUnauthorized)
	} else if user, err := auth.CheckToken(token); err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
	} else if err = fn(w, r, user); err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*errors.Http); ok {
			code = e.Code
		}
		http.Error(w, err.Error(), code)
	}
}
