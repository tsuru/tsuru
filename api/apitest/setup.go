// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testing provides test helpers for various actions.
package apitest

import (
	"io"
	"net/http"
	"strconv"
	"sync"
)

type TestHandler struct {
	Body    []byte
	Method  string
	URL     string
	Content string
	Header  http.Header
}

func (h *TestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Method = r.Method
	h.URL = r.URL.String()
	b, _ := io.ReadAll(r.Body)
	h.Body = b
	h.Header = r.Header
	w.Write([]byte(h.Content))
}

type MultiTestHandler struct {
	Body               [][]byte
	Method             []string
	URL                []string
	Content            string
	ConditionalContent map[string]interface{}
	Header             []http.Header
	RspCode            int
	RspHeader          http.Header
	Hook               func(w http.ResponseWriter, r *http.Request) bool
	mu                 sync.Mutex
}

func (h *MultiTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Method = append(h.Method, r.Method)
	h.URL = append(h.URL, r.URL.String())
	b, _ := io.ReadAll(r.Body)
	h.Body = append(h.Body, b)
	h.Header = append(h.Header, r.Header)
	if h.Hook != nil && h.Hook(w, r) {
		return
	}
	if h.RspCode == 0 {
		h.RspCode = http.StatusOK
	}
	if h.RspHeader != nil {
		for k, values := range h.RspHeader {
			for _, value := range values {
				w.Header().Add(k, value)
			}
		}
	}
	condContent := h.ConditionalContent[r.Method+" "+r.URL.String()]
	if condContent == nil {
		condContent = h.ConditionalContent[r.URL.String()]
	}
	if content, ok := condContent.(string); ok {
		w.WriteHeader(h.RspCode)
		w.Write([]byte(content))
	} else if content, ok := condContent.([]string); ok {
		code, _ := strconv.Atoi(content[0])
		w.WriteHeader(code)
		w.Write([]byte(content[1]))
	} else {
		w.WriteHeader(h.RspCode)
		w.Write([]byte(h.Content))
	}
}

func (h *MultiTestHandler) WithLock(fn func()) {
	h.mu.Lock()
	fn()
	h.mu.Unlock()
}
