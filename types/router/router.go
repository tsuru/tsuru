// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import "fmt"

type HealthcheckData struct {
	Path    string
	Status  int
	Body    string
	TCPOnly bool
}

func (hc *HealthcheckData) String() string {
	if hc.TCPOnly {
		return "tcp only"
	}
	status := ""
	if hc.Status != 0 {
		status = fmt.Sprintf(", status: %d", hc.Status)
	}
	path := hc.Path
	if path == "" {
		path = "/"
	}
	body := hc.Body
	if body != "" {
		body = fmt.Sprintf(", body: %q", body)
	}
	return fmt.Sprintf("path: %q%s%s", path, status, body)
}
