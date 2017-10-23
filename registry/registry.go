// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/net"
)

type dockerRegistry struct {
	server string
}

type StorageDeleteDisabledError struct {
	StatusCode int
}

func (e *StorageDeleteDisabledError) Error() string {
	return fmt.Sprintf("storage delete is disabled (%d)", e.StatusCode)
}

// RemoveImage removes an image manifest from a remote registry v2 server, returning an error
// in case of failure.
func RemoveImage(imageName string) error {
	registry, image, tag := parseImage(imageName)
	if registry == "" {
		var err error
		registry, err = config.GetString("docker:registry")
		if err != nil {
			return err
		}
	}
	r := &dockerRegistry{server: registry}
	digest, err := r.getDigest(image, tag)
	if err != nil {
		return fmt.Errorf("failed to get digest for image %s/%s:%s on registry: %v\n", r.server, image, tag, err)
	}
	err = r.removeImage(image, digest)
	if err != nil {
		if err, ok := err.(*StorageDeleteDisabledError); ok {
			return err
		}
		return fmt.Errorf("failed to remove image %s/%s:%s/%s on registry: %v\n", r.server, image, tag, digest, err)
	}
	return nil
}

// RemoveAppImages removes all app images from a remote registry v2 server, returning an error
// in case of failure.
func RemoveAppImages(appName string) error {
	registry, err := config.GetString("docker:registry")
	if err != nil {
		return err
	}
	r := &dockerRegistry{server: registry}
	image := fmt.Sprintf("tsuru/app-%s", appName)
	tags, err := r.getImageTags(image)
	if err != nil {
		return err
	}
	for _, tag := range tags {
		digest, err := r.getDigest(image, tag)
		if err != nil {
			fmt.Printf("failed to get digest for image %s/%s:%s on registry: %v\n", r.server, image, tag, err)
			continue
		}
		err = r.removeImage(image, digest)
		if err != nil {
			if err, ok := err.(*StorageDeleteDisabledError); ok {
				return err
			}
			fmt.Printf("failed to remove image %s/%s:%s/%s on registry: %v\n", r.server, image, tag, digest, err)
		}
	}
	return nil
}

func (r dockerRegistry) getDigest(image, tag string) (string, error) {
	path := fmt.Sprintf("/v2/%s/manifests/%s", image, tag)
	resp, err := r.doRequest("HEAD", path, map[string]string{"Accept": "application/vnd.docker.distribution.manifest.v2+json"})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest {
		return "", fmt.Errorf("manifest not found (%d)", resp.StatusCode)
	}
	return resp.Header.Get("Docker-Content-Digest"), nil
}

type imageTags struct {
	Name string
	Tags []string
}

func (r dockerRegistry) getImageTags(image string) ([]string, error) {
	path := fmt.Sprintf("/v2/%s/tags/list", image)
	resp, err := r.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest {
		return nil, fmt.Errorf("image not found (%d)", resp.StatusCode)
	}
	var it imageTags
	if err := json.NewDecoder(resp.Body).Decode(&it); err != nil {
		return nil, err
	}
	return it.Tags, nil
}

func (r dockerRegistry) removeImage(image, digest string) error {
	path := fmt.Sprintf("/v2/%s/manifests/%s", image, digest)
	resp, err := r.doRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("repository not found (%d)", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		return &StorageDeleteDisabledError{resp.StatusCode}
	}
	return nil
}

func (r *dockerRegistry) doRequest(method, path string, headers map[string]string) (*http.Response, error) {
	endpoint := fmt.Sprintf("http://%s%s", r.server, path)
	req, err := http.NewRequest(method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := net.Dial5Full300ClientNoKeepAlive.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func parseImage(imageName string) (registry string, image string, tag string) {
	parts := strings.SplitN(imageName, "/", 3)
	if len(parts) < 3 {
		image = imageName
	} else {
		registry = parts[0]
		image = strings.Join(parts[1:], "/")
	}
	parts = strings.SplitN(image, ":", 2)
	image = parts[0]
	tag = parts[1]
	return
}
