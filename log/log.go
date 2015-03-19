// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package log provides logging utility.
//
// It abstracts the logger from the standard log package, allowing the
// developer to pick the logging target, changing this to a file, or syslog,
// for example.
package log

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/tsuru/config"
)

type Logger interface {
	Error(string)
	Errorf(string, ...interface{})
	Fatal(string)
	Fatalf(string, ...interface{})
	Debug(string)
	Debugf(string, ...interface{})
	GetStdLogger() *log.Logger
}

func Init() {
	var loggers []Logger
	debug, _ := config.GetBool("debug")
	if logFileName, err := config.GetString("log:file"); err == nil {
		loggers = append(loggers, NewFileLogger(logFileName, debug))
	} else if err == config.ErrMismatchConf {
		panic(fmt.Sprintf("%s please see http://docs.tsuru.io/en/latest/reference/config.html#log-file", err))
	}
	if disableSyslog, _ := config.GetBool("log:disable-syslog"); !disableSyslog {
		tag, _ := config.GetString("log:syslog-tag")
		if tag == "" {
			tag = "tsr"
		}
		loggers = append(loggers, NewSyslogLogger(tag, debug))
	}
	if useStderr, _ := config.GetBool("log:use-stderr"); useStderr {
		loggers = append(loggers, newWriterLogger(os.Stderr, debug))
	}
	SetLogger(NewMultiLogger(loggers...))
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

// GetStdLogger returns a standard Logger instance
// useful for configuring log in external packages.
func (t *Target) GetStdLogger() *log.Logger {
	t.mut.RLock()
	defer t.mut.RUnlock()
	if t.logger != nil {
		return t.logger.GetStdLogger()
	}
	return nil
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

// GetStdLogger is a wrapper for DefaultTarget.GetStdLogger.
func GetStdLogger() *log.Logger {
	return DefaultTarget.GetStdLogger()
}

// SetLogger is a wrapper for DefaultTarget.SetLogger.
func SetLogger(logger Logger) {
	DefaultTarget.SetLogger(logger)
}

func WrapError(err error) error {
	if err != nil {
		Error(err.Error())
	}
	return err
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
