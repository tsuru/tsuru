// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"sync"

	appTypes "github.com/tsuru/tsuru/types/app"
)

type logStreamTracker struct {
	sync.Mutex
	conn map[appTypes.LogWatcher]struct{}
}

func (t *logStreamTracker) add(l appTypes.LogWatcher) {
	t.Lock()
	defer t.Unlock()
	if t.conn == nil {
		t.conn = make(map[appTypes.LogWatcher]struct{})
	}
	t.conn[l] = struct{}{}
}

func (t *logStreamTracker) remove(l appTypes.LogWatcher) {
	t.Lock()
	defer t.Unlock()
	if t.conn == nil {
		t.conn = make(map[appTypes.LogWatcher]struct{})
	}
	delete(t.conn, l)
}

func (t *logStreamTracker) String() string {
	return "log pub/sub connections"
}

func (t *logStreamTracker) Shutdown(ctx context.Context) error {
	t.Lock()
	defer t.Unlock()
	for l := range t.conn {
		l.Close()
	}
	return nil
}

var logTracker logStreamTracker
