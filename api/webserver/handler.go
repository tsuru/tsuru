// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/tsuru/api/auth"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"net/http"
)

const (
	tsuruMin = "0.3"
	craneMin = "0.2"
)

func setVersionHeaders(w http.ResponseWriter) {
	w.Header().Set("Supported-Tsuru", tsuruMin)
	w.Header().Set("Supported-Crane", craneMin)
}

type Handler func(http.ResponseWriter, *http.Request) error

func (fn Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setVersionHeaders(w)
	defer func() {
		if r.Body != nil {
			r.Body.Close()
		}
	}()
	fw := FilteredWriter{w, false}
	if err := fn(&fw, r); err != nil {
		if fw.wrote {
			fmt.Fprintln(&fw, err)
		} else {
			http.Error(&fw, err.Error(), http.StatusInternalServerError)
		}
		log.Print(err)
	}
}

type AuthorizationRequiredHandler func(http.ResponseWriter, *http.Request, *auth.User) error

func (fn AuthorizationRequiredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setVersionHeaders(w)
	defer func() {
		if r.Body != nil {
			r.Body.Close()
		}
	}()
	fw := FilteredWriter{w, false}
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(&fw, "You must provide the Authorization header", http.StatusUnauthorized)
	} else if user, err := auth.CheckToken(token); err != nil {
		http.Error(&fw, "Invalid token", http.StatusUnauthorized)
	} else if err = fn(&fw, r, user); err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*errors.Http); ok {
			code = e.Code
		}
		if fw.wrote {
			fmt.Fprintln(&fw, err)
		} else {
			http.Error(&fw, err.Error(), code)
		}
	}
}
