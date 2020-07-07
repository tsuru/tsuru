// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"sort"
	"time"

	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/types/app"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

var _ = check.Suite(&ProvisionerWrapperSuite{})

type ProvisionerWrapperSuite struct {
	tsuruLogService    appTypes.AppLogService
	provisionerWrapper *provisionerWrapper
}

func (s *ProvisionerWrapperSuite) SetUpSuite(c *check.C) {
	provisioner := provisiontest.NewFakeProvisioner()
	provisioner.LogsEnabled = true
	var err error
	s.tsuruLogService, err = memoryAppLogService()
	c.Check(err, check.IsNil)

	s.provisionerWrapper = &provisionerWrapper{
		logService: s.tsuruLogService,
		provisionerGetter: func(a appTypes.App) (provision.LogsProvisioner, error) {
			return provisioner, nil
		},
	}
	servicemanager.App = &app.MockAppService{
		Apps: []app.App{
			&app.MockApp{Name: "myapp", Pool: "mypool"},
		},
	}
}

func (s *ProvisionerWrapperSuite) Test_List(c *check.C) {
	err := s.tsuruLogService.Enqueue(&app.Applog{
		AppName: "myapp",
		Message: "Fake message from tsuru logs",
	})
	c.Check(err, check.IsNil)

	logs, err := s.provisionerWrapper.List(app.ListLogArgs{
		AppName: "myapp",
	})
	sort.SliceStable(logs, func(i, j int) bool {
		return logs[i].Message < logs[j].Message
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 2)
	c.Check(logs[0].Message, check.Equals, "Fake message from provisioner")
	c.Check(logs[1].Message, check.Equals, "Fake message from tsuru logs")
}

func (s *ProvisionerWrapperSuite) Test_Watch(c *check.C) {
	watcher, err := s.provisionerWrapper.Watch(app.ListLogArgs{
		AppName: "myapp",
	})
	c.Assert(err, check.IsNil)

	err = s.tsuruLogService.Enqueue(&app.Applog{
		AppName: "myapp",
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

func (s *ProvisionerWrapperSuite) Test_MultiWatcher(c *check.C) {
	watcher1 := app.NewMockLogWatcher()
	watcher2 := app.NewMockLogWatcher()
	mw := newMultiWatcher(watcher1, watcher2)

	now := time.Now()
	watcher1.Enqueue(app.Applog{Message: "from watcher 1", Date: now})
	watcher2.Enqueue(app.Applog{Message: "from watcher 2", Date: now.Add(time.Second)})
	appLogs := []app.Applog{}

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
