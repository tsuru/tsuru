// Copyright 2014 Globo.com. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"fmt"
	"io"
)

type Checker func() error

type warningErr struct{ msg string }

func (e *warningErr) Error() string {
	return e.msg
}

func NewWarning(msg string) error {
	return &warningErr{msg: msg}
}

// Check a parsed config file and consider warnings as errors.
func Check(checkers []Checker) error {
	return CheckWithWarnings(checkers, nil)
}

// Check a parsed config file and writes warnings to received writer.
func CheckWithWarnings(checkers []Checker, warningWriter io.Writer) error {
	for _, check := range checkers {
		err := check()
		if _, isWarn := err.(*warningErr); warningWriter != nil && isWarn {
			fmt.Fprintf(warningWriter, "WARNING: %s\n", err)
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}
