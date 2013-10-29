// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package log

import (
	"log"
	"os"
)

var (
	errorPrefix = "ERROR: %s"
)

func newFileLogger(fileName string) Logger {
	file, _ := os.OpenFile(fileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	logger := log.New(file, "", log.LstdFlags)
	return &fileLogger{logger}
}

type fileLogger struct {
	logger *log.Logger
}

func (l *fileLogger) Error(o string) {
	l.logger.Printf(errorPrefix, o)
}

func (l *fileLogger) Errorf(format string, o ...interface{}) {}
func (l *fileLogger) Debug(o string)                         {}
func (l *fileLogger) Debugf(format string, o ...interface{}) {}
