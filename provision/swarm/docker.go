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

const (
	dockerDialTimeout  = 5 * time.Second
	dockerFullTimeout  = time.Minute
	dockerTCPKeepALive = 30 * time.Second
)

func newClient(address string) (*docker.Client, error) {
	client, err := docker.NewClient(address)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{
		Timeout:   dockerDialTimeout,
		KeepAlive: dockerTCPKeepALive,
	}
	transport := http.Transport{
		Dial:                dialer.Dial,
		TLSHandshakeTimeout: dockerDialTimeout,
		TLSClientConfig:     swarmConfig.tlsConfig,
		// No connection pooling so that we have reliable dial timeouts. Slower
		// but safer.
		DisableKeepAlives:   true,
		MaxIdleConnsPerHost: -1,
	}
	httpClient := &http.Client{
		Transport: &transport,
		Timeout:   dockerFullTimeout,
	}
	client.HTTPClient = httpClient
	client.Dialer = dialer
	client.TLSConfig = swarmConfig.tlsConfig
	return client, nil
}
