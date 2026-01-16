// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
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

// Builder is the basic interface of this package.
type Builder interface {
	Build(ctx context.Context, app *appTypes.App, evt *event.Event, opts BuildOpts) (appTypes.AppVersion, error)
	BuildJob(ctx context.Context, job *jobTypes.Job, opts BuildOpts) (string, error)
}

var builders = make(map[string]Builder)

// PlatformBuilder is a builder where administrators can manage
// platforms (automatically adding, removing and updating platforms).
type PlatformBuilder interface {
	PlatformBuild(context.Context, appTypes.PlatformOptions) ([]string, error)
}

// Register registers a new builder in the Builder registry.
func Register(name string, builder Builder) {
	builders[name] = builder
}

// GetForProvisioner gets the builder required by the provisioner.
func GetForProvisioner(p provision.Provisioner) (Builder, error) {
	builder, err := get(p.GetName())
	if err != nil {
		if _, ok := p.(provision.BuilderDeploy); ok {
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
		if pb, ok := b.(PlatformBuilder); ok {
			images, err := pb.PlatformBuild(ctx, opts)
			if err == nil {
				return images, nil
			}

			return nil, err
		}
	}

	return nil, errors.New("No builder available")
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
