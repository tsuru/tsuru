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
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello world from tsuru - web"))
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("PORT_web")
	}
	if port == "" {
		port = "8888"
	}

	http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
}
