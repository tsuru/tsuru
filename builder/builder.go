// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"

	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var (
	DefaultBuilder = "docker"

	ErrBuildV2NotSupported = errors.New("build v2 not supported")
)

type BuildOpts struct {
	IsTsuruBuilderImage bool
	Rebuild             bool
	Redeploy            bool
	ArchiveURL          string
	ArchiveFile         io.Reader
	ArchiveSize         int64
	ImageID             string
	Tag                 string
	Message             string
	Output              io.Writer
	Dockerfile          string
}

type BuilderV2 interface {
	BuildV2(ctx context.Context, app provision.App, evt *event.Event, opts BuildOpts) (appTypes.AppVersion, error)
}

// Builder is the basic interface of this package.
type Builder interface {
	Build(ctx context.Context, p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *BuildOpts) (appTypes.AppVersion, error)
}

var builders = make(map[string]Builder)

type PlatformBuilderV2 interface {
	PlatformBuildV2(context.Context, appTypes.PlatformOptions) ([]string, error)
}

// PlatformBuilder is a builder where administrators can manage
// platforms (automatically adding, removing and updating platforms).
type PlatformBuilder interface {
	PlatformBuild(context.Context, appTypes.PlatformOptions) ([]string, error)
	PlatformRemove(ctx context.Context, name string) error
}

// Register registers a new builder in the Builder registry.
func Register(name string, builder Builder) {
	builders[name] = builder
}

func List() map[string]Builder {
	bs := make(map[string]Builder)
	for name, b := range builders {
		bs[name] = b
	}

	return bs
}

// GetForProvisioner gets the builder required by the provisioner.
func GetForProvisioner(p provision.Provisioner) (Builder, error) {
	builder, err := get(p.GetName())
	if err != nil {
		if _, ok := p.(provision.BuilderDeployKubeClient); ok {
			return get("kubernetes")
		}
	}
	return builder, err
}

// get gets the named builder from the registry.
func get(name string) (Builder, error) {
	b, ok := builders[name]
	if !ok {
		return nil, fmt.Errorf("unknown builder: %q", name)
	}
	return b, nil
}

// Registry returns the list of registered builders.
func Registry() ([]Builder, error) {
	names := make([]string, 0, len(builders))
	for name := range builders {
		names = append(names, name)
	}

	sort.Strings(names) // returns builder in a predictable way

	registry := make([]Builder, 0, len(builders))
	for _, name := range names {
		registry = append(registry, builders[name])
	}

	return registry, nil
}

func PlatformBuild(ctx context.Context, opts appTypes.PlatformOptions) ([]string, error) {
	builders, err := Registry()
	if err != nil {
		return nil, err
	}

	opts.ExtraTags = []string{"latest"}

	for _, b := range builders {
		if pbv2, ok := b.(PlatformBuilderV2); ok {
			images, err := pbv2.PlatformBuildV2(ctx, opts)
			if err == nil {
				return images, nil
			}

			if !errors.Is(err, ErrBuildV2NotSupported) {
				return nil, err
			}

			// otherwise fallback to legacy platform build method
		}

		if pb, ok := b.(PlatformBuilder); ok {
			return pb.PlatformBuild(ctx, opts)
		}
	}

	return nil, errors.New("No builder available")
}

func PlatformRemove(ctx context.Context, name string) error {
	builders, err := Registry()
	if err != nil {
		return err
	}
	multiErr := tsuruErrors.NewMultiError()
	for _, b := range builders {
		if platformBuilder, ok := b.(PlatformBuilder); ok {
			err = platformBuilder.PlatformRemove(ctx, name)
			if err == nil {
				return nil
			}
			multiErr.Add(err)
		}
	}
	if multiErr.Len() > 0 {
		return multiErr
	}
	return errors.New("No builder available")
}

func CompressDockerFile(data []byte) io.Reader {
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	writer.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Mode: 0644,
		Size: int64(len(data)),
	})
	writer.Write(data)
	writer.Close()
	return &buf
}

func DownloadArchiveFromURL(ctx context.Context, url string) (io.ReadCloser, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := net.Dial15Full300Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, 0, errors.New("could not download the archive: unexpected status code")
	}

	var out bytes.Buffer
	s, err := io.Copy(&out, resp.Body)
	if err != nil {
		return nil, 0, err
	}

	if s == 0 {
		return nil, 0, errors.New("archive file is empty")
	}

	return io.NopCloser(&out), out.Len(), nil
}
