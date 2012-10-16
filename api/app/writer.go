// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"net/http"
	"regexp"
)

// FilteredWriter is a custom writer
// that filter deprecation warnings and juju log output.
type FilteredWriter struct {
	w http.ResponseWriter
}

// WriteHeader calls the w.Header
func (w *FilteredWriter) Header() http.Header {
	return w.w.Header()
}

// Write writes the data, filtering the juju warnings.
func (w *FilteredWriter) Write(data []byte) (int, error) {
	_, err := w.w.Write(filterOutput(data))
	// returning the len(data) to skip the 'short write' error
	return len(data), err
}

// WriteHeader calls the w.WriteHeader
func (w *FilteredWriter) WriteHeader(code int) {
	w.w.WriteHeader(code)
}

// filterOutput filters output from juju.
//
// It removes all lines that does not represent useful output, like juju's
// logging and Python's deprecation warnings.
func filterOutput(output []byte) []byte {
	var result [][]byte
	var ignore bool
	deprecation := []byte("DeprecationWarning")
	regexLog := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}`)
	regexSshWarning := regexp.MustCompile(`^Warning: Permanently added`)
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
		if !regexSshWarning.Match(line) && !regexLog.Match(line) {
			result = append(result, line)
		}
	}
	return bytes.Join(result, []byte{'\n'})
}
