// Copyright 2025 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/startup", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello world from tsuru startupcheck - web secondary"))
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello world from tsuru - web secondary"))
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}
	http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
}
