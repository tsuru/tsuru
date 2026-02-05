// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
)

type S struct {
	mockService servicemock.MockService
}

var (
	_                = check.Suite(&S{})
	_ ServiceManager = &recordManager{}
)

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "servicecommon_tests_s")
	config.Set("routers:fake:type", "fake")
	servicemock.SetMockService(&s.mockService)
}

func (s *S) SetUpTest(c *check.C) {
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	plan := appTypes.Plan{
		Name:    "default",
		Default: true,
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{plan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &plan, nil
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == plan.Name {
			return &plan, nil
		}
		return nil, appTypes.ErrPlanNotFound
	}
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownTest(c *check.C) {
	app.GetAppRouterUpdater().Shutdown(context.Background())
	s.mockService.ResetPlan()
	s.mockService.ResetPlatform()
	s.mockService.ResetTeam()
}

type managerCall struct {
	action           string
	app              *appTypes.App
	processName      string
	version          appTypes.AppVersion
	versionNumber    int
	labels           *provision.LabelSet
	replicas         int
	preserveVersions bool
}

type recordManager struct {
	deployErrMap map[string]error
	removeErrMap map[string]error
	lastLabels   map[string]*provision.LabelSet
	lastReplicas map[string]*int32
	calls        []managerCall
}

func (m *recordManager) reset() {
	m.deployErrMap = nil
	m.removeErrMap = nil
	m.calls = nil
}

func (m *recordManager) CurrentLabels(ctx context.Context, a *appTypes.App, processName string, versionNumber int) (ls *provision.LabelSet, rep *int32, err error) {
	key := fmt.Sprintf("%s-v%d", processName, versionNumber)
	if m.lastLabels != nil {
		ls = m.lastLabels[key]
	}
	if m.lastReplicas != nil {
		rep = m.lastReplicas[key]
	}
	return ls, rep, err
}

func (m *recordManager) DeployService(ctx context.Context, opts DeployServiceOpts) error {
	call := managerCall{
		action:           "deploy",
		processName:      opts.ProcessName,
		version:          opts.Version,
		labels:           opts.Labels,
		replicas:         opts.Replicas,
		app:              opts.App,
		preserveVersions: opts.PreserveVersions,
	}
	m.calls = append(m.calls, call)
	if m.deployErrMap != nil {
		return m.deployErrMap[opts.ProcessName]
	}
	return nil
}

func (m *recordManager) CleanupServices(ctx context.Context, a *appTypes.App, versionNumber int, preserveVersions bool) error {
	call := managerCall{
		action:           "cleanup",
		app:              a,
		versionNumber:    versionNumber,
		preserveVersions: preserveVersions,
	}
	m.calls = append(m.calls, call)
	return nil
}

func (m *recordManager) RemoveService(ctx context.Context, a *appTypes.App, processName string, versionNumber int) error {
	call := managerCall{
		action:        "remove",
		processName:   processName,
		app:           a,
		versionNumber: versionNumber,
	}
	m.calls = append(m.calls, call)
	if m.removeErrMap != nil {
		return m.removeErrMap[processName]
	}
	return nil
}

func newVersion(c *check.C, app *appTypes.App, customData map[string]interface{}) appTypes.AppVersion {
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: app,
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBuildImage()
	c.Assert(err, check.IsNil)
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = version.AddData(appTypes.AddVersionDataArgs{
		CustomData: customData,
	})
	c.Assert(err, check.IsNil)
	return version
}

func newSuccessfulVersion(c *check.C, app *appTypes.App, customData map[string]interface{}) appTypes.AppVersion {
	version := newVersion(c, app, customData)
	err := version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	return version
}

func (s *S) TestRunServicePipeline(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	oldVersion := newSuccessfulVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web1",
			"worker1": "python worker1",
		},
	})
	newVersion := newVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web2",
			"worker2": "python worker2",
		},
	})
	err := RunServicePipeline(context.TODO(), m, oldVersion.Version(), provision.DeployArgs{
		App:     fakeApp,
		Version: newVersion,
	}, ProcessSpec{
		"web":     ProcessState{Increment: 5},
		"worker2": ProcessState{},
	})
	c.Assert(err, check.IsNil)
	labelsWeb, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 2,
	})
	c.Assert(err, check.IsNil)
	labelsWorker, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "worker2",
		Version: 2,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", version: newVersion, replicas: 5, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker2", version: newVersion, replicas: 0, labels: labelsWorker},
		{action: "remove", app: fakeApp, processName: "worker1", versionNumber: oldVersion.Version()},
		{action: "cleanup", app: fakeApp, versionNumber: newVersion.Version()},
	})
	c.Assert(newVersion.VersionInfo().DeploySuccessful, check.Equals, true)
}

