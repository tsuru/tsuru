// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"net"
	"net/http"
	"time"

	"github.com/fsouza/go-dockerclient"
)

func newClient(address string) (*docker.Client, error) {
	client, err := docker.NewClient(address)
	if err != nil {
		return nil, err
	}
	dialTimeout := 5 * time.Second
	fullTimeout := time.Minute
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}
	transport := http.Transport{
		Dial:                dialer.Dial,
		TLSHandshakeTimeout: dialTimeout,
		MaxIdleConnsPerHost: -1,
		DisableKeepAlives:   true,
		TLSClientConfig:     swarmConfig.tlsConfig,
	}
	httpClient := &http.Client{
		Transport: &transport,
		Timeout:   fullTimeout,
	}
	client.HTTPClient = httpClient
	client.Dialer = dialer
	client.TLSConfig = swarmConfig.tlsConfig
	return client, nil
}
