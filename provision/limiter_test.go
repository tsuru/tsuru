// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"gopkg.in/check.v1"
	"time"
)

func (s *S) TestLocalLimiterAddDone(c *check.C) {
	l := LocalLimiter{}
	l.SetLimit(3)
	l.Add("node1")
	l.Add("node1")
	l.Add("node1")
	done := make(chan bool)
	go func() {
		l.Add("node1")
		close(done)
	}()
	select {
	case <-done:
		c.Fatal("add should have blocked")
	case <-time.After(100 * time.Millisecond):
	}
	l.Add("node2")
	l.Done("node1")
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		c.Fatal("timed out waiting for unblock")
	}
}
