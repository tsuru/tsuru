// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/log"
	"sync"
	"sync/atomic"
	"time"
)

const (
	stopped int32 = iota
	running
	stopping
)

// Handler is a thread safe generic handler for queue messages.
//
// When started, whenever a new message arrives, handler invokes F, giving the
// message as parameter. F is invoked in its own goroutine, so the handler can
// handle other messages as they arrive.
type Handler struct {
	F     func(*Message)
	state int32
	id    string
}

// Start starts the handler. It's safe to call this function multiple times.
func (h *Handler) Start() {
	r.add(h)
	if atomic.CompareAndSwapInt32(&h.state, stopped, running) {
		go h.loop()
	}
}

// DryRun changes the state of the handler, but does not start it.
//
// It's intended for using in tests. It returns an error if the handler is not
// stopped.
func (h *Handler) DryRun() error {
	if !atomic.CompareAndSwapInt32(&h.state, stopped, running) {
		return errors.New("Handler is not stopped.")
	}
	r.add(h)
	go h.fakeLoop()
	return nil
}

// Stop sends a signal to stop the handler, it won't stop the handler
// immediately. After calling Stop, one should call Wait for blocking until the
// handler is stopped.
//
// This method will return an error if the handler is not running.
func (h *Handler) Stop() error {
	if !atomic.CompareAndSwapInt32(&h.state, running, stopping) {
		return errors.New("Not running.")
	}
	r.remove(h)
	return nil
}

// Wait blocks until the handler is stopped.
func (h *Handler) Wait() {
	for atomic.LoadInt32(&h.state) != stopped {
		time.Sleep(1e3)
	}
}

func (h *Handler) fakeLoop() {
	for atomic.LoadInt32(&h.state) == running {
		time.Sleep(1e3)
	}
	atomic.StoreInt32(&h.state, stopped)
}

// loop will get messages from the queue and dispatch them to Handler.F.
func (h *Handler) loop() {
	for {
		if message, err := Get(1e9); err == nil {
			go h.F(message)
		} else if atomic.LoadInt32(&h.state) == running {
			log.Printf("Failed to get message from the queue: %s. Trying again...", err)
			continue
		} else {
			atomic.StoreInt32(&h.state, stopped)
			return
		}
	}
}

// registry stores references to all running handlers.
type registry struct {
	mut      sync.Mutex
	handlers map[string]*Handler
}

func newRegistry() *registry {
	return &registry{
		handlers: make(map[string]*Handler),
	}
}

func (r *registry) add(h *Handler) {
	if h.id == "" {
		var buf [16]byte
		rand.Read(buf[:])
		h.id = fmt.Sprintf("%x", buf)
	}
	r.mut.Lock()
	r.handlers[h.id] = h
	r.mut.Unlock()
}

func (r *registry) remove(h *Handler) {
	if h.id != "" {
		r.mut.Lock()
		delete(r.handlers, h.id)
		r.mut.Unlock()
	}
}

var r *registry = newRegistry()

// Preempt calls Stop and Wait for each running handler.
func Preempt() {
	var wg sync.WaitGroup
	r.mut.Lock()
	preemptable := make(map[string]*Handler, len(r.handlers))
	for k, v := range r.handlers {
		preemptable[k] = v
	}
	r.mut.Unlock()
	wg.Add(len(preemptable))
	for _, h := range preemptable {
		go func(h *Handler) {
			defer wg.Done()
			h.Stop()
			h.Wait()
		}(h)
	}
	wg.Wait()
}
