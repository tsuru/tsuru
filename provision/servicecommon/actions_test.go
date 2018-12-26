// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type S struct {
	mockService servicemock.MockService
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "servicecommon_tests_s")
	config.Set("routers:fake:type", "fake")
	config.Set("queue:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("queue:mongo-database", "queue_servicecommon_tests_s")
	config.Set("queue:mongo-polling-interval", 0.01)
	servicemock.SetMockService(&s.mockService)
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Apps().Database)
	c.Assert(err, check.IsNil)
	plan := appTypes.Plan{
		Name:     "default",
		Default:  true,
		CpuShare: 100,
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{plan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &plan, nil
	}
}

func (s *S) TearDownTest(c *check.C) {
	app.GetAppRouterUpdater().Shutdown(context.Background())
	s.mockService.ResetPlan()
	s.mockService.ResetPlatform()
	s.mockService.ResetTeam()
}

type managerCall struct {
	action      string
	app         provision.App
	processName string
	image       string
	labels      *provision.LabelSet
	replicas    int
}

type recordManager struct {
	deployErrMap map[string]error
	removeErrMap map[string]error
	lastLabels   map[string]*provision.LabelSet
	calls        []managerCall
}

func (m *recordManager) reset() {
	m.deployErrMap = nil
	m.removeErrMap = nil
	m.calls = nil
}

func (m *recordManager) CurrentLabels(a provision.App, processName string) (*provision.LabelSet, error) {
	if m.lastLabels != nil {
		return m.lastLabels[processName], nil
	}
	return nil, nil
}

func (m *recordManager) DeployService(ctx context.Context, a provision.App, processName string, labels *provision.LabelSet, replicas int, image string) error {
	call := managerCall{
		action:      "deploy",
		processName: processName,
		image:       image,
		labels:      labels,
		replicas:    replicas,
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
	}, nil)
	c.Assert(err, check.IsNil)
	labelsWeb, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "web",
		Replicas: 5,
	})
	c.Assert(err, check.IsNil)
	labelsWorker, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "worker2",
		Replicas: 0,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "newImage", replicas: 5, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker2", image: "newImage", replicas: 0, labels: labelsWorker},
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
	err = RunServicePipeline(m, fakeApp, "newImage", nil, nil)
	c.Assert(err, check.IsNil)
	labelsWeb, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "web",
		Replicas: 1,
	})
	c.Assert(err, check.IsNil)
	labelsWorker, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "worker2",
		Replicas: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "newImage", replicas: 1, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker2", image: "newImage", replicas: 1, labels: labelsWorker},
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
	}, nil)
	c.Assert(err, check.IsNil)
	labelsWeb, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "web",
		Replicas: 1,
	})
	c.Assert(err, check.IsNil)
	labelsWeb.SetRestarts(1)
	labelsWorker, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "worker1",
		Replicas: 0,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "oldImage", replicas: 1, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker1", image: "oldImage", replicas: 0, labels: labelsWorker},
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
	labelsWeb, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "web",
		Replicas: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "image", replicas: 1, labels: labelsWeb},
	})
	c.Assert(fakeApp.Quota.InUse, check.Equals, 1)
}

func (s *S) TestActionUpdateServicesForwardMultiple(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newImage:         "image",
		newImageSpec:     ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{Start: true}},
		currentImage:     "oldImage",
		currentImageSpec: ProcessSpec{"web": ProcessState{}, "worker1": ProcessState{}},
	}
	processes, err := updateServices.Forward(action.FWContext{Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, []string{"web", "worker2"})
	labelsWeb, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "web",
		Replicas: 5,
	})
	c.Assert(err, check.IsNil)
	labelsWorker, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "worker2",
		Replicas: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "image", replicas: 5, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker2", image: "image", replicas: 1, labels: labelsWorker},
	})
	c.Assert(fakeApp.Quota.InUse, check.Equals, 6)
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
	labelsWeb, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "web",
		Replicas: 5,
	})
	c.Assert(err, check.IsNil)
	labelsWebOld, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "web",
		Replicas: 0,
	})
	c.Assert(err, check.IsNil)
	labelsWorker, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "worker2",
		Replicas: 0,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "image", replicas: 5, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker2", image: "image", replicas: 0, labels: labelsWorker},
		{action: "deploy", app: fakeApp, processName: "web", image: "oldImage", replicas: 0, labels: labelsWebOld},
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
	labelsWeb, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "web",
		Replicas: 5,
	})
	c.Assert(err, check.IsNil)
	labelsWorker, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "worker2",
		Replicas: 0,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "image", replicas: 5, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker2", image: "image", replicas: 0, labels: labelsWorker},
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
	labelsWeb, err := provision.ServiceLabels(provision.ServiceLabelsOpts{
		App:      fakeApp,
		Process:  "web",
		Replicas: 0,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", image: "oldImage", replicas: 0, labels: labelsWeb},
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

func (s *S) TestRunServicePipelineUpdateStates(c *check.C) {
	m := &recordManager{}
	a := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	err := image.SaveImageCustomData("myimg", map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
	c.Assert(err, check.IsNil)
	tests := []struct {
		states []ProcessState
		fn     func(replicas int, ls *provision.LabelSet)
	}{
		{
			states: []ProcessState{
				{Start: true}, {Increment: 1},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(replicas, check.Equals, 2)
				c.Assert(a.Quota.InUse, check.Equals, 2)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(replicas, check.Equals, 0)
				c.Assert(ls.AppReplicas(), check.Equals, 3)
				c.Assert(ls.IsStopped(), check.Equals, true)
				c.Assert(a.Quota.InUse, check.Equals, 3)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(ls.IsAsleep(), check.Equals, true)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Start: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(replicas, check.Equals, 3)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {Start: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(ls.IsAsleep(), check.Equals, false)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Restart: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(replicas, check.Equals, 3)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {Restart: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(ls.IsAsleep(), check.Equals, false)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(replicas, check.Equals, 0)
				c.Assert(ls.AppReplicas(), check.Equals, 3)
				c.Assert(ls.IsStopped(), check.Equals, true)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Sleep: true}, {},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(ls.IsAsleep(), check.Equals, true)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Restart: true}, {Restart: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(replicas, check.Equals, 1)
				c.Assert(ls.Restarts(), check.Equals, 2)
			},
		},
	}
	for _, tt := range tests {
		for _, s := range tt.states {
			m.reset()
			err = RunServicePipeline(m, a, "myimg", ProcessSpec{
				"p1": s,
			}, nil)
			c.Assert(err, check.IsNil)
			c.Assert(m.calls, check.HasLen, 1)
			m.lastLabels = map[string]*provision.LabelSet{
				"p1": m.calls[0].labels,
			}
		}
		c.Assert(m.calls, check.HasLen, 1)
		tt.fn(m.calls[0].replicas, m.calls[0].labels)
		m.reset()
		m.lastLabels = nil
	}
}
