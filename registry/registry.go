// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/containerd/containerd/v2/core/remotes/docker/auth"
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

// RemoveImageUnlessShared removes an image like RemoveImageIgnoreNotFound, but
// leaves it alone, and reports shared, when another tag of its repository
// points at the same manifest. A registry deletes by digest, never by tag, and
// two builds of unchanged sources get the same digest (so does `latest`):
// deleting an old version's image would otherwise delete the image a newer
// version runs. The tags are read from the registry at the time of the call,
// so a version deployed meanwhile counts. removing lists the images the caller
// is removing as well: their tags do not hold the manifest, or a group of
// identical old versions would keep each other for good. A shared image is the
// caller's to retry.
func RemoveImageUnlessShared(ctx context.Context, imageName string, removing []string) (shared bool, err error) {
	shared, err = removeImage(ctx, imageName, true, removing)
	if err != nil {
		cause := errors.Cause(err)
		if cause != ErrDeleteDisabled && cause != ErrDigestNotFound && cause != ErrImageNotFound {
			return false, err
		}
	}
	return shared, nil
}

// RemoveImage removes an image manifest from a remote registry v2 server, returning an error
// in case of failure.
func RemoveImage(ctx context.Context, imageName string) error {
	_, err := removeImage(ctx, imageName, false, nil)
	return err
}

func removeImage(ctx context.Context, imageName string, keepShared bool, removing []string) (shared bool, err error) {
	if imageName == "" {
		return false, errors.New("invalid empty image name")
	}
	removingParts := make([][3]string, 0, len(removing))
	for _, k := range removing {
		kr, ki, kt := image.ParseImageParts(k)
		removingParts = append(removingParts, [3]string{kr, ki, kt})
	}
	registry, image, tag := image.ParseImageParts(imageName)
	if registry == "" {
		return false, errors.New("invalid empty registry")
	}
	if accountID, region, fips, ok := isECRRegistry(registry); ok {
		return false, removeECRImage(ctx, registry, accountID, region, fips, image, tag)
	}
	r := &dockerRegistry{registry: registry}
	err = r.registryAuth(ctx, imageName)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get auth for %s registry", r.registry)
	}
	digest, err := r.getDigest(ctx, image, tag)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get digest for image %s/%s:%s on registry", r.registry, image, tag)
	}
	if keepShared {
		ignore := map[string]bool{}
		for _, k := range removingParts {
			if k[0] == registry && k[1] == image {
				ignore[k[2]] = true
			}
		}
		other, err := r.otherTagWithDigest(ctx, image, tag, digest, ignore)
		if err == errTagListIncomplete {
			log.Errorf("[registry] keeping %s/%s:%s: %v", r.registry, image, tag, err)
			return true, nil
		}
		if err != nil {
			return false, errors.Wrapf(err, "failed to look for other tags of image %s/%s:%s on registry", r.registry, image, tag)
		}
		if other != "" {
			log.Debugf("[registry] keeping %s/%s:%s: tag %q points at the same manifest", r.registry, image, tag, other)
			return true, nil
		}
	}
	err = r.removeImage(ctx, image, tag, digest)
	if err != nil {
		return false, errors.Wrapf(err, "failed to remove image %s/%s:%s/%s on registry", r.registry, image, tag, digest)
	}
	return false, nil
}

// manifestMediaTypes are the manifest kinds an image may be pushed as. A
// registry answers 404 for a manifest whose kind the request does not accept,
// so leaving one out reports an existing image as missing: BuildKit pushes OCI
// manifests, and a request accepting the Docker v2 kind alone never finds them.
var manifestMediaTypes = strings.Join([]string{
	"application/vnd.docker.distribution.manifest.v2+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.oci.image.index.v1+json",
}, ", ")

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
	resp, err := r.doRequest(ctx, "HEAD", path, map[string]string{"Accept": manifestMediaTypes})
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

// errTagListIncomplete is what otherTagWithDigest returns when the registry's
// tag listing lacks the very tag being removed: the listing cannot be trusted
// to show the other tags either.
var errTagListIncomplete = errors.New("the registry's tag listing lacks the tag being removed")

// maxTagListPages bounds the pages of one tag listing, against a registry that
// keeps answering with a next page.
const maxTagListPages = 1000

// otherTagWithDigest returns a tag of image, not one of ignore, whose manifest
// is digest, or "" when there is none. tag is the one being removed.
func (r dockerRegistry) otherTagWithDigest(ctx context.Context, image, tag, digest string, ignore map[string]bool) (string, error) {
	tags, err := r.listTags(ctx, image)
	if err != nil {
		return "", err
	}
	if !slices.Contains(tags, tag) {
		return "", errTagListIncomplete
	}
	for _, t := range tags {
		if t == tag || ignore[t] {
			continue
		}
		d, err := r.getDigest(ctx, image, t)
		if err == ErrDigestNotFound {
			continue
		}
		if err != nil {
			return "", err
		}
		if d == digest {
			return t, nil
		}
	}
	return "", nil
}

// listTags returns every tag of image, following the registry's pages.
func (r dockerRegistry) listTags(ctx context.Context, image string) ([]string, error) {
	var tags []string
	path := fmt.Sprintf("/v2/%s/tags/list", image)
	for range maxTagListPages {
		page, next, err := r.listTagsPage(ctx, image, path)
		if err != nil {
			return nil, err
		}
		tags = append(tags, page...)
		if next == "" {
			return tags, nil
		}
		path = next
	}
	return nil, errors.Errorf("too many pages listing the tags of %v", image)
}

func (r dockerRegistry) listTagsPage(ctx context.Context, image, path string) (tags []string, next string, err error) {
	resp, err := r.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", errors.Errorf("invalid status listing the tags of %v: %v", image, resp.StatusCode)
	}
	var list struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, "", errors.Wrapf(err, "invalid tag list for %v", image)
	}
	return list.Tags, nextPagePath(resp.Header.Get("Link")), nil
}

// nextPagePath returns the path and query of the rel="next" target of a Link
// header, or "" when there is none. A registry links to itself, so the host of
// an absolute target is dropped.
func nextPagePath(link string) string {
	for part := range strings.SplitSeq(link, ",") {
		target, params, ok := strings.Cut(strings.TrimSpace(part), ";")
		if !ok || !strings.Contains(strings.ReplaceAll(params, " ", ""), `rel="next"`) {
			continue
		}
		u, err := url.Parse(strings.Trim(strings.TrimSpace(target), "<>"))
		// The path is appended to the registry's address, so one that does not
		// start with a slash ("@other.host/") would point the request, and its
		// credentials, at another host.
		// The check is on the text that is sent: the decoded path of "%2F@other.host/"
		// starts with a slash, its escaped form does not.
		if err != nil || u.Opaque != "" {
			return ""
		}
		next := u.RequestURI()
		if !strings.HasPrefix(next, "/") {
			return ""
		}
		return next
	}
	return ""
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
	for range maxTries {
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
	maps.Copy(req.Header, r.authHeaders)
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
		for scope := range strings.SplitSeq(scopeStr, " ") {
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
