// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"net/http"
	"os"
)

func main() {
	generateCert()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello world from tsuru"))
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	http.ListenAndServeTLS(":"+port, "cert.pem", "key.pem", nil)
}
