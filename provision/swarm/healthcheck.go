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

func toHealthConfig(meta provision.TsuruYamlData, port int) *container.HealthConfig {
	hc := meta.Healthcheck
	path := hc.Path
	method := hc.Method
	match := hc.Match
	status := hc.Status
	scheme := hc.Scheme
	if scheme == "" {
		scheme = provision.DefaultHealthcheckScheme
	}
	allowedFailures := hc.AllowedFailures
	maxWaitTime, _ := config.GetInt("docker:healthcheck:max-time")
	if maxWaitTime == 0 {
		maxWaitTime = 120
	}
	var cmdLine string
	if path != "" {
		path = strings.TrimSpace(strings.TrimLeft(path, "/"))
		if method == "" {
			method = "GET"
		}
		method = strings.ToUpper(method)
		if status == 0 && match == "" {
			status = 200
		}
		cmdLine = fmt.Sprintf("curl -k -X%s -fsSL %s://localhost:%d/%s", method, scheme, port, strings.TrimPrefix(path, "/"))
		if match != "" {
			cmdLine = fmt.Sprintf("%s | egrep %q", cmdLine, match)
		} else {
			cmdLine = fmt.Sprintf("%s -o/dev/null -w '%%{http_code}' | grep %d", cmdLine, status)
		}
	}
	if len(meta.Hooks.Restart.After) > 0 {
		restartHooks := fmt.Sprintf(`if [ ! -f %[1]s ]; then %[2]s && touch %[1]s; fi`,
			"/tmp/restartafterok",
			strings.Join(meta.Hooks.Restart.After, " && "),
		)
		if cmdLine == "" {
			cmdLine = restartHooks
		} else {
			cmdLine = fmt.Sprintf("%s && %s", cmdLine, restartHooks)
		}
	}
	if cmdLine == "" {
		return nil
	}
	return &container.HealthConfig{
		Interval: 3 * time.Second,
		Retries:  allowedFailures + 1,
		Timeout:  time.Duration(maxWaitTime) * time.Second,
		Test: []string{
			"CMD-SHELL",
			cmdLine,
		},
	}
}
