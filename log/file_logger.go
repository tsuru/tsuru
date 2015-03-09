// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"fmt"
	"io"
	"log"
	"os"
)

var (
	errorPrefix = "ERROR: %s"
	fatalPrefix = "FATAL: %s"
	debugPrefix = "DEBUG: %s"
)

func NewFileLogger(fileName string, debug bool) Logger {
	file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		panic(err)
	}
	return newWriterLogger(file, debug)
}

func newWriterLogger(writer io.Writer, debug bool) Logger {
	logger := log.New(writer, "", log.LstdFlags)
	return &fileLogger{logger: logger, debug: debug}
}

type fileLogger struct {
	logger *log.Logger
	debug  bool
}

func (l *fileLogger) Error(o string) {
	l.logger.Printf(errorPrefix, o)
}

func (l *fileLogger) Errorf(format string, o ...interface{}) {
	l.Error(fmt.Sprintf(format, o...))
}

func (l *fileLogger) Fatal(o string) {
	l.logger.Printf(fmt.Sprintf(fatalPrefix, o))
	os.Exit(1)
}

func (l *fileLogger) Fatalf(format string, o ...interface{}) {
	l.Fatal(fmt.Sprintf(format, o...))
}

func (l *fileLogger) Debug(o string) {
	if l.debug {
		l.logger.Printf(debugPrefix, o)
	}
}

func (l *fileLogger) Debugf(format string, o ...interface{}) {
	l.Debug(fmt.Sprintf(format, o...))
}

func (l *fileLogger) GetStdLogger() *log.Logger {
	return l.logger
}
