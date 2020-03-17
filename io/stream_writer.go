// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/docker/docker/pkg/jsonmessage"
	"golang.org/x/crypto/ssh/terminal"
)

type wrapWriter struct {
	w io.Writer
}

func (w *wrapWriter) FD() uintptr {
	fd := 0
	switch v := w.w.(type) {
	case withFd:
		fd = int(v.Fd())
	case withFD:
		fd = int(v.FD())
	}
	return uintptr(fd)
}

type streamWriter struct {
	wrapWriter
	b         []byte
	formatter Formatter
}

type syncWriter struct {
	wrapWriter
	mu sync.Mutex
}

func (w *syncWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(b)
}

var ErrInvalidStreamChunk = errors.New("invalid stream chunk")

type Formatter interface {
	Format(out io.Writer, data []byte) error
}

func NewStreamWriter(w io.Writer, formatter Formatter) *streamWriter {
	if formatter == nil {
		formatter = &SimpleJsonMessageFormatter{}
	}
	return &streamWriter{
		wrapWriter: wrapWriter{
			w: &tsWriter{
				wrapWriter:  wrapWriter{w: &syncWriter{wrapWriter: wrapWriter{w: w}}},
				needsPrefix: true,
			},
		},
		formatter: formatter,
	}
}

func (w *streamWriter) Close() error {
	if closeable, ok := w.formatter.(io.Closer); ok {
		return closeable.Close()
	}
	return nil
}

func (w *streamWriter) Remaining() []byte {
	return w.b
}

func (w *streamWriter) Write(b []byte) (int, error) {
	w.b = append(w.b, b...)
	writtenCount := len(b)
	for len(w.b) > 0 {
		parts := bytes.SplitN(w.b, []byte("\n"), 2)
		if len(parts) == 2 && len(parts[0]) == 0 {
			w.b = parts[1]
			continue
		}
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

type SimpleJsonMessage struct {
	Message   string
	Timestamp time.Time
	Error     string `json:",omitempty"`
}

type SimpleJsonMessageFormatter struct {
	pipeReader  io.Reader
	pipeWriter  io.WriteCloser
	done        chan struct{}
	NoTimestamp bool
}

func likeJSON(str []byte) bool {
	data := bytes.TrimSpace(str)
	return len(data) > 1 && data[0] == '{' && data[len(data)-1] == '}'
}

type withFd interface {
	Fd() uintptr
}

type withFD interface {
	FD() uintptr
}

func (f *SimpleJsonMessageFormatter) Close() error {
	if f.pipeWriter != nil {
		f.pipeWriter.Close()
		f.pipeWriter = nil
		<-f.done
		f.pipeReader = nil
	}
	return nil
}

type tsWriter struct {
	wrapWriter
	ts          time.Time
	mu          sync.Mutex
	needsPrefix bool
}

func (w *tsWriter) setTS(ts time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ts = ts
}

func (w *tsWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	ts := w.ts
	w.mu.Unlock()

	if ts.IsZero() {
		return w.w.Write(data)
	}
	if len(data) == 0 {
		return 0, nil
	}

	const timeFormat = "2006-01-02 15:04:05 -0700"
	prefix := []byte(ts.Local().Format(timeFormat) + ": ")

	if w.needsPrefix {
		w.w.Write(prefix)
	}

	nl := []byte("\n")
	pos := 0
	for {
		oldPos := pos
		newPos := bytes.Index(data[pos:], nl)
		if newPos == -1 {
			w.needsPrefix = false
			w.w.Write(data[oldPos:])
			break
		}
		pos += newPos
		if pos == len(data)-1 {
			w.needsPrefix = true
			w.w.Write(data[oldPos:])
			break
		}
		w.w.Write(data[oldPos:pos])
		w.w.Write(data[pos : pos+1])
		w.w.Write(prefix)
		pos++
	}
	return len(data), nil
}

func (f *SimpleJsonMessageFormatter) Format(out io.Writer, data []byte) error {
	if len(data) == 0 || (len(data) == 1 && data[0] == '\n') {
		return nil
	}
	var msg SimpleJsonMessage
	err := json.Unmarshal(data, &msg)
	if err != nil {
		return ErrInvalidStreamChunk
	}
	if msg.Error != "" {
		return errors.New(msg.Error)
	}
	parts := bytes.SplitAfter([]byte(msg.Message), []byte("\n"))
	for i, part := range parts {
		_ = i
		if len(part) == 0 {
			continue
		}
		isJSON := likeJSON(part)
		if !isJSON {
			f.Close()
		}
		if tsw, ok := out.(*tsWriter); ok && !f.NoTimestamp {
			tsw.setTS(msg.Timestamp)
		}
		err = f.formatMessagePart(out, part, isJSON)
		if err != nil {
			return err
		}
	}
	return nil
}

var mockIsTerm func() bool = nil

func (f *SimpleJsonMessageFormatter) formatMessagePart(out io.Writer, msg []byte, isJSON bool) error {
	if !isJSON {
		_, err := out.Write(msg)
		return err
	}
	if f.pipeWriter == nil {
		f.pipeReader, f.pipeWriter = io.Pipe()
		fd := -1
		switch v := out.(type) {
		case withFd:
			fd = int(v.Fd())
		case withFD:
			fd = int(v.FD())
		}
		var isTerm bool
		var uintFD uintptr
		if fd != -1 {
			isTerm = terminal.IsTerminal(fd)
			uintFD = uintptr(fd)
		}
		if mockIsTerm != nil {
			isTerm = mockIsTerm()
		}
		f.done = make(chan struct{})
		go func() {
			defer close(f.done)
			for {
				dispErr := jsonmessage.DisplayJSONMessagesStream(f.pipeReader, out, uintFD, isTerm, nil)
				if dispErr == nil || dispErr == io.EOF {
					return
				}
				fmt.Fprintf(out, "warning: log message lost due to parse error: %v\n", dispErr)
			}
		}()
	}
	_, err := f.pipeWriter.Write(msg)
	return err
}

type SimpleJsonMessageEncoderWriter struct {
	*json.Encoder
	now func() time.Time
}

func (w *SimpleJsonMessageEncoderWriter) Write(msg []byte) (int, error) {
	if w.now == nil {
		w.now = time.Now
	}
	err := w.Encode(SimpleJsonMessage{Message: string(msg), Timestamp: w.now().UTC()})
	if err != nil {
		return 0, err
	}
	return len(msg), nil
}

type DockerErrorCheckWriter struct {
	W io.Writer
	b []byte
}

type dockerJSONMessage struct {
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errorDetail,omitempty"`
	ErrorMessage string `json:"error"`
}

func (w *DockerErrorCheckWriter) Write(data []byte) (n int, err error) {
	n, err = w.W.Write(data)
	if err != nil {
		return
	}
	w.b = append(w.b, data...)
	if len(w.b) == 0 {
		return
	}
	parts := bytes.Split(w.b, []byte("\n"))
	w.b = parts[len(parts)-1]
	var msg dockerJSONMessage
	for _, part := range parts {
		jsonErr := json.Unmarshal(part, &msg)
		if jsonErr != nil {
			continue
		}
		if msg.Error != nil {
			return 0, errors.New(msg.Error.Message)
		}
		if msg.ErrorMessage != "" {
			return 0, errors.New(msg.ErrorMessage)
		}
	}
	return
}
