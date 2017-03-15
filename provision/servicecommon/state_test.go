// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

func (s *S) TestChangeAppState(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	err := image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web1",
			"worker": "python worker1",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(fakeApp.GetName(), "myimg")
	c.Assert(err, check.IsNil)
	err = ChangeAppState(m, fakeApp, "", ProcessState{Restart: true})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "myimg", count: ProcessState{Restart: true}},
		{action: "deploy", app: fakeApp, processName: "worker", image: "myimg", count: ProcessState{Restart: true}},
	})
	m.reset()
	err = ChangeAppState(m, fakeApp, "worker", ProcessState{Restart: true})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "myimg", count: ProcessState{}},
		{action: "deploy", app: fakeApp, processName: "worker", image: "myimg", count: ProcessState{Restart: true}},
	})
}

func (s *S) TestChangeUnits(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	fakeApp.Deploys = 1
	err := image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python web1",
			"worker": "python worker1",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(fakeApp.GetName(), "myimg")
	c.Assert(err, check.IsNil)
	err = ChangeUnits(m, fakeApp, 1, "worker")
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "myimg", count: ProcessState{}},
		{action: "deploy", app: fakeApp, processName: "worker", image: "myimg", count: ProcessState{Increment: 1}},
	})
	err = ChangeUnits(m, fakeApp, 1, "")
	c.Assert(err, check.ErrorMatches, "process error: no process name specified and more than one declared in Procfile")
}

func (s *S) TestChangeUnitsSingleProcess(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	fakeApp.Deploys = 1
	err := image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python web1",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(fakeApp.GetName(), "myimg")
	c.Assert(err, check.IsNil)
	err = ChangeUnits(m, fakeApp, 1, "")
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "myimg", count: ProcessState{Increment: 1}},
	})
}
