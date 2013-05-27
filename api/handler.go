// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"net/http"
)

const (
	tsuruMin      = "0.8"
	craneMin      = "0.4.4"
	tsuruAdminMin = "0.1.7"
)

func setVersionHeaders(w http.ResponseWriter) {
	w.Header().Set("Supported-Tsuru", tsuruMin)
	w.Header().Set("Supported-Crane", craneMin)
	w.Header().Set("Supported-Tsuru-Admin", tsuruAdminMin)
}

type handler func(http.ResponseWriter, *http.Request) error

func (fn handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setVersionHeaders(w)
	defer func() {
		if r.Body != nil {
			r.Body.Close()
		}
	}()
	fw := flushingWriter{w, false}
	if err := fn(&fw, r); err != nil {
		if fw.wrote {
			fmt.Fprintln(&fw, err)
		} else {
			http.Error(&fw, err.Error(), http.StatusInternalServerError)
		}
		log.Print(err)
	}
}

func validate(token string, r *http.Request) (*auth.Token, error) {
	if token == "" {
		return nil, &errors.Http{
			Message: "You must provide the Authorization header",
		}
	}
	invalid := &errors.Http{Message: "Invalid token"}
	t, err := auth.GetToken(token)
	if err != nil {
		return nil, invalid
	}
	if t.AppName != "" {
		if q := r.URL.Query().Get(":app"); q != "" && t.AppName != q {
			return nil, invalid
		}
	}
	return t, nil
}

type authorizationRequiredHandler func(http.ResponseWriter, *http.Request, *auth.Token) error

func (fn authorizationRequiredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setVersionHeaders(w)
	defer func() {
		if r.Body != nil {
			r.Body.Close()
		}
	}()
	fw := flushingWriter{w, false}
	token := r.Header.Get("Authorization")
	if t, err := validate(token, r); err != nil {
		http.Error(&fw, err.Error(), http.StatusUnauthorized)
	} else if err = fn(&fw, r, t); err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*errors.Http); ok {
			code = e.Code
		}
		if fw.wrote {
			fmt.Fprintln(&fw, err)
		} else {
			http.Error(&fw, err.Error(), code)
		}
		log.Print(err)
	}
}

type adminRequiredHandler authorizationRequiredHandler

func (fn adminRequiredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setVersionHeaders(w)
	defer func() {
		if r.Body != nil {
			r.Body.Close()
		}
	}()
	fw := flushingWriter{w, false}
	header := r.Header.Get("Authorization")
	if header == "" {
		http.Error(&fw, "You must provide the Authorization header", http.StatusUnauthorized)
	} else if t, err := auth.GetToken(header); err != nil {
		http.Error(&fw, "Invalid token", http.StatusUnauthorized)
	} else if user, err := t.User(); err != nil || !user.IsAdmin() {
		http.Error(&fw, "Forbidden", http.StatusForbidden)
	} else if err = fn(&fw, r, t); err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*errors.Http); ok {
			code = e.Code
		}
		if fw.wrote {
			fmt.Fprintln(&fw, err)
		} else {
			http.Error(&fw, err.Error(), code)
		}
		log.Print(err)
	}
}
