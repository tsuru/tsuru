// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockertest

import (
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/config"
)

// StartRegistryServer starts a testing HTTP server and sets it as the current
// registry in the tsuru configuration.
func StartRegistryServer() (rollback func()) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	repoUrl := strings.Replace(server.URL, "http://", "", 1)
	config.Set("docker:registry", repoUrl)
	return func() {
		config.Unset("docker:registry")
		server.Close()
	}
}
