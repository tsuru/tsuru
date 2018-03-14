// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision"
)

func toHealthConfig(hc provision.TsuruYamlHealthcheck, port int) *container.HealthConfig {
	path := hc.Path
	method := hc.Method
	match := hc.Match
	status := hc.Status
	scheme := hc.Scheme
	if scheme == "" {
		scheme = provision.DefaultHealthcheckScheme
	}
	allowedFailures := hc.AllowedFailures
	if path == "" {
		return nil
	}
	path = strings.TrimSpace(strings.TrimLeft(path, "/"))
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)
	if status == 0 && match == "" {
		status = 200
	}
	maxWaitTime, _ := config.GetInt("docker:healthcheck:max-time")
	if maxWaitTime == 0 {
		maxWaitTime = 120
	}
	curlLine := fmt.Sprintf("curl -k -X%s -fsSL %s://localhost:%d/%s", method, scheme, port, strings.TrimPrefix(path, "/"))
	if match != "" {
		curlLine = fmt.Sprintf("%s | egrep %q", curlLine, match)
	} else {
		curlLine = fmt.Sprintf("%s -o/dev/null -w '%%{http_code}' | grep %d", curlLine, status)
	}
	return &container.HealthConfig{
		Interval: 3 * time.Second,
		Retries:  allowedFailures + 1,
		Timeout:  time.Duration(maxWaitTime) * time.Second,
		Test: []string{
			"CMD-SHELL",
			curlLine,
		},
	}
}
