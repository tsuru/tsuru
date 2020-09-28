// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/hc"
)

// title: healthcheck
// path: /healthcheck
// method: GET
// responses:
//   200: OK
//   500: Internal server error
func healthcheck(w http.ResponseWriter, r *http.Request) {
	var checks []string
	values := r.URL.Query()
	if values != nil {
		checks = values["check"]
	}
	fullHealthcheck(r.Context(), w, checks)
}

func fullHealthcheck(ctx context.Context, w http.ResponseWriter, checks []string) {
	var buf bytes.Buffer
	results := hc.Check(ctx, checks...)
	if len(results) == 0 {
		w.Write([]byte(hc.HealthCheckOK))
		return
	}
	status := http.StatusOK
	for _, result := range results {
		fmt.Fprintf(&buf, "%s: %s (%s)\n", result.Name, result.Status, result.Duration)
		if result.Status != hc.HealthCheckOK {
			status = http.StatusInternalServerError
		}
	}
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}