func (s *S) TestRunServicePipelineNilSpec(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	oldVersion := newSuccessfulVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web1",
			"worker1": "python worker1",
		},
	})
	newVersion := newVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web2",
			"worker2": "python worker2",
		},
	})
	err := RunServicePipeline(context.TODO(), m, oldVersion.Version(), provision.DeployArgs{
		App:     fakeApp,
		Version: newVersion,
	}, nil)
	c.Assert(err, check.IsNil)
	labelsWeb, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 2,
	})
	c.Assert(err, check.IsNil)
	labelsWorker, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "worker2",
		Version: 2,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", version: newVersion, replicas: 1, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker2", version: newVersion, replicas: 1, labels: labelsWorker},
		{action: "remove", app: fakeApp, processName: "worker1", versionNumber: oldVersion.Version()},
		{action: "cleanup", app: fakeApp, versionNumber: newVersion.Version()},
	})
	c.Assert(newVersion.VersionInfo().DeploySuccessful, check.Equals, true)
}

func (s *S) TestRunServicePipelineSingleProcess(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	version := newVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web1",
			"worker1": "python worker1",
		},
	})
	err := RunServicePipeline(context.TODO(), m, 0, provision.DeployArgs{
		App:     fakeApp,
		Version: version,
	}, ProcessSpec{
		"web": ProcessState{Restart: true},
	})
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
		Process: "worker1",
		Version: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", version: version, replicas: 1, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker1", version: version, replicas: 0, labels: labelsWorker},
		{action: "cleanup", app: fakeApp, versionNumber: version.Version()},
	})
}

func (s *S) TestActionUpdateServicesForward(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	oldVersion := newSuccessfulVersion(c, fakeApp, nil)
	newVersion := newVersion(c, fakeApp, nil)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newVersion:       newVersion,
		newVersionSpec:   ProcessSpec{"web": ProcessState{Increment: 1}},
		oldVersion:       oldVersion,
		oldVersionNumber: oldVersion.Version(),
	}
	processes, err := updateServices.Forward(action.FWContext{Context: context.TODO(), Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, map[string]*labelReplicas{"web": {}})
	newLabelsWeb, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 2,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", version: newVersion, replicas: 1, labels: newLabelsWeb},
	})
}

func (s *S) TestActionUpdateServicesForwardMultiple(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	oldVersion := newSuccessfulVersion(c, fakeApp, nil)
	newVersion := newVersion(c, fakeApp, nil)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newVersion:       newVersion,
		newVersionSpec:   ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{Start: true}},
		oldVersion:       oldVersion,
		oldVersionNumber: oldVersion.Version(),
	}
	processes, err := updateServices.Forward(action.FWContext{Context: context.TODO(), Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, map[string]*labelReplicas{"web": {}, "worker2": {}})
	labelsWeb, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 2,
	})
	c.Assert(err, check.IsNil)
	labelsWorker, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "worker2",
		Version: 2,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", version: newVersion, replicas: 5, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker2", version: newVersion, replicas: 1, labels: labelsWorker},
	})
}

