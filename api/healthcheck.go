// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"github.com/globocom/tsuru/db"
	"net/http"
)

func healthcheck(w http.ResponseWriter, r *http.Request) {
	conn, err := db.NewStorage()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to connect to MongoDB: %s", err)
		return
	}
	defer conn.Close()
	err = conn.Apps().Database.Session.Ping()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to ping MongoDB: %s", err)
		return
	}
	w.Write([]byte("WORKING"))
}
