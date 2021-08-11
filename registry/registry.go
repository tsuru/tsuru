// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
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

func RemoveImageIgnoreNotFound(ctx context.Context, imageName string) error {
	err := RemoveImage(ctx, imageName)
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
func RemoveImage(ctx context.Context, imageName string) error {
	registry, image, tag := image.ParseImageParts(imageName)
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
	digest, err := r.getDigest(ctx, image, tag)
	if err != nil {
		return errors.Wrapf(err, "failed to get digest for image %s/%s:%s on registry", r.server, image, tag)
	}
	err = r.removeImage(ctx, image, tag, digest)
	if err != nil {
		return errors.Wrapf(err, "failed to remove image %s/%s:%s/%s on registry", r.server, image, tag, digest)
	}
	return nil
}

// RemoveAppImages removes all app images from a remote registry v2 server, returning an error
// in case of failure.
func RemoveAppImages(ctx context.Context, appName string) error {
	registry, _ := config.GetString("docker:registry")
	if registry == "" {
		// Nothing to do if no registry is set
		return nil
	}
	r := &dockerRegistry{server: registry}
	image := fmt.Sprintf("tsuru/app-%s", appName)
	tags, err := r.getImageTags(ctx, image)
	if err != nil {
		return err
	}
	multi := tsuruErrors.NewMultiError()
	for _, tag := range tags {
		digest, err := r.getDigest(ctx, image, tag)
		if err != nil {
			multi.Add(errors.Wrapf(err, "failed to get digest for image %s/%s:%s on registry", r.server, image, tag))
			continue
		}
		err = r.removeImage(ctx, image, tag, digest)
		if err != nil {
			multi.Add(errors.Wrapf(err, "failed to remove image %s/%s:%s/%s on registry", r.server, image, tag, digest))
			if errors.Cause(err) == ErrDeleteDisabled {
				break
			}
		}
	}
	return multi.ToError()
}

func (r dockerRegistry) getDigest(ctx context.Context, image, tag string) (string, error) {
	path := fmt.Sprintf("/v2/%s/manifests/%s", image, tag)
	resp, err := r.doRequest(ctx, "HEAD", path, map[string]string{"Accept": "application/vnd.docker.distribution.manifest.v2+json"})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", ErrDigestNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.Errorf("invalid status reading manifest for %v:%v: %v", image, tag, resp.StatusCode)
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

func (r dockerRegistry) getImageTags(ctx context.Context, image string) ([]string, error) {
	path := fmt.Sprintf("/v2/%s/tags/list", image)
	resp, err := r.doRequest(ctx, "GET", path, nil)
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

func (r dockerRegistry) removeImage(ctx context.Context, image, tag, digest string) error {
	// GCR/GAR registries implementation do not completely follow docker
	// registry spec. They require the image tag to be deleted prior to
	// deleting the manifest. Here we try deleting the tag first and then
	// proceed to delete the digest regardless of errors in the previous step.
	tagPath := fmt.Sprintf("/v2/%s/manifests/%s", image, tag)
	err := r.removeImagePath(ctx, tagPath)
	if err != nil {
		log.Errorf("ignored error trying to delete tag from registry %q: %v", tagPath, err)
	}
	return r.removeImagePath(ctx, fmt.Sprintf("/v2/%s/manifests/%s", image, digest))
}

func (r dockerRegistry) removeImagePath(ctx context.Context, path string) error {
	resp, err := r.doRequest(ctx, "DELETE", path, nil)
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

func (r *dockerRegistry) doRequest(ctx context.Context, method, path string, headers map[string]string) (resp *http.Response, err error) {
	u, _ := url.Parse(r.server)
	server := r.server
	if u != nil && u.Host != "" {
		server = u.Host
	}
	authHeaders := registryAuth(server)
	if r.client == nil {
		r.client, err = tsuruNet.WithProxyFromConfig(*tsuruNet.Dial15Full300ClientNoKeepAlive, server)
		if err != nil {
			return nil, err
		}
	}
	for _, scheme := range []string{"https", "http"} {
		endpoint := fmt.Sprintf("%s://%s%s", scheme, server, path)
		var req *http.Request
		req, err = http.NewRequest(method, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if ctx != nil {
			req = req.WithContext(ctx)
		}
		req.Header = http.Header{}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		for k, v := range authHeaders {
			req.Header[k] = v
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

func registryAuth(registry string) http.Header {
	authConfig, err := docker.NewAuthConfigurationsFromCredsHelpers(registry)
	if err != nil {
		configs, err := docker.NewAuthConfigurationsFromDockerCfg()
		if err == nil {
			if config, ok := configs.Configs[registry]; ok {
				authConfig = &config
			}
		}
	}
	if authConfig == nil {
		username, _ := config.GetString("docker:registry-auth:username")
		password, _ := config.GetString("docker:registry-auth:password")
		authConfig = &docker.AuthConfiguration{
			Username: username,
			Password: password,
		}
	}

	headers := http.Header{}
	if *authConfig == (docker.AuthConfiguration{}) {
		return headers
	}

	if authConfig.RegistryToken != "" {
		headers.Set("Authorization", "Bearer "+authConfig.RegistryToken)
	} else if authConfig.Username != "" || authConfig.Password != "" {
		basic := base64.StdEncoding.EncodeToString([]byte(authConfig.Username + ":" + authConfig.Password))
		headers.Set("Authorization", "Basic "+basic)
	}

	return headers
}