func (s *S) TestActionUpdateServicesForwardFailureInMiddle(c *check.C) {
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	labelsWebOld, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 1,
	})
	c.Assert(err, check.IsNil)

	expectedError := errors.New("my deploy error")
	m := &recordManager{
		deployErrMap: map[string]error{"worker2": expectedError},
		lastLabels: map[string]*provision.LabelSet{
			"web-v1": labelsWebOld,
		},
	}

	oldVersion := newSuccessfulVersion(c, fakeApp, nil)
	newVersion := newVersion(c, fakeApp, nil)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newVersion:       newVersion,
		newVersionSpec:   ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{}},
		oldVersion:       oldVersion,
		oldVersionNumber: oldVersion.Version(),
	}
	_, err = updateServices.Forward(action.FWContext{Context: context.TODO(), Params: []interface{}{args}})
	c.Assert(err, check.Equals, expectedError)
	labelsWeb, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 2,
	})
	c.Assert(err, check.IsNil)

	labelsWorker, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "worker2",
		Version: 2,
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", version: newVersion, replicas: 5, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker2", version: newVersion, replicas: 0, labels: labelsWorker},
		{action: "deploy", app: fakeApp, processName: "web", version: oldVersion, replicas: 0, labels: labelsWebOld},
	})
}

func (s *S) TestActionUpdateServicesForwardFailureInMiddleNewProc(c *check.C) {
	expectedError := errors.New("my deploy error")
	m := &recordManager{
		deployErrMap: map[string]error{"worker2": expectedError},
	}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	oldVersion := newSuccessfulVersion(c, fakeApp, nil)
	newVersion := newVersion(c, fakeApp, nil)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newVersion:       newVersion,
		newVersionSpec:   ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{}},
		oldVersion:       oldVersion,
		oldVersionNumber: oldVersion.Version(),
	}
	_, err := updateServices.Forward(action.FWContext{Context: context.TODO(), Params: []interface{}{args}})
	c.Assert(err, check.Equals, expectedError)
	labelsWeb, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 2,
	})
	c.Assert(err, check.IsNil)
	labelsWorker, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "worker2",
		Version: 2,
	})
	c.Assert(err, check.IsNil)
	expected := []managerCall{
		{action: "deploy", app: fakeApp, processName: "web", version: newVersion, replicas: 5, labels: labelsWeb},
		{action: "deploy", app: fakeApp, processName: "worker2", version: newVersion, replicas: 0, labels: labelsWorker},
		{action: "remove", app: fakeApp, processName: "web", versionNumber: newVersion.Version()},
	}
	c.Assert(m.calls, check.DeepEquals, expected)
}

func (s *S) TestActionUpdateServicesBackward(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	oldVersion := newSuccessfulVersion(c, fakeApp, nil)
	newVersion := newVersion(c, fakeApp, nil)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newVersion:       newVersion,
		newVersionSpec:   ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{}},
		oldVersion:       oldVersion,
		oldVersionNumber: oldVersion.Version(),
	}
	labelsWeb, err := provision.ServiceLabels(context.TODO(), provision.ServiceLabelsOpts{
		App:     fakeApp,
		Process: "web",
		Version: 1,
	})
	c.Assert(err, check.IsNil)
	result := map[string]*labelReplicas{
		"web":     {labels: labelsWeb},
		"worker2": {},
	}
	updateServices.Backward(action.BWContext{
		FWResult: result,
		Params:   []interface{}{args},
	})
	sort.Slice(m.calls, func(i, j int) bool {
		return m.calls[0].action < m.calls[1].action
	})
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "remove", app: fakeApp, processName: "worker2", versionNumber: newVersion.Version()},
		{action: "deploy", app: fakeApp, processName: "web", version: oldVersion, replicas: 0, labels: labelsWeb},
	})
}

