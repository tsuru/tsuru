// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"io"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var DefaultBuilder = "docker"

type BuildOpts struct {
	BuildFromFile       bool
	Rebuild             bool
	Redeploy            bool
	IsTsuruBuilderImage bool
	ArchiveURL          string
	ArchiveFile         io.Reader
	ArchiveTarFile      io.ReadCloser
	ArchiveSize         int64
	ImageID             string
	Tag                 string
}

// Builder is the basic interface of this package.
type Builder interface {
	Build(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *BuildOpts) (string, error)
}

var builders = make(map[string]Builder)

// PlatformBuilder is a builder where administrators can manage
// platforms (automatically adding, removing and updating platforms).
type PlatformBuilder interface {
	PlatformAdd(appTypes.PlatformOptions) error
	PlatformUpdate(appTypes.PlatformOptions) error
	PlatformRemove(name string) error
}

// Register registers a new builder in the Builder registry.
func Register(name string, builder Builder) {
	builders[name] = builder
}

// GetForProvisioner gets the builder required by the provisioner.
func GetForProvisioner(p provision.Provisioner) (Builder, error) {
	builder, err := get(p.GetName())
	if err != nil {
		if _, ok := p.(provision.BuilderDeployDockerClient); ok {
			return get("docker")
		} else if _, ok := p.(provision.BuilderDeployKubeClient); ok {
			return get("kubernetes")
		}
	}
	return builder, err
}

// get gets the named builder from the registry.
func get(name string) (Builder, error) {
	b, ok := builders[name]
	if !ok {
		return nil, errors.Errorf("unknown builder: %q", name)
	}
	return b, nil
}

// Registry returns the list of registered builders.
func Registry() ([]Builder, error) {
	registry := make([]Builder, 0, len(builders))
	for _, b := range builders {
		registry = append(registry, b)
	}
	return registry, nil
}

func PlatformAdd(opts appTypes.PlatformOptions) error {
	builders, err := Registry()
	if err != nil {
		return err
	}
	multiErr := tsuruErrors.NewMultiError()
	for _, b := range builders {
		if platformBuilder, ok := b.(PlatformBuilder); ok {
			err = platformBuilder.PlatformAdd(opts)
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

func PlatformUpdate(opts appTypes.PlatformOptions) error {
	builders, err := Registry()
	if err != nil {
		return err
	}
	multiErr := tsuruErrors.NewMultiError()
	for _, b := range builders {
		if platformBuilder, ok := b.(PlatformBuilder); ok {
			err = platformBuilder.PlatformUpdate(opts)
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

func PlatformRemove(name string) error {
	builders, err := Registry()
	if err != nil {
		return err
	}
	multiErr := tsuruErrors.NewMultiError()
	for _, b := range builders {
		if platformBuilder, ok := b.(PlatformBuilder); ok {
			err = platformBuilder.PlatformRemove(name)
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
