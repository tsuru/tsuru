// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"context"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var _ Builder = &MockBuilder{}
var _ PlatformBuilder = &MockBuilder{}

type MockBuilder struct {
	OnBuild          func(provision.BuilderDeploy, provision.App, *event.Event, *BuildOpts) (appTypes.AppVersion, error)
	OnPlatformBuild  func(appTypes.PlatformOptions) ([]string, error)
	OnPlatformRemove func(string) error
}

func (b *MockBuilder) Build(ctx context.Context, p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *BuildOpts) (appTypes.AppVersion, error) {
	if b.OnBuild == nil {
		return nil, nil
	}
	return b.OnBuild(p, app, evt, opts)
}

func (b *MockBuilder) PlatformBuild(ctx context.Context, opts appTypes.PlatformOptions) ([]string, error) {
	if b.OnPlatformBuild == nil {
		return nil, nil
	}
	return b.OnPlatformBuild(opts)
}

func (b *MockBuilder) PlatformRemove(ctx context.Context, name string) error {
	if b.OnPlatformRemove == nil {
		return nil
	}
	return b.OnPlatformRemove(name)
}
