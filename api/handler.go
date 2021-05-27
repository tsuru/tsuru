// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
)

var (
	tokenRequiredErr = &errors.HTTP{
		Code:    http.StatusUnauthorized,
		Message: "You must provide a valid Authorization header",
	}
)

type Handler func(http.ResponseWriter, *http.Request) error

func (fn Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	context.AddRequestError(r, fn(w, r))
}

type AuthorizationRequiredHandler func(http.ResponseWriter, *http.Request, auth.Token) error

func (fn AuthorizationRequiredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t := context.GetAuthToken(r)
	if t == nil {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"tsuru\" scope=\"tsuru\"")
		context.AddRequestError(r, tokenRequiredErr)
	} else {
		context.AddRequestError(r, fn(w, r, t))
	}
}

func deprecateFormContentType(r *http.Request) error {
	contentType := r.Header.Get("Content-Type")

	if contentType == "" || contentType == "application/json" {
		return nil
	}

	return fmt.Errorf("Invalid Content-Type %q", contentType)
}
