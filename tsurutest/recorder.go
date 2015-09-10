// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsurutest

import (
	"net/http/httptest"
	"sync"
)

type SafeResponseRecorder struct {
	*httptest.ResponseRecorder
	mut sync.Mutex
}

func NewSafeResponseRecorder() *SafeResponseRecorder {
	return &SafeResponseRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (r *SafeResponseRecorder) Write(buf []byte) (int, error) {
	r.mut.Lock()
	defer r.mut.Unlock()
	return r.ResponseRecorder.Write(buf)
}

func (r *SafeResponseRecorder) WriteHeader(code int) {
	r.mut.Lock()
	defer r.mut.Unlock()
	r.ResponseRecorder.WriteHeader(code)
}
