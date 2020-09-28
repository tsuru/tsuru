// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"context"
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

func healthCheckDockerRegistry(ctx context.Context) error {
	err := pingDockerRegistry(ctx, "https")
	if err != nil {
		return pingDockerRegistry(ctx, "http")
	}
	return nil
}

func pingDockerRegistry(ctx context.Context, scheme string) error {
	registry, _ := config.GetString("docker:registry")
	if registry == "" {
		return hc.ErrDisabledComponent
	}
	registry = httpRegexp.ReplaceAllString(registry, "")
	registry = fmt.Sprintf("%s://%s", scheme, strings.TrimRight(registry, "/"))
	v1URL := registry + "/v1/_ping"
	v2URL := registry + "/v2/"
	client := tsuruNet.Dial15Full60ClientNoKeepAlive
	req, err := newRequestWithCredentials(http.MethodGet, v2URL)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		req, err = newRequestWithCredentials(http.MethodGet, v1URL)
		if err != nil {
			return err
		}
		resp, err = client.Do(req)
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

func newRequestWithCredentials(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	username, _ := config.GetString("docker:registry-auth:username")
	password, _ := config.GetString("docker:registry-auth:password")
	if len(username) != 0 || len(password) != 0 {
		req.SetBasicAuth(username, password)
	}
	return req, nil
}

func healthCheckDocker(ctx context.Context) error {
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
