// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/docker/engine-api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

const (
	dockerDialTimeout  = 5 * time.Second
	dockerFullTimeout  = time.Minute
	dockerTCPKeepALive = 30 * time.Second
)

func newClient(address string) (*docker.Client, error) {
	client, err := docker.NewClient(address)
	if err != nil {
		return nil, errors.Wrap(err, "")
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

func initSwarm(client *docker.Client, host string) error {
	_, err := client.InitSwarm(docker.InitSwarmOptions{
		InitRequest: swarm.InitRequest{
			ListenAddr:    fmt.Sprintf("0.0.0.0:%d", swarmConfig.swarmPort),
			AdvertiseAddr: host,
		},
	})
	if err != nil && errors.Cause(err) != docker.ErrNodeAlreadyInSwarm {
		return errors.Wrap(err, "")
	}
	return nil
}

func joinSwarm(existingClient *docker.Client, newClient *docker.Client, host string) error {
	swarmInfo, err := existingClient.InspectSwarm(nil)
	if err != nil {
		return errors.Wrap(err, "")
	}
	dockerInfo, err := existingClient.Info()
	if err != nil {
		return errors.Wrap(err, "")
	}
	if len(dockerInfo.Swarm.RemoteManagers) == 0 {
		return errors.Errorf("no remote managers found in node %#v", dockerInfo)
	}
	addrs := make([]string, len(dockerInfo.Swarm.RemoteManagers))
	for i, peer := range dockerInfo.Swarm.RemoteManagers {
		addrs[i] = peer.Addr
	}
	opts := docker.JoinSwarmOptions{
		JoinRequest: swarm.JoinRequest{
			ListenAddr:    fmt.Sprintf("0.0.0.0:%d", swarmConfig.swarmPort),
			AdvertiseAddr: host,
			JoinToken:     swarmInfo.JoinTokens.Manager,
			RemoteAddrs:   addrs,
		},
	}
	err = newClient.JoinSwarm(opts)
	if err != nil {
		return errors.Wrap(err, "")
	}
	return nil
}
