// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/bmizerany/pat"
	"net/http"
)

var m = pat.New()

func RegisterHandler(path string, method string, h http.Handler) {
	if method == "GET" {
		m.Get(path, h)
	}
	if method == "POST" {
		m.Post(path, h)
	}
	if method == "PUT" {
		m.Put(path, h)
	}
	if method == "DELETE" {
		m.Del(path, h)
	}
}

// RunAdminServer()
