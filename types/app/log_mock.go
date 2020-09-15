// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import "context"

var _ AppLogService = &MockAppLogService{}

type MockAppLogService struct{}

func (m *MockAppLogService) Enqueue(entry *Applog) error {
	return nil
}
func (m *MockAppLogService) Add(appName, message, source, unit string) error {
	return nil
}

func (m *MockAppLogService) List(ctx context.Context, args ListLogArgs) ([]Applog, error) {
	return []Applog{}, nil
}

func (m *MockAppLogService) Watch(ctx context.Context, args ListLogArgs) (LogWatcher, error) {
	return NewMockLogWatcher(), nil
}

type MockLogWatcher struct {
	ch chan Applog
}

func (m *MockLogWatcher) Chan() <-chan Applog {
	return m.ch
}

func (m *MockLogWatcher) Close() {
	close(m.ch)
}

func (m *MockLogWatcher) Enqueue(log Applog) {
	m.ch <- log
}

func NewMockLogWatcher() *MockLogWatcher {
	return &MockLogWatcher{
		ch: make(chan Applog, 10),
	}
}
