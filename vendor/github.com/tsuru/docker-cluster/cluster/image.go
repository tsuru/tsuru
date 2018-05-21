// Copyright 2017 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/provision/docker/fix"
	"github.com/tsuru/tsuru/safe"
)

type ImageHistory struct {
	Node    string
	ImageId string
}

type Image struct {
	Repository string `bson:"_id"`
	LastNode   string
	LastId     string
	LastDigest string
	History    []ImageHistory
}

// RemoveImage removes an image from the nodes where this images exists,
// returning an error in case of failure. Will wait for the image to be
// removed on all nodes.
func (c *Cluster) RemoveImage(name string) error {
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
		err = n.RemoveImage(name)
		_, isNetErr := err.(net.Error)
		if err == docker.ErrConnectionRefused {
			isNetErr = true
		}
		if err != nil && err != docker.ErrNoSuchImage && !isNetErr {
			return nil, err
		}
		for _, imgId := range imgIds {
			// Errors are ignored here because we're just ensuring any
			// remaining data that wasn't removed when calling remove with the
			// image name is removed now, no big deal if there're errors here.
			n.RemoveImage(imgId)
			stor.RemoveImage(name, imgId, n.addr)
		}
		return nil, nil
	}, docker.ErrNoSuchImage, true, hosts...)
	return err
}

func parseImageRegistry(imageId string) (string, string) {
	parts := strings.SplitN(imageId, "/", 3)
	if len(parts) < 3 {
		if len(parts) == 2 && (strings.ContainsAny(parts[0], ":.") || parts[0] == "localhost") {
			return parts[0], parts[1]
		}
		return "", strings.Join(parts, "/")
	}
	return parts[0], strings.Join(parts[1:], "/")
}

// PullImage pulls an image from a remote registry server, returning an error
// in case of failure.
//
// It will pull all images in parallel, so users need to make sure that the
// given buffer is safe.
func (c *Cluster) PullImage(opts docker.PullImageOptions, auth docker.AuthConfiguration, nodes ...string) error {
	var w safe.Buffer
	if opts.OutputStream != nil {
		mw := io.MultiWriter(&w, opts.OutputStream)
		opts.OutputStream = mw
	} else {
		opts.OutputStream = &w
	}
	key := imageKey(opts.Repository, opts.Tag)
	_, err := c.runOnNodes(func(n node) (interface{}, error) {
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
	if err != nil {
		return err
	}
	digest, _ := fix.GetImageDigest(w.String())
	return c.storage().SetImageDigest(key, digest)
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

// ImageHistory returns the history of a given image
func (c *Cluster) ImageHistory(repo string) ([]docker.ImageHistory, error) {
	img, err := c.storage().RetrieveImage(repo)
	if err != nil {
		return nil, err
	}
	node, err := c.getNodeByAddr(img.LastNode)
	if err != nil {
		return nil, err
	}
	imgHistory, err := node.ImageHistory(repo)
	return imgHistory, wrapError(node, err)
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
		return errors.New("There is no docker node. Please list one in tsuru.conf or add one with `tsuru node-add`.")
	}
	nodeAddress := nodes[rand.Intn(len(nodes))].Address
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
