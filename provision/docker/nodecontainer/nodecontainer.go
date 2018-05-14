// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/fix"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/provision/nodecontainer"
)

type DockerProvisioner interface {
	GetName() string
	Cluster() *cluster.Cluster
}

// recreateContainers relaunch all node containers in the cluster for the given
// DockerProvisioner, logging progress to the given writer.
//
// It assumes that the given writer is thread safe.
func recreateContainers(p DockerProvisioner, w io.Writer, nodes ...cluster.Node) error {
	return ensureContainersStarted(p, w, true, nil, nodes...)
}

func RecreateNamedContainers(p DockerProvisioner, w io.Writer, name string, pool string) error {
	var nodes []cluster.Node
	var err error
	if pool == "" {
		nodes, err = p.Cluster().UnfilteredNodes()
	} else {
		nodes, err = p.Cluster().UnfilteredNodesForMetadata(map[string]string{provision.PoolMetadataName: pool})
	}
	if err != nil || len(nodes) == 0 {
		return err
	}
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
	workers, _ := config.GetInt("docker:nodecontainer:max-workers")
	if workers == 0 {
		workers = len(nodes)
	}
	step := len(nodes) / workers
	if len(nodes)%workers != 0 {
		step++
	}
	errChan := make(chan error, len(nodes)*len(names))
	log.Debugf("[node containers] recreating %d containers", len(nodes)*len(names))
	recreateContainer := func(node *cluster.Node, confName string) {
		pool := node.Metadata[provision.PoolMetadataName]
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
			confErr = errors.Wrapf(confErr, "[node containers] failed to create container in %s [%s]", node.Address, pool)
			errChan <- log.WrapError(confErr)
		}
	}
	wg := sync.WaitGroup{}
	splitWork := func(start, end int) {
		for i := start; i < end; i++ {
			for j := range names {
				recreateContainer(&nodes[i], names[j])
			}
		}
		wg.Done()
	}
	for i := 0; i < len(nodes); i += step {
		end := i + step
		if end > len(nodes) {
			end = len(nodes)
		}
		wg.Add(1)
		go splitWork(i, end)
	}
	wg.Wait()
	close(errChan)
	var allErrors []error
	for err = range errChan {
		allErrors = append(allErrors, err)
	}
	if len(allErrors) == 0 {
		return nil
	}
	return tsuruErrors.NewMultiError(allErrors...)
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
	c.Config.Labels = provision.NodeContainerLabels(provision.NodeContainerLabelsOpts{
		Name:         c.Name,
		CustomLabels: c.Config.Labels,
		Pool:         poolName,
		Provisioner:  p.GetName(),
	}).ToLabels()
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
			multiErr := tsuruErrors.NewMultiError()
			err = tryRemovingOld(client, opts.Name)
			if err != nil {
				multiErr.Add(errors.Wrapf(err, "unable to remove old node-container"))
			}
			_, err = client.CreateContainer(opts)
			if err != nil {
				multiErr.Add(errors.Wrapf(err, "unable to create new node-container"))
				return multiErr
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
	registryAuth := dockercommon.RegistryAuthConfig(image)
	for ; maxTries > 0; maxTries-- {
		err = client.PullImage(pullOpts, registryAuth)
		if err == nil {
			return buf.String(), nil
		}
	}
	return "", err
}

func RemoveNamedContainers(p DockerProvisioner, w io.Writer, name string, pool string) error {
	var nodes []cluster.Node
	var err error
	if pool == "" {
		nodes, err = p.Cluster().UnfilteredNodes()
	} else {
		nodes, err = p.Cluster().UnfilteredNodesForMetadata(map[string]string{provision.PoolMetadataName: pool})
	}
	if err != nil {
		return errors.WithStack(err)
	}
	errChan := make(chan error, len(nodes))
	wg := sync.WaitGroup{}
	removeContainer := func(node *cluster.Node) {
		pool := node.Metadata[provision.PoolMetadataName]
		client, err := node.Client()
		if err != nil {
			errChan <- err
			return
		}
		err = client.StopContainer(name, 10)
		if err != nil {
			if _, ok := err.(*docker.NoSuchContainer); ok {
				log.Debugf("[node containers] no such container %q in %s [%s]", name, node.Address, pool)
				fmt.Fprintf(w, "no such node container %q in the node %s [%s]\n", name, node.Address, pool)
				return
			}
			if _, ok := err.(*docker.ContainerNotRunning); !ok {
				err = errors.Wrapf(err, "[node containers] failed to stop container in %s [%s]", node.Address, pool)
				errChan <- err
				return
			}
		}
		log.Debugf("[node containers] removing container %q in %s [%s]", name, node.Address, pool)
		fmt.Fprintf(w, "removing node container %q in the node %s [%s]\n", name, node.Address, pool)
		err = client.RemoveContainer(docker.RemoveContainerOptions{ID: name, Force: true})
		if err != nil {
			err = errors.Wrapf(err, "[node containers] failed to remove container in %s [%s]", node.Address, pool)
			errChan <- err
		}
	}
	for i := range nodes {
		wg.Add(1)
		go func(node *cluster.Node) {
			removeContainer(node)
			wg.Done()
		}(&nodes[i])
	}
	wg.Wait()
	close(errChan)
	var allErrors []error
	for err := range errChan {
		allErrors = append(allErrors, err)
	}
	if len(allErrors) == 0 {
		return nil
	}
	return tsuruErrors.NewMultiError(allErrors...)
}

type ClusterHook struct {
	Provisioner DockerProvisioner
}

func (h *ClusterHook) RunClusterHook(evt cluster.HookEvent, node *cluster.Node) error {
	err := ensureContainersStarted(h.Provisioner, nil, false, nil, *node)
	if err != nil {
		return errors.Wrap(err, "unable to start node containers")
	}
	return nil
}
