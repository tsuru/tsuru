// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testing provide test helpers for various actions.
package apitest

import (
	"io/ioutil"
	"net/http"
	"strconv"
)

type TestHandler struct {
	Body    []byte
	Method  string
	Url     string
	Content string
	Header  http.Header
}

func (h *TestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Method = r.Method
	h.Url = r.URL.String()
	b, _ := ioutil.ReadAll(r.Body)
	h.Body = b
	h.Header = r.Header
	w.Write([]byte(h.Content))
}

type MultiTestHandler struct {
	Body               [][]byte
	Method             []string
	Url                []string
	Content            string
	ConditionalContent map[string]interface{}
	Header             []http.Header
	RspCode            int
}

func (h *MultiTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Method = append(h.Method, r.Method)
	h.Url = append(h.Url, r.URL.String())
	b, _ := ioutil.ReadAll(r.Body)
	h.Body = append(h.Body, b)
	h.Header = append(h.Header, r.Header)
	if h.RspCode == 0 {
		h.RspCode = http.StatusOK
	}
	condContent := h.ConditionalContent[r.URL.String()]
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
