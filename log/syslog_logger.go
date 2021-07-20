// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows

package log

import (
	"fmt"
	"log"
	"log/syslog"
	"os"
)

var _ Logger = (*syslogLogger)(nil)

func NewSyslogLogger(tag string, debug bool) (Logger, error) {
	priority := syslog.LOG_LOCAL0 | syslog.LOG_INFO
	w, err := syslog.New(priority, tag)
	if err != nil {
		return nil, err
	}
	return &syslogLogger{w: w, debug: debug}, nil
}

type syslogLogger struct {
	w     *syslog.Writer
	debug bool
}

func (l *syslogLogger) Error(o string) {
	l.w.Err(o)
}

func (l *syslogLogger) Errorf(format string, o ...interface{}) {
	l.w.Err(fmt.Sprintf(format, o...))
}

func (l *syslogLogger) Fatal(o string) {
	l.w.Err(fmt.Sprintf(fatalPrefix, o))
	os.Exit(1)
}

func (l *syslogLogger) Fatalf(format string, o ...interface{}) {
	l.Fatal(fmt.Sprintf(format, o...))
}

func (l *syslogLogger) Debug(o string) {
	if l.debug {
		l.w.Debug(o)
	}
}

func (l *syslogLogger) Debugf(format string, o ...interface{}) {
	l.Debug(fmt.Sprintf(format, o...))
}

func (l *syslogLogger) GetStdLogger() *log.Logger {
	return log.New(l.w, "", 0)
}
