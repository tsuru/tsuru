// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/tsuru/tsuru/log"
)

type Logger interface {
	Log(string, string, string) error
}

type LogWriter struct {
	App    Logger
	Source string
	msgCh  chan []byte
	doneCh chan bool
	closed int32
}

func (w *LogWriter) Async() {
	w.msgCh = make(chan []byte, 1000)
	w.doneCh = make(chan bool)
	go func() {
		defer close(w.doneCh)
		for msg := range w.msgCh {
			err := w.write(msg)
			if err != nil {
				log.Errorf("[LogWriter] failed to write async logs: %s", err)
			}
		}
	}()
}

func (w *LogWriter) Close() {
	atomic.StoreInt32(&w.closed, 1)
	if w.msgCh != nil {
		close(w.msgCh)
	}
}

func (w *LogWriter) Wait(timeout time.Duration) error {
	if w.msgCh == nil {
		return nil
	}
	select {
	case <-w.doneCh:
	case <-time.After(timeout):
		return errors.New("timeout waiting for writer to finish")
	}
	return nil
}

// Write writes and logs the data.
func (w *LogWriter) Write(data []byte) (int, error) {
	if atomic.LoadInt32(&w.closed) == 1 {
		return len(data), nil
	}
	if w.msgCh == nil {
		return len(data), w.write(data)
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	w.msgCh <- copied
	return len(data), nil
}

func (w *LogWriter) write(data []byte) error {
	source := w.Source
	if source == "" {
		source = "tsuru"
	}
	return w.App.Log(string(data), source, "api")
}
