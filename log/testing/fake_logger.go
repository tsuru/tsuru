// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package testing

import (
	"bytes"
	"fmt"
	"github.com/globocom/tsuru/log"
)

func NewFakeLogger() log.Logger {
	return &fakeLogger{Buf: bytes.Buffer{}}
}

type fakeLogger struct {
	Buf bytes.Buffer
}

func (l *fakeLogger) Error(o string) {
	l.Buf.WriteString(o)
}

func (l *fakeLogger) Errorf(format string, o ...interface{}) {
	l.Buf.WriteString(fmt.Sprintf(format, o...))
}

func (l *fakeLogger) Fatal(o string) {
	l.Buf.WriteString(o)
}

func (l *fakeLogger) Fatalf(format string, o ...interface{}) {
	l.Buf.WriteString(fmt.Sprintf(format, o...))
}

func (l *fakeLogger) Debug(o string) {
	l.Buf.WriteString(o)
}

func (l *fakeLogger) Debugf(format string, o ...interface{}) {
	l.Buf.WriteString(fmt.Sprintf(format, o...))
}
