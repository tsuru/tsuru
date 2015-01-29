// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/repository"
)

func healthcheck(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	status := http.StatusOK
	mongoDBStatus := "WORKING"
	fmt.Fprint(&buf, "MongoDB: ")
	conn, err := db.Conn()
	if err != nil {
		status = http.StatusInternalServerError
		mongoDBStatus = fmt.Sprintf("failed to connect - %s", err)
	} else {
		defer conn.Close()
		err = conn.Apps().Database.Session.Ping()
		if err != nil {
			status = http.StatusInternalServerError
			mongoDBStatus = fmt.Sprintf("failed to ping - %s", err)
		}
	}
	fmt.Fprintln(&buf, mongoDBStatus)
	server, err := repository.ServerURL()
	if err != nil && err != repository.ErrGandalfDisabled {
		status = http.StatusInternalServerError
		fmt.Fprintf(&buf, "Gandalf: %s\n", err)
	} else if err == nil {
		gandalfStatus := "WORKING"
		fmt.Fprint(&buf, "Gandalf: ")
		c := gandalf.Client{Endpoint: server}
		_, err = c.GetHealthCheck()
		if err != nil {
			gandalfStatus = fmt.Sprintf("%s", err)
			status = http.StatusInternalServerError
		}
		fmt.Fprintln(&buf, gandalfStatus)
	}
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}
