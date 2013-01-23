// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
)

// filterOutput filters output from juju.
//
// It removes all lines that do not contain useful output, like juju's logging
// and Python's deprecation warnings.
func filterOutput(output []byte) []byte {
	var result [][]byte
	var ignore bool
	deprecation := []byte("DeprecationWarning")
	regexLog := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}`)
	regexSshWarning := regexp.MustCompile(`^Warning: Permanently added`)
	regexPythonWarning := regexp.MustCompile(`^.*warnings.warn`)
	regexUserWarning := regexp.MustCompile(`^.*UserWarning`)
	lines := bytes.Split(output, []byte{'\n'})
	for _, line := range lines {
		if ignore {
			ignore = false
			continue
		}
		if bytes.Contains(line, deprecation) {
			ignore = true
			continue
		}
		if !regexSshWarning.Match(line) &&
			!regexLog.Match(line) &&
			!regexPythonWarning.Match(line) &&
			!regexUserWarning.Match(line) {
			result = append(result, line)
		}
	}
	return bytes.Join(result, []byte{'\n'})
}

// Writer is a custom writer that filters output from Juju.
//
// It ignores all Juju logging and Python warnings.
type Writer struct {
	io.Writer
}

// Write writes data to the underlying writer, filtering the juju warnings.
func (w *Writer) Write(data []byte) (int, error) {
	originalLength := len(data)
	if rw, ok := w.Writer.(http.ResponseWriter); ok {
		if rw.Header().Get("Content-Type") == "text" {
			data = filterOutput(data)
		}
	} else {
		data = filterOutput(data)
	}
	_, err := w.Writer.Write(data)
	// returning the len(data) to skip the "short write" error
	return originalLength, err
}
