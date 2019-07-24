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

	"github.com/docker/docker/pkg/jsonmessage"
	"golang.org/x/crypto/ssh/terminal"
)

type streamWriter struct {
	w         io.Writer
	b         []byte
	formatter Formatter
}

type syncWriter struct {
	w  io.Writer
	mu sync.Mutex
}

func (w *syncWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(b)
}

func (w *syncWriter) FD() uintptr {
	fd := 0
	switch v := w.w.(type) {
	case withFd:
		fd = int(v.Fd())
	case withFD:
		fd = int(v.FD())
	}
	return uintptr(fd)
}

var ErrInvalidStreamChunk = errors.New("invalid stream chunk")

type Formatter interface {
	Format(out io.Writer, data []byte) error
}

func NewStreamWriter(w io.Writer, formatter Formatter) *streamWriter {
	if formatter == nil {
		formatter = &SimpleJsonMessageFormatter{}
	}
	return &streamWriter{w: &syncWriter{w: w}, formatter: formatter}
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
	Message string
	Error   string `json:",omitempty"`
}

type SimpleJsonMessageFormatter struct {
	pipeReader io.Reader
	pipeWriter io.WriteCloser
	done       chan struct{}
}

func likeJSON(str string) bool {
	data := bytes.TrimSpace([]byte(str))
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
		f.pipeReader = nil
		<-f.done
	}
	return nil
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
	if likeJSON(msg.Message) {
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
			f.done = make(chan struct{})
			go func() {
				defer close(f.done)
				jsonmessage.DisplayJSONMessagesStream(f.pipeReader, out, uintFD, isTerm, nil)
			}()
		}
		f.pipeWriter.Write([]byte(msg.Message))
	} else {
		f.Close()
		out.Write([]byte(msg.Message))
	}
	return nil
}

type SimpleJsonMessageEncoderWriter struct {
	*json.Encoder
}

func (w *SimpleJsonMessageEncoderWriter) Write(msg []byte) (int, error) {
	err := w.Encode(SimpleJsonMessage{Message: string(msg)})
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
