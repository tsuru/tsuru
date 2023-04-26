// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"context"
	"time"

	"github.com/tsuru/tsuru/types/auth"
)

type LogType string

const (
	LogTypeApp = LogType("app")
	LogTypeJob = LogType("job")
)

type LogabbleObject interface {
	GetName() string
	GetPool() string
}

type LogWatcher interface {
	Chan() <-chan LogEntry
	Close()
}

type LogService interface {
	Enqueue(entry *LogEntry) error
	Add(name string, tsuruType LogType, message, source, unit string) error
	List(ctx context.Context, args ListLogArgs) ([]LogEntry, error)
	Watch(ctx context.Context, args ListLogArgs) (LogWatcher, error)
}

type LogServiceProvision interface {
	Provision(name, tsuruType LogType) error
	CleanUp(name, tsuruType LogType) error
}

type ListLogArgs struct {
	Name         string
	Type         LogType
	Source       string
	Units        []string
	Limit        int
	InvertSource bool
	Token        auth.Token
}

type LogEntry struct {
	Date    time.Time
	Message string
	Source  string
	Name    string
	Type    LogType
	Unit    string
}
