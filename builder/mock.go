// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"context"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
)

var _ Builder = &MockBuilder{}
var _ PlatformBuilder = &MockBuilder{}

type MockBuilder struct {
	OnBuild          func(provision.App, *event.Event, BuildOpts) (appTypes.AppVersion, error)
	OnBuildJob       func(*jobTypes.Job, BuildOpts) (string, error)
	OnPlatformBuild  func(appTypes.PlatformOptions) ([]string, error)
	OnPlatformRemove func(string) error
}

func (b *MockBuilder) Build(ctx context.Context, app provision.App, evt *event.Event, opts BuildOpts) (appTypes.AppVersion, error) {
	if b.OnBuild == nil {
		return nil, nil
	}
	return b.OnBuild(app, evt, opts)
}

func (b *MockBuilder) BuildJob(ctx context.Context, job *jobTypes.Job, opts BuildOpts) (string, error) {
	if b.OnBuild == nil {
		return "", nil
	}
	return b.OnBuildJob(job, opts)
}

func (b *MockBuilder) PlatformBuild(ctx context.Context, opts appTypes.PlatformOptions) ([]string, error) {
	if b.OnPlatformBuild == nil {
		return nil, nil
	}

	return b.OnPlatformBuild(opts)
}
