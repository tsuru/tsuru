// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package log

import (
	"log"
)

func NewMultiLogger(loggers ...*Logger) Logger {
	return &multiLogger{loggers: loggers}
}

type multiLogger struct {
	loggers []*Logger
}

func (m *multiLogger) Debug(format string) {
	for _, logger := range m.loggers {
		(*logger).Debug(format)
	}
}

func (m *multiLogger) Error(format string) {
	for _, logger := range m.loggers {
		(*logger).Debug(format)
	}
}

func (m *multiLogger) Fatal(format string) {
	for _, logger := range m.loggers {
		(*logger).Debug(format)
	}
}

func (m *multiLogger) Debugf(format string, v ...interface{}) {
	for _, logger := range m.loggers {
		(*logger).Debugf(format, v...)
	}
}

func (m *multiLogger) Errorf(format string, v ...interface{}) {
	for _, logger := range m.loggers {
		(*logger).Errorf(format, v...)
	}
}

func (m *multiLogger) Fatalf(format string, v ...interface{}) {
	for _, logger := range m.loggers {
		(*logger).Fatalf(format, v...)
	}
}

func (m *multiLogger) GetStdLogger() *log.Logger {
	return (*m.loggers[0]).GetStdLogger()
}