func (s *S) TestUpdateImageInDBForward(c *check.C) {
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	newVersion := newVersion(c, fakeApp, nil)
	args := &pipelineArgs{
		app:        fakeApp,
		newVersion: newVersion,
	}
	_, err := updateImageInDB.Forward(action.FWContext{Context: context.TODO(), Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	c.Assert(newVersion.VersionInfo().DeploySuccessful, check.Equals, true)
}

func (s *S) TestRemoveOldServicesForward(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	oldVersion := newSuccessfulVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web1",
			"worker1": "python worker1",
		},
	})
	newVersion := newVersion(c, fakeApp, nil)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newVersion:       newVersion,
		newVersionSpec:   ProcessSpec{"web": ProcessState{Increment: 5}, "worker2": ProcessState{}},
		oldVersion:       oldVersion,
		oldVersionNumber: oldVersion.Version(),
	}
	_, err := removeOldServices.Forward(action.FWContext{Context: context.TODO(), Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	c.Assert(m.calls, check.DeepEquals, []managerCall{
		{action: "remove", app: fakeApp, processName: "worker1", versionNumber: oldVersion.Version()},
		{action: "cleanup", app: fakeApp, versionNumber: newVersion.Version()},
	})
}

func (s *S) TestRemoveOldServicesWithAutoscaleCleanup(c *check.C) {
	ctx := context.Background()

	autoscaleProv := &provisiontest.AutoScaleProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}
	oldProvisioner := provision.DefaultProvisioner
	provision.DefaultProvisioner = "autoscaleProv"
	provision.Register("autoscaleProv", func() (provision.Provisioner, error) {
		return autoscaleProv, nil
	})
	defer func() {
		provision.Unregister("autoscaleProv")
		provision.DefaultProvisioner = oldProvisioner
	}()

	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	fakeApp.Pool = "" // to trigger default provisioner

	err := autoscaleProv.SetAutoScale(ctx, fakeApp, provTypes.AutoScaleSpec{
		Process:    "worker1",
		AverageCPU: "300m",
		MaxUnits:   10,
		MinUnits:   2,
	})
	c.Assert(err, check.IsNil)

	autoscales, err := autoscaleProv.GetAutoScale(ctx, fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(autoscales, check.HasLen, 1)
	c.Assert(autoscales[0].Process, check.Equals, "worker1")

	m := &recordManager{}
	oldVersion := newSuccessfulVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web1",
			"worker1": "python worker1",
		},
	})
	newVersion := newVersion(c, fakeApp, nil)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newVersion:       newVersion,
		newVersionSpec:   ProcessSpec{"web": ProcessState{Increment: 5}},
		oldVersion:       oldVersion,
		oldVersionNumber: oldVersion.Version(),
	}

	_, err = removeOldServices.Forward(action.FWContext{Context: ctx, Params: []interface{}{args}})
	c.Assert(err, check.IsNil)

	autoscales, err = autoscaleProv.GetAutoScale(ctx, fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(autoscales, check.HasLen, 0)

	c.Assert(m.calls, check.HasLen, 2)
	c.Assert(m.calls[0].action, check.Equals, "remove")
	c.Assert(m.calls[0].processName, check.Equals, "worker1")
}

func (s *S) TestRemoveOldServicesWithMultipleAutoscales(c *check.C) {
	ctx := context.Background()

	autoscaleProv := &provisiontest.AutoScaleProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}
	oldProvisioner := provision.DefaultProvisioner
	provision.DefaultProvisioner = "autoscaleProv"
	provision.Register("autoscaleProv", func() (provision.Provisioner, error) {
		return autoscaleProv, nil
	})
	defer func() {
		provision.Unregister("autoscaleProv")
		provision.DefaultProvisioner = oldProvisioner
	}()

	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	fakeApp.Pool = "" // to trigger default provisioner

	err := autoscaleProv.SetAutoScale(ctx, fakeApp, provTypes.AutoScaleSpec{
		Process:    "web",
		AverageCPU: "500m",
		MaxUnits:   5,
		MinUnits:   1,
	})
	c.Assert(err, check.IsNil)

	err = autoscaleProv.SetAutoScale(ctx, fakeApp, provTypes.AutoScaleSpec{
		Process:    "worker1",
		AverageCPU: "300m",
		MaxUnits:   10,
		MinUnits:   2,
	})
	c.Assert(err, check.IsNil)

	autoscales, err := autoscaleProv.GetAutoScale(ctx, fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(autoscales, check.HasLen, 2)

	m := &recordManager{}
	oldVersion := newSuccessfulVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web1",
			"worker1": "python worker1",
		},
	})
	newVersion := newVersion(c, fakeApp, nil)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newVersion:       newVersion,
		newVersionSpec:   ProcessSpec{"web": ProcessState{Increment: 5}}, // Keep only web
		oldVersion:       oldVersion,
		oldVersionNumber: oldVersion.Version(),
	}

	// Remove worker1 process (keep web)
	_, err = removeOldServices.Forward(action.FWContext{Context: ctx, Params: []interface{}{args}})
	c.Assert(err, check.IsNil)

	autoscales, err = autoscaleProv.GetAutoScale(ctx, fakeApp)
	c.Assert(err, check.IsNil)
	c.Assert(autoscales, check.HasLen, 1)
	c.Assert(autoscales[0].Process, check.Equals, "web")
	c.Assert(autoscales[0].AverageCPU, check.Equals, "500m")

	c.Assert(m.calls, check.HasLen, 2)
	c.Assert(m.calls[0].action, check.Equals, "remove")
	c.Assert(m.calls[0].processName, check.Equals, "worker1")
}

