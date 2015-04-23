// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"time"

	"github.com/tsuru/tsuru/app"
	"gopkg.in/check.v1"
)

func (s *S) TestLogStreamTrackerAddRemove(c *check.C) {
	c.Assert(logTracker.conn, check.HasLen, 0)
	l := app.LogListener{}
	logTracker.add(&l)
	c.Assert(logTracker.conn, check.HasLen, 1)
	logTracker.remove(&l)
	c.Assert(logTracker.conn, check.HasLen, 0)
}

func (s *S) TestLogStreamTrackerShutdown(c *check.C) {
	l, err := app.NewLogListener(&app.App{Name: "myapp"}, app.Applog{})
	c.Assert(err, check.IsNil)
	logTracker.add(l)
	logTracker.Shutdown()
	select {
	case <-l.C:
	case <-time.After(5 * time.Second):
		c.Fatal("timed out waiting for channel to close")
	}
}
