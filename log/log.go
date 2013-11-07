// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package log provides logging utility.
//
// It abstracts the logger from the standard log package, allowing the
// developer to patck the logging target, changing this to a file, or syslog,
// for example.
package log

import (
	"github.com/globocom/config"
	"io"
	"sync"
)

type Logger interface {
	Error(string)
	Errorf(string, ...interface{})
	Fatal(string)
	Fatalf(string, ...interface{})
	Debug(string)
	Debugf(string, ...interface{})
}

func Init() {
	debug, err := config.GetBool("debug")
	if err != nil {
		debug = false
	}
	logFileName, err := config.GetString("log:file")
	var logger Logger
	if err != nil {
		logger = newSyslogLogger(debug)
	} else {
		logger = newFileLogger(logFileName, debug)
	}
	SetLogger(logger)
}

// Target is the current target for the log package.
type Target struct {
	logger Logger
	mut    sync.RWMutex
}

// SetLogger defines a new logger for the current target.
//
// See the builtin log package for more details.
func (t *Target) SetLogger(l Logger) {
	t.mut.Lock()
	defer t.mut.Unlock()
	t.logger = l
}

// Error writes the given values to the Target
// logger.
func (t *Target) Error(v string) {
	t.mut.RLock()
	defer t.mut.RUnlock()
	if t.logger != nil {
		t.logger.Error(v)
	}
}

// Errorf writes the formatted string to the Target
// logger.
func (t *Target) Errorf(format string, v ...interface{}) {
	t.mut.RLock()
	defer t.mut.RUnlock()
	if t.logger != nil {
		t.logger.Errorf(format, v...)
	}
}

// Fatal writes the given values to the Target
// logger.
func (t *Target) Fatal(v string) {
	t.mut.RLock()
	defer t.mut.RUnlock()
	if t.logger != nil {
		t.logger.Fatal(v)
	}
}

// Fatalf writes the formatted string to the Target
// logger.
func (t *Target) Fatalf(format string, v ...interface{}) {
	t.mut.RLock()
	defer t.mut.RUnlock()
	if t.logger != nil {
		t.logger.Fatalf(format, v...)
	}
}

// Debug writes the value to the Target
// logger.
func (t *Target) Debug(v string) {
	t.mut.RLock()
	defer t.mut.RUnlock()
	if t.logger != nil {
		t.logger.Debug(v)
	}
}

// Debugf writes the formatted string to the Target
// logger.
func (t *Target) Debugf(format string, v ...interface{}) {
	t.mut.RLock()
	defer t.mut.RUnlock()
	if t.logger != nil {
		t.logger.Debugf(format, v...)
	}
}

var DefaultTarget = new(Target)

// Error is a wrapper for DefaultTarget.Error.
func Error(v string) {
	DefaultTarget.Error(v)
}

// Errorf is a wrapper for DefaultTarget.Errorf.
func Errorf(format string, v ...interface{}) {
	DefaultTarget.Errorf(format, v...)
}

// Fatal is a wrapper for DefaultTarget.Fatal.
func Fatal(v string) {
	DefaultTarget.Fatal(v)
}

// Fatalf is a wrapper for DefaultTarget.Errorf.
func Fatalf(format string, v ...interface{}) {
	DefaultTarget.Fatalf(format, v...)
}

// Debug is a wrapper for DefaultTarget.Debug.
func Debug(v string) {
	DefaultTarget.Debug(v)
}

// Debugf is a wrapper for DefaultTarget.Debugf.
func Debugf(format string, v ...interface{}) {
	DefaultTarget.Debugf(format, v...)
}

// SetLogger is a wrapper for DefaultTarget.SetLogger.
func SetLogger(logger Logger) {
	DefaultTarget.SetLogger(logger)
}

func Write(w io.Writer, content []byte) error {
	n, err := w.Write(content)
	if err != nil {
		return err
	}
	if n != len(content) {
		return io.ErrShortWrite
	}
	return nil
}
