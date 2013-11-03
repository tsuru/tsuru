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
	l := &FakeLogger{Buf: bytes.Buffer{}}
	log.SetLogger(l)
	return l
}

type FakeLogger struct {
	Buf bytes.Buffer
}

func (l *FakeLogger) Error(o string) {
	l.Buf.WriteString(fmt.Sprintf("%s\n", o))
}

func (l *FakeLogger) Errorf(format string, o ...interface{}) {
	l.Error(fmt.Sprintf(format, o...))
}

func (l *FakeLogger) Fatal(o string) {
	l.Error(o)
}

func (l *FakeLogger) Fatalf(format string, o ...interface{}) {
	l.Error(fmt.Sprintf(format, o...))
}

func (l *FakeLogger) Debug(o string) {
	l.Error(o)
}

func (l *FakeLogger) Debugf(format string, o ...interface{}) {
	l.Error(fmt.Sprintf(format, o...))
}
