// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
	tsuruNet "github.com/tsuru/tsuru/net"
)

var httpRegexp = regexp.MustCompile(`^https?://`)

func init() {
	hc.AddChecker("docker-registry", healthCheckDockerRegistry)
	hc.AddChecker("docker", healthCheckDocker)
}

func healthCheckDockerRegistry() error {
	err := pingDockerRegistry("https")
	if err != nil {
		return pingDockerRegistry("http")
	}
	return nil
}

func pingDockerRegistry(scheme string) error {
	registry, _ := config.GetString("docker:registry")
	if registry == "" {
		return hc.ErrDisabledComponent
	}
	registry = httpRegexp.ReplaceAllString(registry, "")
	registry = fmt.Sprintf("%s://%s", scheme, strings.TrimRight(registry, "/"))
	v1URL := registry + "/v1/_ping"
	v2URL := registry + "/v2/"
	resp, err := tsuruNet.Dial5Full60ClientNoKeepAlive.Get(v2URL)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		resp, err = tsuruNet.Dial5Full60ClientNoKeepAlive.Get(v1URL)
		if err != nil {
			return err
		}
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("unexpected status - %s", body)
	}
	return nil
}

func healthCheckDocker() error {
	nodes, err := mainDockerProvisioner.Cluster().Nodes()
	if err != nil {
		return err
	}
	if len(nodes) < 1 {
		return errors.New("error - no nodes available for running containers")
	}
	if len(nodes) > 1 {
		return hc.ErrDisabledComponent
	}
	client, err := nodes[0].Client()
	if err != nil {
		return err
	}
	err = client.Ping()
	if err != nil {
		return errors.Wrap(err, "ping failed")
	}
	return nil
}
