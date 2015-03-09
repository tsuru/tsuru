// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"log"
	"os"
)

func NewMultiLogger(loggers ...Logger) Logger {
	return &multiLogger{loggers: loggers}
}

type multiLogger struct {
	loggers []Logger
}

func (m *multiLogger) Debug(message string) {
	for _, logger := range m.loggers {
		logger.Debug(message)
	}
}

func (m *multiLogger) Error(message string) {
	for _, logger := range m.loggers {
		logger.Error(message)
	}
}

func (m *multiLogger) Fatal(message string) {
	for _, logger := range m.loggers {
		logger.Error(message)
	}
	os.Exit(1)
}

func (m *multiLogger) Debugf(format string, v ...interface{}) {
	for _, logger := range m.loggers {
		logger.Debugf(format, v...)
	}
}

func (m *multiLogger) Errorf(format string, v ...interface{}) {
	for _, logger := range m.loggers {
		logger.Errorf(format, v...)
	}
}

func (m *multiLogger) Fatalf(format string, v ...interface{}) {
	for _, logger := range m.loggers {
		logger.Errorf(format, v...)
	}
	os.Exit(1)
}

func (m *multiLogger) GetStdLogger() *log.Logger {
	return m.loggers[0].GetStdLogger()
}
