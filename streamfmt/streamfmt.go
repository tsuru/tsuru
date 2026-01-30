// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package streamfmt

import (
	"fmt"
	"io"
	"strings"
)

const (
	SectionPrefix = "---- "
	SectionSuffix = " ----"

	ActionPrefix = " ---> "

	ErrorPrefix = "**** "
	ErrorSuffix = " ****"
)

func Section(text string) string {
	return SectionPrefix + text + SectionSuffix
}

func Action(text string) string {
	return ActionPrefix + text
}

func Error(text string) string {
	return ErrorPrefix + strings.ToUpper(text) + ErrorSuffix
}

// Formatted versions

func Sectionf(format string, a ...interface{}) string {
	return SectionPrefix + fmt.Sprintf(format, a...) + SectionSuffix
}

func Actionf(format string, a ...interface{}) string {
	return ActionPrefix + fmt.Sprintf(format, a...)
}

func Errorf(format string, a ...interface{}) string {
	return ErrorPrefix + strings.ToUpper(fmt.Sprintf(format, a...)) + ErrorSuffix
}

// io.Writer versions

func FprintSectionf(w io.Writer, format string, a ...interface{}) {
	fmt.Fprint(w, Sectionf(format, a...))
}

func FprintActionf(w io.Writer, format string, a ...interface{}) {
	fmt.Fprint(w, Actionf(format, a...))
}

func FprintErrorf(w io.Writer, format string, a ...interface{}) {
	fmt.Fprint(w, Errorf(format, a...))
}

// io.Writer versions with newline

func FprintlnSectionf(w io.Writer, format string, a ...interface{}) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, Sectionf(format, a...))
}

func FprintlnActionf(w io.Writer, format string, a ...interface{}) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, Actionf(format, a...))
}

func FprintlnErrorf(w io.Writer, format string, a ...interface{}) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, Errorf(format, a...))
}
