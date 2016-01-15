// Copyright 2016 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
)

type ImageHistory struct {
	Node    string
	ImageId string
}

type Image struct {
	Repository string `bson:"_id"`
	LastNode   string
	LastId     string
	History    []ImageHistory
}

// RemoveImageIgnoreLast works similarly to RemoveImage except it won't
// remove the last built/pulled/commited image.
func (c *Cluster) RemoveImageIgnoreLast(name string) error {
	return c.removeImage(name, true)
}

// RemoveImage removes an image from the nodes where this images exists,
// returning an error in case of failure. Will wait for the image to be
// removed on all nodes.
func (c *Cluster) RemoveImage(name string) error {
	return c.removeImage(name, false)
}

func (c *Cluster) removeImage(name string, ignoreLast bool) error {
	stor := c.storage()
	image, err := stor.RetrieveImage(name)
	if err != nil {
		return err
	}
	hosts := []string{}
	idMap := map[string][]string{}
	for _, entry := range image.History {
		_, isOld := idMap[entry.Node]
		idMap[entry.Node] = append(idMap[entry.Node], entry.ImageId)
		if !isOld {
			hosts = append(hosts, entry.Node)
		}
	}
	_, err = c.runOnNodes(func(n node) (interface{}, error) {
		imgIds, _ := idMap[n.addr]
		var lastErr error
		for _, imgId := range imgIds {
			if ignoreLast && imgId == image.LastId {
				continue
			}
			err := n.RemoveImage(imgId)
			_, isNetErr := err.(*net.OpError)
			if err == nil || err == docker.ErrNoSuchImage || isNetErr {
				err = stor.RemoveImage(name, imgId, n.addr)
				if err != nil {
					lastErr = err
				}
			} else {
				lastErr = err
			}
		}
		return nil, lastErr
	}, docker.ErrNoSuchImage, true, hosts...)
	return err
}

func parseImageRegistry(imageId string) (string, string) {
	parts := strings.SplitN(imageId, "/", 3)
	if len(parts) < 3 {
		return "", strings.Join(parts, "/")
	}
	return parts[0], strings.Join(parts[1:], "/")
}

func (c *Cluster) RemoveFromRegistry(imageId string) error {
	registryServer, imageTag := parseImageRegistry(imageId)
	if registryServer == "" {
		return nil
	}
	url := fmt.Sprintf("http://%s/v1/repositories/%s/", registryServer, imageTag)
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	request.Close = true
	rsp, err := timeout10Client.Do(request)
	if err == nil {
		rsp.Body.Close()
	}
	return err
}

// PullImage pulls an image from a remote registry server, returning an error
// in case of failure.
//
// It will pull all images in parallel, so users need to make sure that the
// given buffer is safe.
func (c *Cluster) PullImage(opts docker.PullImageOptions, auth docker.AuthConfiguration, nodes ...string) error {
	_, err := c.runOnNodes(func(n node) (interface{}, error) {
		key := imageKey(opts.Repository, opts.Tag)
		n.setPersistentClient()
		err := n.PullImage(opts, auth)
		if err != nil {
			return nil, err
		}
		img, err := n.InspectImage(key)
		if err != nil {
			return nil, err
		}
		return nil, c.storage().StoreImage(key, img.ID, n.addr)
	}, docker.ErrNoSuchImage, true, nodes...)
	return err
}

// TagImage adds a tag to the given image, returning an error in case of
// failure.
func (c *Cluster) TagImage(name string, opts docker.TagImageOptions) error {
	img, err := c.storage().RetrieveImage(name)
	if err != nil {
		return err
	}
	node, err := c.getNodeByAddr(img.LastNode)
	if err != nil {
		return err
	}
	err = node.TagImage(name, opts)
	if err != nil {
		return wrapError(node, err)
	}
	key := imageKey(opts.Repo, opts.Tag)
	return c.storage().StoreImage(key, img.LastId, node.addr)
}

// PushImage pushes an image to a remote registry server, returning an error in
// case of failure.
func (c *Cluster) PushImage(opts docker.PushImageOptions, auth docker.AuthConfiguration) error {
	key := imageKey(opts.Name, opts.Tag)
	img, err := c.storage().RetrieveImage(key)
	if err != nil {
		return err
	}
	node, err := c.getNodeByAddr(img.LastNode)
	if err != nil {
		return err
	}
	node.setPersistentClient()
	return wrapError(node, node.PushImage(opts, auth))
}

// InspectContainer inspects an image based on its repo name
func (c *Cluster) InspectImage(repo string) (*docker.Image, error) {
	img, err := c.storage().RetrieveImage(repo)
	if err != nil {
		return nil, err
	}
	node, err := c.getNodeByAddr(img.LastNode)
	if err != nil {
		return nil, err
	}
	dockerImg, err := node.InspectImage(repo)
	return dockerImg, wrapError(node, err)
}

// ListImages lists images existing in each cluster node
func (c *Cluster) ListImages(opts docker.ListImagesOptions) ([]docker.APIImages, error) {
	nodes, err := c.UnfilteredNodes()
	if err != nil {
		return nil, err
	}
	resultChan := make(chan []docker.APIImages, len(nodes))
	errChan := make(chan error, len(nodes))
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			client, err := c.getNodeByAddr(addr)
			if err != nil {
				errChan <- err
			}
			nodeImages, err := client.ListImages(opts)
			if err != nil {
				errChan <- wrapError(client, err)
			}
			resultChan <- nodeImages
		}(node.Address)
	}
	wg.Wait()
	close(resultChan)
	close(errChan)
	var allImages []docker.APIImages
	for images := range resultChan {
		allImages = append(allImages, images...)
	}
	select {
	case err := <-errChan:
		return allImages, err
	default:
	}
	return allImages, nil
}

// ImportImage imports an image from a url or stdin
func (c *Cluster) ImportImage(opts docker.ImportImageOptions) error {
	_, err := c.runOnNodes(func(n node) (interface{}, error) {
		return nil, n.ImportImage(opts)
	}, docker.ErrNoSuchImage, false)
	return err
}

//BuildImage build an image and pushes it to registry
func (c *Cluster) BuildImage(buildOptions docker.BuildImageOptions) error {
	nodes, err := c.Nodes()
	if err != nil {
		return err
	}
	if len(nodes) < 1 {
		return errors.New("There is no docker node. Please list one in tsuru.conf or add one with `tsuru docker-node-add`.")
	}
	nodeAddress := nodes[0].Address
	node, err := c.getNodeByAddr(nodeAddress)
	if err != nil {
		return err
	}
	node.setPersistentClient()
	err = node.BuildImage(buildOptions)
	if err != nil {
		return wrapError(node, err)
	}
	img, err := node.InspectImage(buildOptions.Name)
	if err != nil {
		return wrapError(node, err)
	}
	return c.storage().StoreImage(buildOptions.Name, img.ID, nodeAddress)
}

func imageKey(repo, tag string) string {
	key := repo
	if key != "" && tag != "" {
		key = fmt.Sprintf("%s:%s", key, tag)
	}
	return key
}
