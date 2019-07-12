// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"time"

	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func (s *S) TestLogStreamTrackerAddRemove(c *check.C) {
	c.Assert(logTracker.conn, check.HasLen, 0)
	var l appTypes.LogWatcher = nil
	logTracker.add(l)
	c.Assert(logTracker.conn, check.HasLen, 1)
	logTracker.remove(l)
	c.Assert(logTracker.conn, check.HasLen, 0)
}

func (s *S) TestLogStreamTrackerShutdown(c *check.C) {
	l, err := servicemanager.AppLog.Watch("myapp", "", "", nil)
	c.Assert(err, check.IsNil)
	logTracker.add(l)
	logTracker.Shutdown(context.Background())
	select {
	case <-l.Chan():
	case <-time.After(5 * time.Second):
		c.Fatal("timed out waiting for channel to close")
	}
}
