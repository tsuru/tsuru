// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func newEmptyVersion(c *check.C, app appTypes.App) appTypes.AppVersion {
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: app,
	})
	c.Assert(err, check.IsNil)
	return version
}

func newVersion(c *check.C, app appTypes.App, customData map[string]interface{}) appTypes.AppVersion {
	version := newEmptyVersion(c, app)
	err := version.CommitBuildImage()
	c.Assert(err, check.IsNil)
	err = version.AddData(appTypes.AddVersionDataArgs{
		CustomData: customData,
	})
	c.Assert(err, check.IsNil)
	return version
}

func newCommittedVersion(c *check.C, app appTypes.App, customData map[string]interface{}) appTypes.AppVersion {
	version := newVersion(c, app, customData)
	err := version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	return version
}

func newSuccessfulVersion(c *check.C, app appTypes.App, customData map[string]interface{}) appTypes.AppVersion {
	version := newCommittedVersion(c, app, customData)
	err := version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	return version
}

func (s *S) newTestEvent(c *check.C, a provision.App) *event.Event {
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	return evt
}
