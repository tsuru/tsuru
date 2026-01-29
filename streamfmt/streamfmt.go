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
	sectionPrefix = "---- "
	sectionSuffix = " ----"

	actionPrefix = " ---> "

	errorPrefix = "**** "
	errorSuffix = " ****"
)

func Section(text string) string {
	return sectionPrefix + text + sectionSuffix
}

func Action(text string) string {
	return actionPrefix + text
}

func Error(text string) string {
	return errorPrefix + strings.ToUpper(text) + errorSuffix
}

// Formatted versions

func Sectionf(format string, a ...interface{}) string {
	return sectionPrefix + fmt.Sprintf(format, a...) + sectionSuffix
}

func Actionf(format string, a ...interface{}) string {
	return actionPrefix + fmt.Sprintf(format, a...)
}

func Errorf(format string, a ...interface{}) string {
	return errorPrefix + strings.ToUpper(fmt.Sprintf(format, a...)) + errorSuffix
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
	fmt.Fprintln(w, Sectionf(format, a...))
}

func FprintlnActionf(w io.Writer, format string, a ...interface{}) {
	fmt.Fprintln(w, Actionf(format, a...))
}

func FprintlnErrorf(w io.Writer, format string, a ...interface{}) {
	fmt.Fprintln(w, Errorf(format, a...))
}
