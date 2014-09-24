// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

type streamWriter struct {
	w         io.Writer
	b         []byte
	formatter Formatter
}

var ErrInvalidStreamChunk = errors.New("invalid stream chunk")

type Formatter interface {
	Format(out io.Writer, data []byte) error
}

func NewStreamWriter(w io.Writer, formatter Formatter) *streamWriter {
	return &streamWriter{w: w, formatter: formatter}
}

func (w *streamWriter) Remaining() []byte {
	return w.b
}

func (w *streamWriter) Write(b []byte) (int, error) {
	w.b = append(w.b, b...)
	writtenCount := len(b)
	for len(w.b) > 0 {
		parts := bytes.SplitAfterN(w.b, []byte("\n"), 2)
		err := w.formatter.Format(w.w, parts[0])
		if err != nil {
			if err == ErrInvalidStreamChunk {
				if len(parts) == 1 {
					return writtenCount, nil
				} else {
					err = fmt.Errorf("Unparseable chunk: %q", parts[0])
				}
			}
			return writtenCount, err
		}
		if len(parts) == 1 {
			w.b = []byte{}
		} else {
			w.b = parts[1]
		}
	}
	return writtenCount, nil
}
