// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package log provides logging utility.
//
// It abstracts the Logger from the standard log package, allowing the
// developer to patck the logging target, changing this to a file, or syslog,
// for example.
package log

import "log"

// Target is the current target for the log package.
var Target *log.Logger

// Fatal is equivalent to Print() followed by os.Exit(1).
func Fatal(v ...interface{}) {
	if Target != nil {
		Target.Fatal(v...)
	}
}

// Print is similar to fmt.Print, writing the given values to the Target
// logger.
func Print(v ...interface{}) {
	if Target != nil {
		Target.Print(v...)
	}
}

// Printf is similar to fmt.Printf, writing the formatted string to the Target
// logger.
func Printf(format string, v ...interface{}) {
	if Target != nil {
		Target.Printf(format, v...)
	}
}

// Panic is equivalent to Print() followed by panic().
func Panic(v ...interface{}) {
	if Target != nil {
		Target.Panic(v...)
	}
}

// Panicf is equivalent to Printf() followed by panic().
func Panicf(format string, v ...interface{}) {
	if Target != nil {
		Target.Panicf(format, v...)
	}
}