func (s *S) TestRemoveOldServicesWithNonAutoscaleProvisioner(c *check.C) {
	m := &recordManager{}
	fakeApp := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	oldVersion := newSuccessfulVersion(c, fakeApp, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":     "python web1",
			"worker1": "python worker1",
		},
	})
	newVersion := newVersion(c, fakeApp, nil)
	args := &pipelineArgs{
		manager:          m,
		app:              fakeApp,
		newVersion:       newVersion,
		newVersionSpec:   ProcessSpec{"web": ProcessState{Increment: 5}},
		oldVersion:       oldVersion,
		oldVersionNumber: oldVersion.Version(),
	}

	// Should not crash even though provisioner doesn't support autoscaling
	_, err := removeOldServices.Forward(action.FWContext{Context: context.TODO(), Params: []interface{}{args}})
	c.Assert(err, check.IsNil)

	c.Assert(m.calls, check.HasLen, 2)
	c.Assert(m.calls[0].action, check.Equals, "remove")
	c.Assert(m.calls[0].processName, check.Equals, "worker1")
}

func (s *S) TestRunServicePipelineUpdateStates(c *check.C) {
	m := &recordManager{}
	a := provisiontest.NewFakeApp("myapp", "whitespace", 1)
	newVersion := newVersion(c, a, map[string]interface{}{
		"processes": map[string]interface{}{
			"p1": "cm1",
		},
	})
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
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(replicas, check.Equals, 0)
				c.Assert(ls.IsStopped(), check.Equals, true)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Start: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(replicas, check.Equals, 1)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {Restart: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(replicas, check.Equals, 1)
				c.Assert(ls.IsStopped(), check.Equals, false)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: -1}, {Stop: true},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(ls.IsStopped(), check.Equals, true)
			},
		},
		{
			states: []ProcessState{
				{Start: true}, {Increment: 2}, {Stop: true}, {},
			},
			fn: func(replicas int, ls *provision.LabelSet) {
				c.Assert(replicas, check.Equals, 0)
				c.Assert(ls.IsStopped(), check.Equals, true)
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
	for i, tt := range tests {
		c.Logf("test %d", i)
		for _, s := range tt.states {
			m.reset()
			err := RunServicePipeline(context.TODO(), m, newVersion.Version(), provision.DeployArgs{
				App:     a,
				Version: newVersion,
			}, ProcessSpec{
				"p1": s,
			})
			c.Assert(err, check.IsNil)
			c.Assert(m.calls, check.HasLen, 2)
			c.Assert(m.calls[0].action, check.Equals, "deploy")
			c.Assert(m.calls[1].action, check.Equals, "cleanup")
			m.lastLabels = map[string]*provision.LabelSet{
				"p1-v1": m.calls[0].labels,
			}
			rep := int32(m.calls[0].replicas)
			m.lastReplicas = map[string]*int32{
				"p1-v1": &rep,
			}
		}
		c.Assert(m.calls, check.HasLen, 2)
		c.Assert(m.calls[0].action, check.Equals, "deploy")
		c.Assert(m.calls[1].action, check.Equals, "cleanup")
		tt.fn(m.calls[0].replicas, m.calls[0].labels)
		m.reset()
		m.lastLabels = nil
		m.lastReplicas = nil
	}
}
