// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import "sync/atomic"

// Counter implements a thread-safe, lock-free counter, that supports
// operations like increment and decrement.
//
// It uses an int64 internally, so all int64 boundaries also apply here.
type Counter struct {
	v int64
}

// NewCounter creates a new counter have the given value as the initial value.
func NewCounter(initial int64) *Counter {
	return &Counter{v: initial}
}

// Val returns the current value of the counter.
func (c *Counter) Val() int64 {
	return atomic.LoadInt64(&c.v)
}

// Increment increments the value of c by 1.
func (c *Counter) Increment() {
	atomic.AddInt64(&c.v, 1)
}

// Decrement decrements the value of c by 1.
func (c *Counter) Decrement() {
	atomic.AddInt64(&c.v, -1)
}
