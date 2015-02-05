// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repositorytest

import (
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/gandalftest"
)

// StartGandalfTestServer starts a new HTTP server, and sets the value of
// git:api-server configuration entry.
func StartGandalfTestServer(h http.Handler) *httptest.Server {
	ts := gandalftest.TestServer(h)
	config.Set("git:api-server", ts.URL)
	return ts
}
