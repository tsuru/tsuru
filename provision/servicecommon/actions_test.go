// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

type S struct {
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "servicecommon_tests_s")
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Apps().Database)
	c.Assert(err, check.IsNil)
}

type managerCall struct {
	action      string
	app         provision.App
	processName string
	image       string
	count       ProcessState
}

type recordManager struct {
	deployErrMap map[string]error
	removeErrMap map[string]error
	calls        []managerCall
}

func (m *recordManager) reset() {
	m.deployErrMap = nil
	m.removeErrMap = nil
	m.calls = nil
}

func (m *recordManager) DeployService(a provision.App, processName string, count ProcessState, image string) error {
	call := managerCall{
		action:      "deploy",
		processName: processName,
		image:       image,
		count:       count,
		app:         a,
	}
	m.calls = append(m.calls, call)
	if m.deployErrMap != nil {
		return m.deployErrMap[processName]
	}
	return nil
}

func (m *recordManager) RemoveService(a provision.App, processName string) error {
	call := managerCall{
		action:      "remove",
		processName: processName,
		app:         a,
	}
	m.calls = append(m.calls, call)
	if m.removeErrMap != nil {
		return m.removeErrMap[processName]
	}
	return nil
}

func (s *S) TestRunServicePipeline(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	err := image.SaveImageCustomData("oldImage", map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web1",
			"worker1": "python worker1",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(fakeApp.GetName(), "oldImage")
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("newImage", map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web2",
			"worker2": "python worker2",
		},
	})
	c.Assert(err, check.IsNil)
	err = RunServicePipeline(m, fakeApp, "newImage", ProcessSpec{
		"web":     ProcessState{Increment: 5},
		"worker2": ProcessState{},
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "newImage", count: ProcessState{Increment: 5}},
		{action: "deploy", app: fakeApp, processName: "worker2", image: "newImage", count: ProcessState{}},
		{action: "remove", app: fakeApp, processName: "worker1"},
	})
	imgName, err := image.AppCurrentImageName(fakeApp.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(imgName, check.Equals, "newImage")
}

func (s *S) TestRunServicePipelineNilSpec(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	err := image.SaveImageCustomData("oldImage", map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web1",
			"worker1": "python worker1",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(fakeApp.GetName(), "oldImage")
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("newImage", map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web2",
			"worker2": "python worker2",
		},
	})
	c.Assert(err, check.IsNil)
	err = RunServicePipeline(m, fakeApp, "newImage", nil)
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "newImage", count: ProcessState{Start: true}},
		{action: "deploy", app: fakeApp, processName: "worker2", image: "newImage", count: ProcessState{Start: true}},
		{action: "remove", app: fakeApp, processName: "worker1"},
	})
	imgName, err := image.AppCurrentImageName(fakeApp.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(imgName, check.Equals, "newImage")
}

func (s *S) TestRunServicePipelineSingleProcess(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	err := image.SaveImageCustomData("oldImage", map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web1",
			"worker1": "python worker1",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(fakeApp.GetName(), "oldImage")
	c.Assert(err, check.IsNil)
	err = RunServicePipeline(m, fakeApp, "oldImage", ProcessSpec{
		"web": ProcessState{Restart: true},
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "oldImage", count: ProcessState{Restart: true}},
		{action: "deploy", app: fakeApp, processName: "worker1", image: "oldImage", count: ProcessState{}},
	})
}

func (s *S) TestActionUpdateServicesForward(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newImage:         "image",
		newImageSpec:     ProcessSpec{"web": ProcessState{Increment: 1}},
		currentImage:     "oldImage",
		currentImageSpec: ProcessSpec{},
	}
	processes, err := updateServices.Forward(action.FWContext{Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, []string{"web"})
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "image", count: ProcessState{Increment: 1}},
	})
}

func (s *S) TestActionUpdateServicesForwardMultiple(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newImage:         "image",
		newImageSpec:     ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{}},
		currentImage:     "oldImage",
		currentImageSpec: ProcessSpec{"web": ProcessState{}, "worker1": ProcessState{}},
	}
	processes, err := updateServices.Forward(action.FWContext{Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, []string{"web", "worker2"})
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "image", count: ProcessState{Increment: 5}},
		{action: "deploy", app: fakeApp, processName: "worker2", image: "image", count: ProcessState{}},
	})
}

func (s *S) TestActionUpdateServicesForwardFailureInMiddle(c *check.C) {
	expectedError := errors.New("my deploy error")
	m := &recordManager{
		deployErrMap: map[string]error{"worker2": expectedError},
	}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newImage:         "image",
		newImageSpec:     ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{}},
		currentImage:     "oldImage",
		currentImageSpec: ProcessSpec{"web": ProcessState{}, "worker1": ProcessState{}},
	}
	processes, err := updateServices.Forward(action.FWContext{Params: []interface{}{args}})
	c.Assert(err, check.Equals, expectedError)
	c.Assert(processes, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "image", count: ProcessState{Increment: 5}},
		{action: "deploy", app: fakeApp, processName: "worker2", image: "image", count: ProcessState{}},
		{action: "deploy", app: fakeApp, processName: "web", image: "oldImage", count: ProcessState{}},
	})
}

func (s *S) TestActionUpdateServicesForwardFailureInMiddleNewProc(c *check.C) {
	expectedError := errors.New("my deploy error")
	m := &recordManager{
		deployErrMap: map[string]error{"worker2": expectedError},
	}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newImage:         "image",
		newImageSpec:     ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{}},
		currentImage:     "oldImage",
		currentImageSpec: ProcessSpec{"worker1": ProcessState{}},
	}
	processes, err := updateServices.Forward(action.FWContext{Params: []interface{}{args}})
	c.Assert(err, check.Equals, expectedError)
	c.Assert(processes, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "image", count: ProcessState{Increment: 5}},
		{action: "deploy", app: fakeApp, processName: "worker2", image: "image", count: ProcessState{}},
		{action: "remove", app: fakeApp, processName: "web"},
	})
}

func (s *S) TestActionUpdateServicesBackward(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newImage:         "image",
		newImageSpec:     ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{}},
		currentImage:     "oldImage",
		currentImageSpec: ProcessSpec{"web": ProcessState{}, "worker1": ProcessState{}},
	}
	updateServices.Backward(action.BWContext{
		FWResult: []string{"web", "worker2"},
		Params:   []interface{}{args},
	})
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "oldImage", count: ProcessState{}},
		{action: "remove", app: fakeApp, processName: "worker2"},
	})
}

func (s *S) TestUpdateImageInDBForward(c *check.C) {
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	args := &pipelineArgs{
		app:      fakeApp,
		newImage: "newImage",
	}
	_, err := updateImageInDB.Forward(action.FWContext{Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	imgName, err := image.AppCurrentImageName(fakeApp.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(imgName, check.Equals, "newImage")
}

func (s *S) TestRemoveOldServicesForward(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newImage:         "image",
		newImageSpec:     ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{}},
		currentImage:     "oldImage",
		currentImageSpec: ProcessSpec{"web": ProcessState{}, "worker1": ProcessState{}},
	}
	_, err := removeOldServices.Forward(action.FWContext{Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "remove", app: fakeApp, processName: "worker1"},
	})
}
