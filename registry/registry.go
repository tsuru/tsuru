// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/containerd/containerd/remotes/docker/auth"
	dockerConfig "github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	dockerConfigTypes "github.com/docker/cli/cli/config/types"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/servicemanager"
)

type dockerRegistry struct {
	registry    string
	client      *http.Client
	token       string
	expires     time.Time
	authConfig  dockerConfigTypes.AuthConfig
	authHeaders http.Header
}

const defaultExpiration = 60

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
	}
	return nil
}

// RemoveImage removes an image manifest from a remote registry v2 server, returning an error
// in case of failure.
func RemoveImage(ctx context.Context, imageName string) error {
	if imageName == "" {
		return errors.New("invalid empty image name")
	}
	registry, image, tag := image.ParseImageParts(imageName)
	if registry == "" {
		return errors.New("invalid empty registry")
	}
	r := &dockerRegistry{registry: registry}
	err := r.registryAuth(ctx, imageName)
	if err != nil {
		return errors.Wrapf(err, "failed to get auth for %s registry", r.registry)
	}
	digest, err := r.getDigest(ctx, image, tag)
	if err != nil {
		return errors.Wrapf(err, "failed to get digest for image %s/%s:%s on registry", r.registry, image, tag)
	}
	err = r.removeImage(ctx, image, tag, digest)
	if err != nil {
		return errors.Wrapf(err, "failed to remove image %s/%s:%s/%s on registry", r.registry, image, tag, digest)
	}
	return nil
}

