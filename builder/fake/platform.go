// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fake

import (
	"errors"

	"github.com/tsuru/tsuru/builder"
)

var _ builder.PlatformBuilder = &FakePlatformBuilder{}

type provisionedPlatform struct {
	Name    string
	Args    map[string]string
	Version int
}

type FakePlatformBuilder struct {
	platforms []provisionedPlatform
}

func (p *FakePlatformBuilder) PlatformAdd(opts builder.PlatformOptions) error {
	if p.GetPlatform(opts.Name) != nil {
		return errors.New("duplicate platform")
	}
	p.platforms = append(p.platforms, provisionedPlatform{Name: opts.Name, Args: opts.Args, Version: 1})
	return nil
}

func (p *FakePlatformBuilder) PlatformUpdate(opts builder.PlatformOptions) error {
	index, platform := p.getPlatform(opts.Name)
	if platform == nil {
		return errors.New("platform not found")
	}
	platform.Version += 1
	platform.Args = opts.Args
	p.platforms[index] = *platform
	return nil
}

func (p *FakePlatformBuilder) PlatformRemove(name string) error {
	index, _ := p.getPlatform(name)
	if index < 0 {
		return errors.New("platform not found")
	}
	p.platforms[index] = p.platforms[len(p.platforms)-1]
	p.platforms = p.platforms[:len(p.platforms)-1]
	return nil
}

func (p *FakePlatformBuilder) GetPlatform(name string) *provisionedPlatform {
	_, platform := p.getPlatform(name)
	return platform
}

func (p *FakePlatformBuilder) getPlatform(name string) (int, *provisionedPlatform) {
	for i, platform := range p.platforms {
		if platform.Name == name {
			return i, &platform
		}
	}
	return -1, nil
}
