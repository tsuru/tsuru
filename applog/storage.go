// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"context"
	"strings"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	appTypes "github.com/tsuru/tsuru/types/app"
)

func StorageAppLogService() (appTypes.AppLogService, error) {
	queueSize, _ := config.GetInt("server:app-log-buffer-size")
	if queueSize == 0 {
		queueSize = 500000
	}
	return &storageLogService{
		dispatcher: newlogDispatcher(queueSize),
	}, nil
}

type storageLogService struct {
	dispatcher *LogDispatcher
}

func (s *storageLogService) Enqueue(entry *appTypes.Applog) error {
	return s.dispatcher.Send(entry)
}

func (s *storageLogService) Add(appName, message, source, unit string) error {
	messages := strings.Split(message, "\n")
	logs := make([]interface{}, 0, len(messages))
	for _, msg := range messages {
		if msg != "" {
			l := appTypes.Applog{
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
	conn, err := db.LogConn()
	if err != nil {
		return err
	}
	defer conn.Close()
	coll, err := conn.CreateAppLogCollection(appName)
	if err != nil {
		return err
	}
	return coll.Insert(logs...)
}

func (s *storageLogService) List(filters appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	conn, err := db.LogConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	logs := []appTypes.Applog{}
	q := bson.M{}
	if filters.Source != "" {
		q["source"] = filters.Source
	}
	if filters.Unit != "" {
		q["unit"] = filters.Unit
	}
	if filters.InvertFilters {
		for k, v := range q {
			q[k] = bson.M{"$ne": v}
		}
	}
	err = conn.AppLogCollection(filters.AppName).Find(q).Sort("-$natural").Limit(filters.Limit).All(&logs)
	if err != nil {
		return nil, err
	}
	l := len(logs)
	for i := 0; i < l/2; i++ {
		logs[i], logs[l-1-i] = logs[l-1-i], logs[i]
	}
	return logs, nil
}

func (s *storageLogService) Watch(appName, source, unit string) (appTypes.LogWatcher, error) {
	listener, err := newLogListener(s, appName, appTypes.Applog{Source: source, Unit: unit})
	if err != nil {
		return nil, err
	}
	return listener, nil
}

func (s *storageLogService) Shutdown(ctx context.Context) error {
	return s.dispatcher.Shutdown(ctx)
}
