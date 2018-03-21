// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
)

type dockerRegistry struct {
	server string
	client *http.Client
}

var (
	ErrImageNotFound  = errors.New("image not found")
	ErrDigestNotFound = errors.New("digest not found")
	ErrDeleteDisabled = errors.New("delete disabled")
)

func RemoveImageIgnoreNotFound(imageName string) error {
	err := RemoveImage(imageName)
	if err != nil {
		cause := errors.Cause(err)
		if cause != ErrDeleteDisabled && cause != ErrDigestNotFound && cause != ErrImageNotFound {
			return err
		}
		log.Debugf("ignored error removing image from registry: %v", err.Error())
	}
	return nil
}

// RemoveImage removes an image manifest from a remote registry v2 server, returning an error
// in case of failure.
func RemoveImage(imageName string) error {
	registry, image, tag := parseImage(imageName)
	if registry == "" {
		registry, _ = config.GetString("docker:registry")
	}
	if registry == "" {
		// Nothing to do if no registry is set
		return nil
	}
	if image == "" {
		return errors.Errorf("empty image after parsing %q", imageName)
	}
	r := &dockerRegistry{server: registry}
	digest, err := r.getDigest(image, tag)
	if err != nil {
		return errors.Wrapf(err, "failed to get digest for image %s/%s:%s on registry", r.server, image, tag)
	}
	err = r.removeImage(image, digest)
	if err != nil {
		return errors.Wrapf(err, "failed to remove image %s/%s:%s/%s on registry", r.server, image, tag, digest)
	}
	return nil
}

// RemoveAppImages removes all app images from a remote registry v2 server, returning an error
// in case of failure.
func RemoveAppImages(appName string) error {
	registry, _ := config.GetString("docker:registry")
	if registry == "" {
		// Nothing to do if no registry is set
		return nil
	}
	r := &dockerRegistry{server: registry}
	image := fmt.Sprintf("tsuru/app-%s", appName)
	tags, err := r.getImageTags(image)
	if err != nil {
		return err
	}
	multi := tsuruErrors.NewMultiError()
	for _, tag := range tags {
		digest, err := r.getDigest(image, tag)
		if err != nil {
			multi.Add(errors.Wrapf(err, "failed to get digest for image %s/%s:%s on registry", r.server, image, tag))
			continue
		}
		err = r.removeImage(image, digest)
		if err != nil {
			multi.Add(errors.Wrapf(err, "failed to remove image %s/%s:%s/%s on registry", r.server, image, tag, digest))
			if errors.Cause(err) == ErrDeleteDisabled {
				break
			}
		}
	}
	return multi.ToError()
}

func (r dockerRegistry) getDigest(image, tag string) (string, error) {
	path := fmt.Sprintf("/v2/%s/manifests/%s", image, tag)
	resp, err := r.doRequest("HEAD", path, map[string]string{"Accept": "application/vnd.docker.distribution.manifest.v2+json"})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", ErrDigestNotFound
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", errors.Errorf("empty digest returned for image %v:%v", image, tag)
	}
	return digest, nil
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
		return nil, errors.Errorf("image not found (%d)", resp.StatusCode)
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
		return ErrImageNotFound
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		return ErrDeleteDisabled
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := ioutil.ReadAll(resp.Body)
		return errors.Errorf("invalid status code trying to remove image (%d): %s", resp.StatusCode, string(data))
	}
	return nil
}

func (r *dockerRegistry) doRequest(method, path string, headers map[string]string) (resp *http.Response, err error) {
	u, _ := url.Parse(r.server)
	server := r.server
	if u != nil && u.Host != "" {
		server = u.Host
	}
	if r.client == nil {
		r.client = tsuruNet.Dial5Full300ClientNoKeepAlive
	}
	for _, scheme := range []string{"https", "http"} {
		endpoint := fmt.Sprintf("%s://%s%s", scheme, server, path)
		var req *http.Request
		req, err = http.NewRequest(method, endpoint, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		username, _ := config.GetString("docker:registry-auth:username")
		password, _ := config.GetString("docker:registry-auth:password")
		if len(username) > 0 || len(password) > 0 {
			req.SetBasicAuth(username, password)
		}
		resp, err = r.client.Do(req)
		if err != nil {
			if _, ok := err.(net.Error); ok {
				continue
			}
			return nil, err
		}
		return resp, nil
	}
	return nil, err
}

func parseImage(imageName string) (registry string, image string, tag string) {
	parts := strings.SplitN(imageName, "/", 3)
	switch len(parts) {
	case 1:
		image = imageName
	case 2:
		if strings.ContainsAny(parts[0], ":.") || parts[0] == "localhost" {
			registry = parts[0]
			image = parts[1]
			break
		}
		image = imageName
	case 3:
		registry = parts[0]
		image = strings.Join(parts[1:], "/")
	}
	parts = strings.SplitN(image, ":", 2)
	if len(parts) < 2 {
		return registry, parts[0], ""
	}
	return registry, parts[0], parts[1]
}
