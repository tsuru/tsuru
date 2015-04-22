// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"sync"

	"github.com/tsuru/tsuru/app"
)

type logStreamTracker struct {
	sync.Mutex
	conn map[*app.LogListener]struct{}
}

func (t *logStreamTracker) add(l *app.LogListener) {
	t.Lock()
	defer t.Unlock()
	if t.conn == nil {
		t.conn = make(map[*app.LogListener]struct{})
	}
	t.conn[l] = struct{}{}
}

func (t *logStreamTracker) remove(l *app.LogListener) {
	t.Lock()
	defer t.Unlock()
	if t.conn == nil {
		t.conn = make(map[*app.LogListener]struct{})
	}
	delete(t.conn, l)
}

func (t *logStreamTracker) Shutdown() {
	t.Lock()
	defer t.Unlock()
	for l := range t.conn {
		l.Close()
	}
}

var logTracker logStreamTracker
