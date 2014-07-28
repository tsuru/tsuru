// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package log

import (
	"fmt"
	"log"
	"log/syslog"
	"os"
)

func NewSyslogLogger(tag string, debug bool) Logger {
	w, err := syslog.New(syslog.LOG_INFO, tag)
	if err != nil {
		log.Fatal(err)
	}
	return &syslogLogger{w: w, debug: debug}
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
