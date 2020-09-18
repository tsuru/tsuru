// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"context"

	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	check "gopkg.in/check.v1"
)

func (s *S) TestChangeAppState(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	latestVersion := newSuccessfulVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web1",
			"worker": "python worker1",
		},
	})
	err := ChangeAppState(context.TODO(), m, fakeApp, "", ProcessState{Restart: true}, latestVersion)
	c.Assert(err, check.IsNil)
	labelsWeb, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 1,
	})
	c.Assert(err, check.IsNil)
	labelsWeb.SetRestarts(1)
	labelsWorker, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "worker",
		Version: 1,
	})
	c.Assert(err, check.IsNil)
	labelsWorker.SetRestarts(1)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", replicas: 1, labels: labelsWeb, version: latestVersion, preserveVersions: true},
		{action: "deploy", app: fakeApp, processName: "worker", replicas: 1, labels: labelsWorker, version: latestVersion, preserveVersions: true},
		{action: "cleanup", app: fakeApp, versionNumber: latestVersion.Version(), preserveVersions: true},
	})
	m.reset()
	err = ChangeAppState(context.TODO(), m, fakeApp, "worker", ProcessState{Restart: true}, latestVersion)
	c.Assert(err, check.IsNil)
	labelsWeb, err = provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", replicas: 0, labels: labelsWeb, version: latestVersion, preserveVersions: true},
		{action: "deploy", app: fakeApp, processName: "worker", replicas: 1, labels: labelsWorker, version: latestVersion, preserveVersions: true},
		{action: "cleanup", app: fakeApp, versionNumber: latestVersion.Version(), preserveVersions: true},
	})
}

func (s *S) TestChangeUnits(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	fakeApp.Deploys = 1
	latestVersion := newSuccessfulVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web1",
			"worker": "python worker1",
		},
	})
	err := ChangeUnits(context.TODO(), m, fakeApp, 1, "worker", latestVersion)
	c.Assert(err, check.IsNil)
	labelsWeb, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 1,
	})
	c.Assert(err, check.IsNil)
	labelsWorker, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "worker",
		Version: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", replicas: 0, labels: labelsWeb, version: latestVersion, preserveVersions: true},
		{action: "deploy", app: fakeApp, processName: "worker", replicas: 1, labels: labelsWorker, version: latestVersion, preserveVersions: true},
		{action: "cleanup", app: fakeApp, versionNumber: latestVersion.Version(), preserveVersions: true},
	})
	err = ChangeUnits(context.TODO(), m, fakeApp, 1, "", latestVersion)
	c.Assert(err, check.ErrorMatches, "process error: no process name specified and more than one declared in Procfile")
}

func (s *S) TestChangeUnitsSingleProcess(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	fakeApp.Deploys = 1
	latestVersion := newSuccessfulVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python web1",
		},
	})
	err := ChangeUnits(context.TODO(), m, fakeApp, 1, "", latestVersion)
	c.Assert(err, check.IsNil)
	labelsWeb, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", replicas: 1, labels: labelsWeb, version: latestVersion, preserveVersions: true},
		{action: "cleanup", app: fakeApp, versionNumber: latestVersion.Version(), preserveVersions: true},
	})
}
