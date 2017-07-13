// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fake

import (
	"errors"

	"github.com/tsuru/tsuru/builder"
)

var _ builder.PlatformBuilder = &FakeBuilder{}

type provisionedPlatform struct {
	Name    string
	Args    map[string]string
	Version int
}

func (b *FakeBuilder) PlatformAdd(opts builder.PlatformOptions) error {
	if err := b.getError("PlatformAdd"); err != nil {
		return err
	}
	if b.GetPlatform(opts.Name) != nil {
		return errors.New("duplicate platform")
	}
	b.platforms = append(b.platforms, provisionedPlatform{Name: opts.Name, Args: opts.Args, Version: 1})
	return nil
}

func (b *FakeBuilder) PlatformUpdate(opts builder.PlatformOptions) error {
	index, platform := b.getPlatform(opts.Name)
	if platform == nil {
		return errors.New("platform not found")
	}
	platform.Version += 1
	platform.Args = opts.Args
	b.platforms[index] = *platform
	return nil
}

func (b *FakeBuilder) PlatformRemove(name string) error {
	index, _ := b.getPlatform(name)
	if index < 0 {
		return errors.New("platform not found")
	}
	b.platforms[index] = b.platforms[len(b.platforms)-1]
	b.platforms = b.platforms[:len(b.platforms)-1]
	return nil
}

func (b *FakeBuilder) GetPlatform(name string) *provisionedPlatform {
	_, platform := b.getPlatform(name)
	return platform
}

func (b *FakeBuilder) Reset() {
	b.platforms = nil
}

func (b *FakeBuilder) getPlatform(name string) (int, *provisionedPlatform) {
	for i, platform := range b.platforms {
		if platform.Name == name {
			return i, &platform
		}
	}
	return -1, nil
}
