// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build pprof

package api

import (
	"net/http"
	"net/http/pprof"
)

func init() {
	RegisterHandler("Get", "/debug/pprof/", http.HandlerFunc(pprof.Index))
	RegisterHandler("Get", "/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	RegisterHandler("Get", "/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	RegisterHandler("Get", "/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	RegisterHandler("Get", "/debug/pprof/heap", http.HandlerFunc(pprof.Index))
	RegisterHandler("Get", "/debug/pprof/goroutine", http.HandlerFunc(pprof.Index))
	RegisterHandler("Get", "/debug/pprof/threadcreate", http.HandlerFunc(pprof.Index))
	RegisterHandler("Get", "/debug/pprof/block", http.HandlerFunc(pprof.Index))
}
