// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
)

var httpRegexp = regexp.MustCompile(`^https?://`)

func init() {
	hc.AddChecker("docker-registry", healthCheckDockerRegistry)
}

func healthCheckDockerRegistry() error {
	registry, _ := config.GetString("docker:registry")
	if registry == "" {
		return hc.ErrDisabledComponent
	}
	if !httpRegexp.MatchString(registry) {
		registry = "http://" + registry
	}
	registry = strings.TrimRight(registry, "/")
	url := registry + "/v1/_ping"
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status - %s", body)
	}
	return nil
}
