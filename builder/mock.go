// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
)

var _ Builder = &MockBuilder{}
var _ PlatformBuilder = &MockBuilder{}

type MockBuilder struct {
	OnBuild          func(provision.BuilderDeploy, provision.App, *event.Event, *BuildOpts) (string, error)
	OnPlatformAdd    func(PlatformOptions) error
	OnPlatformUpdate func(PlatformOptions) error
	OnPlatformRemove func(string) error
}

func (b *MockBuilder) Build(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *BuildOpts) (string, error) {
	if b.OnBuild == nil {
		return "", nil
	}
	return b.OnBuild(p, app, evt, opts)
}

func (b *MockBuilder) PlatformAdd(opts PlatformOptions) error {
	if b.OnPlatformAdd == nil {
		return nil
	}
	return b.OnPlatformAdd(opts)
}

func (b *MockBuilder) PlatformUpdate(opts PlatformOptions) error {
	if b.OnPlatformUpdate == nil {
		return nil
	}
	return b.OnPlatformUpdate(opts)
}

func (b *MockBuilder) PlatformRemove(name string) error {
	if b.OnPlatformRemove == nil {
		return nil
	}
	return b.OnPlatformRemove(name)
}

func (b *MockBuilder) Reset() {
	b.OnBuild = nil
	b.OnPlatformAdd = nil
	b.OnPlatformRemove = nil
	b.OnPlatformUpdate = nil
}
