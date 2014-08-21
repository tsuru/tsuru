// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/db"
)

func healthcheck(w http.ResponseWriter, r *http.Request) {
	conn, err := db.Conn()
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
