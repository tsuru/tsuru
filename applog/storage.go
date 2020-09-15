// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"context"
	"strings"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type storageLogService struct {
	dispatcher *logDispatcher
	storage    appTypes.AppLogStorage
}

func storageAppLogService() (appTypes.AppLogService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	queueSize, _ := config.GetInt("server:app-log-buffer-size")
	if queueSize == 0 {
		queueSize = 500000
	}
	s := &storageLogService{
		dispatcher: newlogDispatcher(queueSize, dbDriver.AppLogStorage),
		storage:    dbDriver.AppLogStorage,
	}
	shutdown.Register(s)
	return s, nil
}

func (s *storageLogService) Enqueue(entry *appTypes.Applog) error {
	return s.dispatcher.send(entry)
}

func (s *storageLogService) Add(appName, message, source, unit string) error {
	messages := strings.Split(message, "\n")
	logs := make([]*appTypes.Applog, 0, len(messages))
	for _, msg := range messages {
		if msg != "" {
			l := &appTypes.Applog{
				Date:    time.Now().In(time.UTC),
				Message: msg,
				Source:  source,
				AppName: appName,
				Unit:    unit,
			}
			logs = append(logs, l)
		}
	}
	if len(logs) == 0 {
		return nil
	}
	return s.storage.InsertApp(appName, logs...)
}

func (s *storageLogService) List(ctx context.Context, filters appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	if filters.Limit < 0 {
		return []appTypes.Applog{}, nil
	}
	return s.storage.List(ctx, filters)
}

func (s *storageLogService) Watch(ctx context.Context, filters appTypes.ListLogArgs) (appTypes.LogWatcher, error) {
	return s.storage.Watch(ctx, filters)
}

func (s *storageLogService) Shutdown(ctx context.Context) error {
	return s.dispatcher.shutdown(ctx)
}
