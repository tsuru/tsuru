// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"io"
	"time"

	"github.com/tsuru/tsuru/log"
)

type Logger interface {
	Log(string, string, string) error
}

type LogWriter struct {
	App    Logger
	Writer io.Writer
	Source string
	msgCh  chan []byte
	doneCh chan bool
}

func (w *LogWriter) Async() {
	w.msgCh = make(chan []byte, 1000)
	w.doneCh = make(chan bool)
	go func() {
		defer close(w.doneCh)
		for msg := range w.msgCh {
			_, err := w.write(msg)
			if err != nil {
				log.Errorf("[LogWriter] failed to write async logs: %s", err)
				return
			}
		}
	}()
}

func (w *LogWriter) Wait(timeout time.Duration) error {
	if w.msgCh == nil {
		return nil
	}
	close(w.msgCh)
	select {
	case <-w.doneCh:
	case <-time.After(timeout):
		return errors.New("timeout waiting for writer to finish")
	}
	return nil
}

// Write writes and logs the data.
func (w *LogWriter) Write(data []byte) (int, error) {
	if w.msgCh == nil {
		return w.write(data)
	}
	w.msgCh <- data
	return len(data), nil
}

func (w *LogWriter) write(data []byte) (int, error) {
	source := w.Source
	if source == "" {
		source = "tsuru"
	}
	err := w.App.Log(string(data), source, "api")
	if err != nil {
		return 0, err
	}
	return w.Writer.Write(data)
}
