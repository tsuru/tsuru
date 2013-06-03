// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	stopped int32 = iota
	running
	stopping
)

// executor will execute the inner function until Stop is called. It implements
// the Handler interface.
type executor struct {
	inner func()
	state int32
	id    string
}

func (e *executor) Start() {
	if atomic.CompareAndSwapInt32(&e.state, stopped, running) {
		r.add(e)
		go e.loop()
	}
}

func (e *executor) Stop() error {
	if !atomic.CompareAndSwapInt32(&e.state, running, stopping) {
		return errors.New("Not running.")
	}
	r.remove(e)
	return nil
}

// Wait blocks until the handler is stopped.
func (e *executor) Wait() {
	for atomic.LoadInt32(&e.state) != stopped {
		time.Sleep(1e3)
	}
}

func (e *executor) loop() {
	for atomic.LoadInt32(&e.state) == running {
		e.inner()
	}
	atomic.StoreInt32(&e.state, stopped)
}

// registry stores references to all running handlers.
type registry struct {
	mut      sync.Mutex
	handlers map[string]*executor
}

func newRegistry() *registry {
	return &registry{
		handlers: make(map[string]*executor),
	}
}

func (r *registry) add(e *executor) {
	if e.id == "" {
		var buf [16]byte
		rand.Read(buf[:])
		e.id = fmt.Sprintf("%x", buf)
	}
	r.mut.Lock()
	r.handlers[e.id] = e
	r.mut.Unlock()
}

func (r *registry) remove(e *executor) {
	if e.id != "" {
		r.mut.Lock()
		delete(r.handlers, e.id)
		r.mut.Unlock()
	}
}

var r = newRegistry()

// Preempt calls Stop and Wait for each running handler.
func Preempt() {
	var wg sync.WaitGroup
	r.mut.Lock()
	preemptable := make(map[string]*executor, len(r.handlers))
	for k, v := range r.handlers {
		preemptable[k] = v
	}
	r.mut.Unlock()
	wg.Add(len(preemptable))
	for _, e := range preemptable {
		go func(e *executor) {
			defer wg.Done()
			if err := e.Stop(); err == nil {
				e.Wait()
			}
		}(e)
	}
	wg.Wait()
}
