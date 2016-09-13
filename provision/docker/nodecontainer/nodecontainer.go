// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision/docker/fix"
	"github.com/tsuru/tsuru/provision/nodecontainer"
)

type DockerProvisioner interface {
	GetName() string
	Cluster() *cluster.Cluster
	RegistryAuthConfig() docker.AuthConfiguration
}

// recreateContainers relaunch all node containers in the cluster for the given
// DockerProvisioner, logging progress to the given writer.
//
// It assumes that the given writer is thread safe.
func recreateContainers(p DockerProvisioner, w io.Writer, nodes ...cluster.Node) error {
	return ensureContainersStarted(p, w, true, nil, nodes...)
}

func RecreateNamedContainers(p DockerProvisioner, w io.Writer, name string, nodes ...cluster.Node) error {
	return ensureContainersStarted(p, w, true, []string{name}, nodes...)
}

func ensureContainersStarted(p DockerProvisioner, w io.Writer, relaunch bool, names []string, nodes ...cluster.Node) error {
	if w == nil {
		w = ioutil.Discard
	}
	var err error
	if len(names) == 0 {
		names, err = nodecontainer.AllNodeContainersNames()
		if err != nil {
			return err
		}
	}
	if len(nodes) == 0 {
		nodes, err = p.Cluster().UnfilteredNodes()
		if err != nil {
			return err
		}
	}
	errChan := make(chan error, len(nodes))
	wg := sync.WaitGroup{}
	log.Debugf("[node containers] recreating %d containers", len(nodes))
	recreateContainer := func(node *cluster.Node, confName string) {
		defer wg.Done()
		pool := node.Metadata["pool"]
		containerConfig, confErr := nodecontainer.LoadNodeContainer(pool, confName)
		if confErr != nil {
			errChan <- confErr
			return
		}
		if !containerConfig.Valid() {
			return
		}
		log.Debugf("[node containers] recreating container %q in %s [%s]", confName, node.Address, pool)
		fmt.Fprintf(w, "relaunching node container %q in the node %s [%s]\n", confName, node.Address, pool)
		confErr = create(containerConfig, node, pool, p, relaunch)
		if confErr != nil {
			msg := fmt.Sprintf("[node containers] failed to create container in %s [%s]: %s", node.Address, pool, confErr)
			log.Error(msg)
			errChan <- errors.New(msg)
		}
	}
	for i := range nodes {
		wg.Add(1)
		go func(node *cluster.Node) {
			defer wg.Done()
			for j := range names {
				wg.Add(1)
				go recreateContainer(node, names[j])
			}
		}(&nodes[i])
	}
	wg.Wait()
	close(errChan)
	var allErrors []string
	for err = range errChan {
		allErrors = append(allErrors, err.Error())
	}
	if len(allErrors) == 0 {
		return nil
	}
	return fmt.Errorf("multiple errors: %s", strings.Join(allErrors, ", "))
}

func pullImage(c *nodecontainer.NodeContainerConfig, client *docker.Client, p DockerProvisioner, pool string) (string, error) {
	image := c.Image()
	output, err := pullWithRetry(client, p, image, 3)
	if err != nil {
		return "", err
	}
	digest, _ := fix.GetImageDigest(output)
	err = c.PinImageIfNeeded(image, digest, pool)
	if err != nil {
		return "", err
	}
	return image, err
}

func create(c *nodecontainer.NodeContainerConfig, node *cluster.Node, poolName string, p DockerProvisioner, relaunch bool) error {
	client, err := node.Client()
	if err != nil {
		return err
	}
	c.Config.Image, err = pullImage(c, client, p, poolName)
	if err != nil {
		return err
	}
	c.Config.Env = append([]string{"DOCKER_ENDPOINT=" + node.Address}, c.Config.Env...)
	if c.Config.Labels == nil {
		c.Config.Labels = map[string]string{}
	}
	c.Config.Labels["tsuru.nodecontainer"] = strconv.FormatBool(true)
	c.Config.Labels["tsuru.node.pool"] = poolName
	c.Config.Labels["tsuru.node.address"] = node.Address
	c.Config.Labels["tsuru.node.provisioner"] = p.GetName()
	opts := docker.CreateContainerOptions{
		Name:       c.Name,
		HostConfig: &c.HostConfig,
		Config:     &c.Config,
	}
	_, err = client.CreateContainer(opts)
	if err != nil {
		if err != docker.ErrContainerAlreadyExists {
			return err
		}
		if relaunch {
			rmErr := tryRemovingOld(client, opts.Name)
			if rmErr != nil {
				log.Errorf("unable to remove old node-container: %s", rmErr)
			}
			_, err = client.CreateContainer(opts)
			if err != nil {
				return fmt.Errorf("unable to create new node-container: %s - previour rm error: %s", err, rmErr)
			}
		}
	}
	err = client.StartContainer(c.Name, nil)
	if _, ok := err.(*docker.ContainerAlreadyRunning); !ok {
		return err
	}
	return nil
}

func tryRemovingOld(client *docker.Client, id string) error {
	err := client.StopContainer(id, 10)
	if err == nil {
		err = client.RemoveContainer(docker.RemoveContainerOptions{ID: id})
	}
	for retries := 2; err != nil && retries > 0; retries-- {
		time.Sleep(time.Second)
		err = client.RemoveContainer(docker.RemoveContainerOptions{ID: id, Force: true})
	}
	return err
}

func pullWithRetry(client *docker.Client, p DockerProvisioner, image string, maxTries int) (string, error) {
	var buf bytes.Buffer
	var err error
	pullOpts := docker.PullImageOptions{Repository: image, OutputStream: &buf, InactivityTimeout: net.StreamInactivityTimeout}
	registryAuth := p.RegistryAuthConfig()
	for ; maxTries > 0; maxTries-- {
		err = client.PullImage(pullOpts, registryAuth)
		if err == nil {
			return buf.String(), nil
		}
	}
	return "", err
}

type ClusterHook struct {
	Provisioner DockerProvisioner
}

func (h *ClusterHook) RunClusterHook(evt cluster.HookEvent, node *cluster.Node) error {
	err := ensureContainersStarted(h.Provisioner, nil, false, nil, *node)
	if err != nil {
		return fmt.Errorf("unable to start node containers: %s", err)
	}
	return nil
}
