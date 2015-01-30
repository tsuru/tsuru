// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/hc"
)

func healthcheck(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("check") == "all" {
		fullHealthcheck(w, r)
		return
	}
	w.Write([]byte(hc.HealthCheckOK))
}

func fullHealthcheck(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	results := hc.Check()
	status := http.StatusOK
	for _, result := range results {
		fmt.Fprintf(&buf, "%s: %s\n", result.Name, result.Status)
		if result.Status != hc.HealthCheckOK {
			status = http.StatusInternalServerError
		}
	}
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}
