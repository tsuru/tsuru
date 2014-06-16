// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"net/http"
)

var (
	tokenRequiredErr = &errors.HTTP{
		Code:    http.StatusUnauthorized,
		Message: "You must provide the Authorization header",
	}
	adminRequiredErr = &errors.HTTP{
		Code:    http.StatusForbidden,
		Message: "You must be an admin",
	}
)

type Handler func(http.ResponseWriter, *http.Request) error

func (fn Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	context.AddRequestError(r, fn(w, r))
}

type authorizationRequiredHandler func(http.ResponseWriter, *http.Request, auth.Token) error

func (fn authorizationRequiredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t := context.GetAuthToken(r)
	if t == nil {
		context.AddRequestError(r, tokenRequiredErr)
	} else {
		context.AddRequestError(r, fn(w, r, t))
	}
}

type AdminRequiredHandler authorizationRequiredHandler

func (fn AdminRequiredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t := context.GetAuthToken(r)
	if t == nil {
		context.AddRequestError(r, tokenRequiredErr)
	} else if user, err := t.User(); err != nil || !user.IsAdmin() {
		context.AddRequestError(r, adminRequiredErr)
	} else {
		context.AddRequestError(r, fn(w, r, t))
	}
}