// RemoveAppImages removes all app images on all registry v2 server, returning an error
// in case of failure.
func RemoveAppImages(ctx context.Context, appName string) error {
	appVersions, err := servicemanager.AppVersion.AllAppVersions(ctx, appName)
	if err != nil {
		return err
	}
	multi := tsuruErrors.NewMultiError()
	for _, av := range appVersions {
		for _, version := range av.Versions {
			if version.BuildImage != "" {
				err := RemoveImageIgnoreNotFound(ctx, version.BuildImage)
				if err != nil {
					multi.Add(errors.Wrapf(err, "failed to remove image %s", version.BuildImage))
				}
			}
			if version.DeployImage != "" {
				err := RemoveImageIgnoreNotFound(ctx, version.DeployImage)
				if err != nil {
					multi.Add(errors.Wrapf(err, "failed to remove image %s", version.DeployImage))
				}
			}
			if version.CustomBuildTag != "" {
				err := RemoveImageIgnoreNotFound(ctx, version.CustomBuildTag)
				if err != nil {
					multi.Add(errors.Wrapf(err, "failed to remove image %s", version.CustomBuildTag))
				}
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
		data, _ := io.ReadAll(resp.Body)
		return errors.Errorf("invalid status code trying to remove image (%d): %s", resp.StatusCode, string(data))
	}
	return nil
}

func (r *dockerRegistry) doRequest(ctx context.Context, method, path string, headers map[string]string) (*http.Response, error) {
	var err error
	if r.client == nil {
		server := getServerFromRegistry(r.registry)
		r.client, err = tsuruNet.WithProxyFromConfig(*tsuruNet.Dial15Full300ClientNoKeepAlive, server)
		if err != nil {
			return nil, err
		}
	}

	maxTries := 5
	for attemptNum := 0; attemptNum < maxTries; attemptNum++ {
		for _, scheme := range []string{"https", "http"} {
			resp, err := r.attemptRequest(ctx, method, path, headers, scheme)

			if _, ok := err.(net.Error); ok {
				continue
			}

			if resp != nil && resp.StatusCode == http.StatusUnauthorized &&
				resp.Header.Get("WWW-Authenticate") != "" &&
				r.checkTokenIsValidForRenew() {

				closeRespBody(resp)

				err = r.refreshToken(ctx, resp.Header)
				if err != nil {
					return nil, err
				}

				break
			}

			if err != nil {
				return nil, err
			}
			return resp, nil
		}
	}

	return nil, errors.New("exceeded maximum request attempts")
}

func (r *dockerRegistry) attemptRequest(ctx context.Context, method, path string, headers map[string]string, scheme string) (*http.Response, error) {
	server := getServerFromRegistry(r.registry)
	endpoint := fmt.Sprintf("%s://%s%s", scheme, server, path)

	req, err := http.NewRequest(method, endpoint, nil)
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

	r.fillAuthCredentials(req)

	return r.client.Do(req)
}

func (r *dockerRegistry) refreshToken(ctx context.Context, respHeaders http.Header) error {
	to := auth.TokenOptions{
		Username: r.authConfig.Username,
		Secret:   r.authConfig.Password,
	}

	challenges := auth.ParseAuthHeader(respHeaders)
	for _, c := range challenges {
		if c.Scheme == auth.BearerAuth {
			to.Realm = c.Parameters["realm"]
			to.Service = c.Parameters["service"]
			to.Scopes = append(to.Scopes, c.Parameters["scope"])
		}
	}

	to.Scopes = parseScopes(to.Scopes).normalize()

	authResp, err := auth.FetchToken(ctx, r.client, http.Header{}, to)
	if err != nil {
		return err
	}

	if authResp.IssuedAt.IsZero() {
		authResp.IssuedAt = time.Now()
	}

	if authResp.ExpiresInSeconds == 0 {
		authResp.ExpiresInSeconds = defaultExpiration
	}

	expiry := authResp.IssuedAt.Add(time.Duration(float64(authResp.ExpiresInSeconds)*0.9) * time.Second)
	if time.Now().Before(expiry) {
		r.expires = expiry
	}

	r.token = authResp.Token
	return nil
}

func getServerFromRegistry(registry string) string {
	u, _ := url.Parse(registry)
	if u != nil && u.Host != "" {
		return u.Host
	}
	return registry
}

func closeRespBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
}

func (r *dockerRegistry) fillAuthCredentials(req *http.Request) {
	if r.token != "" && time.Now().Before(r.expires) {
		req.Header.Set("Authorization", "Bearer "+r.token)
		return
	}
	for k, v := range r.authHeaders {
		req.Header[k] = v
	}
}

func (r *dockerRegistry) checkTokenIsValidForRenew() bool {
	if r.token == "" || (r.token != "" && time.Now().After(r.expires)) {
		return true
	}
	return false
}

func (r *dockerRegistry) registryAuth(ctx context.Context, image string) error {
	var err error
	var config *configfile.ConfigFile
	clusters, clusterListErr := servicemanager.Cluster.List(ctx)
	if err != clusterListErr {
		return clusterListErr
	}
	for _, cluster := range clusters {
		clusterRegistry, registryExists := cluster.CustomData["registry"]
		dockerConfigJson, dockerConfigExists := cluster.CustomData["docker-config-json"]
		if registryExists && dockerConfigExists && strings.Contains(image, clusterRegistry) {
			config, err = dockerConfig.LoadFromReader(strings.NewReader(dockerConfigJson))
			if err != nil {
				return err
			}
		}
	}
	if config == nil {
		return nil
	}
	r.authConfig, err = config.GetAuthConfig(r.registry)
	if err != nil {
		if confAuth, ok := config.AuthConfigs[r.registry]; ok {
			r.authConfig = confAuth
		} else {
			return fmt.Errorf("failed to get auth config for registry %s: %v", r.registry, err)
		}
	}
	if r.authHeaders == nil {
		r.authHeaders = http.Header{}
	}
	if r.authConfig.RegistryToken != "" {
		r.authHeaders.Set("Authorization", "Bearer "+r.authConfig.RegistryToken)
	} else if r.authConfig.Username != "" || r.authConfig.Password != "" {
		basic := base64.StdEncoding.EncodeToString([]byte(r.authConfig.Username + ":" + r.authConfig.Password))
		r.authHeaders.Set("Authorization", "Basic "+basic)
	}
	return nil
}

type scopes map[string]map[string]struct{}

func parseScopes(s []string) scopes {
	// https://docs.docker.com/registry/spec/auth/scope/
	m := map[string]map[string]struct{}{}
	for _, scopeStr := range s {
		if scopeStr == "" {
			return nil
		}
		// The scopeStr may have strings that contain multiple scopes separated by a space.
		for _, scope := range strings.Split(scopeStr, " ") {
			parts := strings.SplitN(scope, ":", 3)
			names := []string{parts[0]}
			if len(parts) > 1 {
				names = append(names, parts[1])
			}
			var actions []string
			if len(parts) == 3 {
				actions = append(actions, strings.Split(parts[2], ",")...)
			}
			name := strings.Join(names, ":")
			ma, ok := m[name]
			if !ok {
				ma = map[string]struct{}{}
				m[name] = ma
			}

			for _, a := range actions {
				ma[a] = struct{}{}
			}
		}
	}
	return m
}

func (s scopes) normalize() []string {
	names := make([]string, 0, len(s))
	for n := range s {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]string, 0, len(s))

	for _, n := range names {
		actions := make([]string, 0, len(s[n]))
		for a := range s[n] {
			actions = append(actions, a)
		}
		sort.Strings(actions)

		out = append(out, n+":"+strings.Join(actions, ","))
	}
	return out
}
