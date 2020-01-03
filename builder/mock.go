// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var _ Builder = &MockBuilder{}
var _ PlatformBuilder = &MockBuilder{}

type MockBuilder struct {
	OnBuild          func(provision.BuilderDeploy, provision.App, *event.Event, *BuildOpts) (provision.NewImageInfo, error)
	OnPlatformBuild  func(appTypes.PlatformOptions) error
	OnPlatformRemove func(string) error
}

func (b *MockBuilder) Build(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *BuildOpts) (provision.NewImageInfo, error) {
	if b.OnBuild == nil {
		return image.NewImageInfo{}, nil
	}
	return b.OnBuild(p, app, evt, opts)
}

func (b *MockBuilder) PlatformBuild(opts appTypes.PlatformOptions) error {
	if b.OnPlatformBuild == nil {
		return nil
	}
	return b.OnPlatformBuild(opts)
}

func (b *MockBuilder) PlatformRemove(name string) error {
	if b.OnPlatformRemove == nil {
		return nil
	}
	return b.OnPlatformRemove(name)
}

type MockImageInfo struct {
	FakeBaseImageName  string
	FakeBuildImageName string
	FakeVersion        int
	FakeIsBuild        bool
}

func (i MockImageInfo) BaseImageName() string {
	return i.FakeBaseImageName
}

func (i MockImageInfo) BuildImageName() string {
	return i.FakeBuildImageName
}

func (i MockImageInfo) Version() int {
	return i.FakeVersion
}

func (i MockImageInfo) IsBuild() bool {
	return i.FakeIsBuild
}
