// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/servicemanager"
)

type LogWriter struct {
	AppName string
	Source  string
	msgCh   chan []byte
	doneCh  chan bool
	closed  bool
	finLk   sync.RWMutex
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
	w.finLk.Lock()
	defer w.finLk.Unlock()
	w.closed = true
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
	w.finLk.RLock()
	defer w.finLk.RUnlock()
	if w.closed {
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
	return servicemanager.AppLog.Add(w.AppName, string(data), source, "api")
}
