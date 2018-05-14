// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon

import (
	"io"
	"strings"
	"time"

	"context"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/safe"
)

const (
	JsonFileLogDriver = "json-file"
)

type PullAndCreateClient struct {
	*docker.Client
}

var _ provision.BuilderDockerClient = &PullAndCreateClient{}
var _ provision.ExecDockerClient = &PullAndCreateClient{}

func (c *PullAndCreateClient) SetTimeout(timeout time.Duration) {
	c.Client.HTTPClient.Timeout = timeout
}

func (c *PullAndCreateClient) PullAndCreateContainer(opts docker.CreateContainerOptions, w io.Writer) (*docker.Container, string, error) {
	if w != nil {
		w = &tsuruIo.DockerErrorCheckWriter{W: w}
	}
	pullOpts := docker.PullImageOptions{
		Repository:        opts.Config.Image,
		OutputStream:      w,
		InactivityTimeout: tsuruNet.StreamInactivityTimeout,
		RawJSONStream:     true,
	}
	err := c.Client.PullImage(pullOpts, RegistryAuthConfig(opts.Config.Image))
	if err != nil {
		return nil, "", err
	}
	cont, err := c.Client.CreateContainer(opts)
	return cont, net.URLToHost(c.Client.Endpoint()), err
}

type Client interface {
	PushImage(docker.PushImageOptions, docker.AuthConfiguration) error
	InspectImage(string) (*docker.Image, error)
	TagImage(string, docker.TagImageOptions) error
	CommitContainer(docker.CommitContainerOptions) (*docker.Image, error)
	DownloadFromContainer(string, docker.DownloadFromContainerOptions) error
}

func DownloadFromContainer(client Client, contID string, filePath string) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		downloadOpts := docker.DownloadFromContainerOptions{
			OutputStream: writer,
			Path:         filePath,
		}
		err := client.DownloadFromContainer(contID, downloadOpts)
		if err != nil {
			writer.CloseWithError(err)
		} else {
			writer.Close()
		}
	}()
	return reader, nil
}

func WaitDocker(client *docker.Client) error {
	timeout, _ := config.GetInt("docker:api-timeout")
	if timeout == 0 {
		timeout = 600
	}
	timeoutChan := time.After(time.Duration(timeout) * time.Second)
	pong := make(chan error, 1)
	exit := make(chan struct{})
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := client.PingWithContext(ctx)
			cancel()
			if err == nil {
				pong <- nil
				return
			}
			if e, ok := err.(*docker.Error); ok && e.Status > 499 {
				pong <- err
				return
			}
			select {
			case <-exit:
				return
			case <-time.After(time.Second):
			}
		}
	}()
	select {
	case err := <-pong:
		return err
	case <-timeoutChan:
		close(exit)
		return errors.Errorf("Docker API at %q didn't respond after %d seconds", client.Endpoint(), timeout)
	}
}

func PushImage(client Client, name, tag string, authconfig docker.AuthConfiguration) error {
	if _, err := config.GetString("docker:registry"); err == nil {
		var buf safe.Buffer
		pushOpts := docker.PushImageOptions{
			Name:              name,
			Tag:               tag,
			OutputStream:      &buf,
			InactivityTimeout: net.StreamInactivityTimeout,
		}
		if authconfig == (docker.AuthConfiguration{}) {
			authconfig = RegistryAuthConfig(name)
		}
		err = client.PushImage(pushOpts, authconfig)
		if err != nil {
			log.Errorf("[docker] Failed to push image %q (%s): %s", name, err, buf.String())
			return err
		}
	}
	return nil
}

func RegistryAuthConfig(image string) docker.AuthConfiguration {
	var authConfig docker.AuthConfiguration
	addr, _ := config.GetString("docker:registry")
	if !strings.HasPrefix(image, addr) {
		return authConfig
	}
	authConfig.ServerAddress = addr
	authConfig.Email, _ = config.GetString("docker:registry-auth:email")
	authConfig.Username, _ = config.GetString("docker:registry-auth:username")
	authConfig.Password, _ = config.GetString("docker:registry-auth:password")
	return authConfig
}

func GetNodeByHost(c *cluster.Cluster, host string) (cluster.Node, error) {
	nodes, err := c.UnfilteredNodes()
	if err != nil {
		return cluster.Node{}, err
	}
	for _, node := range nodes {
		if net.URLToHost(node.Address) == host {
			return node, nil
		}
	}
	return cluster.Node{}, errors.Errorf("node with host %q not found", host)
}
