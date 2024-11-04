// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"context"
	"sort"
	"time"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/job"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	jobTypes "github.com/tsuru/tsuru/types/job"
	logTypes "github.com/tsuru/tsuru/types/log"
	"gopkg.in/check.v1"
)

var _ = check.Suite(&ProvisionerWrapperSuite{})

type ProvisionerWrapperSuite struct {
	tsuruLogService    appTypes.AppLogService
	provisionerWrapper *provisionerWrapper
}

func newFakeJob(c *check.C) {
	_, err := servicemanager.Job.GetByName(context.TODO(), "j1")
	if err == jobTypes.ErrJobNotFound {
		collection, err := storagev2.JobsCollection()
		c.Assert(err, check.IsNil)
		fakeJob := jobTypes.Job{
			Name: "j1",
			Pool: "mypool",
		}
		_, err = collection.InsertOne(context.TODO(), fakeJob)
		c.Assert(err, check.IsNil)
	}
}

func (s *ProvisionerWrapperSuite) SetUpSuite(c *check.C) {
	provisioner := provisiontest.NewFakeProvisioner()
	provisioner.LogsEnabled = true
	var err error
	s.tsuruLogService, err = memoryAppLogService()
	c.Check(err, check.IsNil)

	s.provisionerWrapper = &provisionerWrapper{
		logService: s.tsuruLogService,
		provisionerGetter: func(ctx context.Context, obj *logTypes.LogabbleObject) (provision.LogsProvisioner, error) {
			return provisioner, nil
		},
	}
	servicemanager.App = &appTypes.MockAppService{
		Apps: []*appTypes.App{
			&appTypes.App{Name: "myapp", Pool: "mypool"},
		},
	}
	servicemanager.Job, err = job.JobService()
	c.Assert(err, check.IsNil)
	newFakeJob(c)
}

func (s *ProvisionerWrapperSuite) Test_List(c *check.C) {
	err := s.tsuruLogService.Enqueue(&appTypes.Applog{
		Name:    "myapp",
		Message: "Fake message from tsuru logs",
	})
	c.Check(err, check.IsNil)

	logs, err := s.provisionerWrapper.List(context.TODO(), appTypes.ListLogArgs{
		Name: "myapp",
	})
	sort.SliceStable(logs, func(i, j int) bool {
		return logs[i].Message < logs[j].Message
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 2)
	c.Check(logs[0].Message, check.Equals, "Fake message from provisioner")
	c.Check(logs[1].Message, check.Equals, "Fake message from tsuru logs")
}

func (s *ProvisionerWrapperSuite) Test_List_LogTypeJob(c *check.C) {
	logs, err := s.provisionerWrapper.List(context.TODO(), appTypes.ListLogArgs{
		Name: "j1",
		Type: logTypes.LogTypeJob,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Check(logs[0].Message, check.Equals, "Fake message from provisioner")
}

func (s *ProvisionerWrapperSuite) Test_Watch(c *check.C) {
	watcher, err := s.provisionerWrapper.Watch(context.TODO(), appTypes.ListLogArgs{
		Name: "myapp",
	})
	c.Assert(err, check.IsNil)
	err = s.tsuruLogService.Enqueue(&appTypes.Applog{
		Name:    "myapp",
		Message: "Fake message from tsuru logs",
	})
	c.Check(err, check.IsNil)
	logs := []appTypes.Applog{}
	ch := watcher.Chan()
	timer := time.After(time.Second)
loop:
	for {
		select {
		case log, ok := <-ch:
			if !ok {
				break loop
			}
			logs = append(logs, log)
			if len(logs) == 2 {
				break loop
			}
		case <-timer:
			break loop
		}
	}
	sort.SliceStable(logs, func(i, j int) bool {
		return logs[i].Message < logs[j].Message
	})
	c.Assert(logs, check.HasLen, 2)
	c.Check(logs[0].Message, check.Equals, "Fake message from provisioner")
	c.Check(logs[1].Message, check.Equals, "Fake message from tsuru logs")
}

func (s *ProvisionerWrapperSuite) Test_Watch_LogTypeJob(c *check.C) {
	newFakeJob(c)
	watcher, err := s.provisionerWrapper.Watch(context.TODO(), appTypes.ListLogArgs{
		Name: "j1",
		Type: logTypes.LogTypeJob,
	})
	c.Assert(err, check.IsNil)
	err = s.tsuruLogService.Enqueue(&appTypes.Applog{
		Name:    "j1",
		Message: "Fake message from tsuru logs",
	})
	c.Check(err, check.IsNil)
	logs := []appTypes.Applog{}
	ch := watcher.Chan()
	timer := time.After(time.Second)
loop:
	for {
		select {
		case log, ok := <-ch:
			if !ok {
				break loop
			}
			logs = append(logs, log)
			break loop
		case <-timer:
			break loop
		}
	}
	c.Assert(logs, check.HasLen, 1)
	c.Check(logs[0].Message, check.Equals, "Fake message from provisioner")
}

func (s *ProvisionerWrapperSuite) Test_MultiWatcher(c *check.C) {
	watcher1 := appTypes.NewMockLogWatcher()
	watcher2 := appTypes.NewMockLogWatcher()
	mw := newMultiWatcher(watcher1, watcher2)

	now := time.Now()
	watcher1.Enqueue(appTypes.Applog{Message: "from watcher 1", Date: now})
	watcher2.Enqueue(appTypes.Applog{Message: "from watcher 2", Date: now.Add(time.Second)})
	appLogs := []appTypes.Applog{}

	for {
		appLog := <-mw.Chan()
		appLogs = append(appLogs, appLog)
		if len(appLogs) == 2 {
			mw.Close()
			break
		}
	}

	sort.SliceStable(appLogs, func(i, j int) bool {
		return appLogs[i].Date.Before(appLogs[j].Date)
	})

	c.Check(appLogs[0].Message, check.Equals, "from watcher 1")
	c.Check(appLogs[1].Message, check.Equals, "from watcher 2")
}

func (s *ProvisionerWrapperSuite) Test_Instance(c *check.C) {
	provisioner := provisiontest.NewFakeProvisioner()
	provisioner.LogsEnabled = true
	memoryService, err := memoryAppLogService()
	c.Check(err, check.IsNil)

	pw := &provisionerWrapper{
		logService: memoryService,
		provisionerGetter: func(ctx context.Context, obj *logTypes.LogabbleObject) (provision.LogsProvisioner, error) {
			return provisioner, nil
		},
	}
	instanceService := pw.Instance()
	c.Check(instanceService, check.Equals, memoryService)

	aggregatorService := &aggregatorLogService{
		base: memoryService,
	}

	pw = &provisionerWrapper{
		logService: aggregatorService,
		provisionerGetter: func(ctx context.Context, obj *logTypes.LogabbleObject) (provision.LogsProvisioner, error) {
			return provisioner, nil
		},
	}
	instanceService = pw.Instance()
	c.Check(instanceService, check.Equals, memoryService)
}
