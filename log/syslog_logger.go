// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package log

import (
	"fmt"
	"log"
	"log/syslog"
)

func newSyslogLogger() Logger {
	w, err := syslog.New(syslog.LOG_INFO, "tsuru")
	if err != nil {
		log.Fatal(err)
	}
	return &syslogLogger{w}
}

type syslogLogger struct {
	w *syslog.Writer
}

func (l *syslogLogger) Error(o string) {
	l.w.Err(o)
}

func (l *syslogLogger) Errorf(format string, o ...interface{}) {
	l.w.Err(fmt.Sprintf(format, o...))
}

func (l *syslogLogger) Debug(o string) {
	l.w.Debug(o)
}

func (l *syslogLogger) Debugf(format string, o ...interface{}) {
	l.w.Debug(fmt.Sprintf(format, o...))
}
