// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net"
	"net/http"
	"sync"
)

type idleTracker struct {
	sync.Mutex
	connMap map[net.Conn]struct{}
}

func (i *idleTracker) trackConn(conn net.Conn, state http.ConnState) {
	if state == http.StateIdle || state == http.StateClosed || state == http.StateHijacked {
		i.Lock()
		defer i.Unlock()
		if state == http.StateIdle {
			i.connMap[conn] = struct{}{}
		} else {
			delete(i.connMap, conn)
		}
	}
}

func (i *idleTracker) String() string {
	return "idle connections"
}

func (i *idleTracker) Shutdown() {
	i.Lock()
	defer i.Unlock()
	for conn := range i.connMap {
		conn.Close()
	}
}

func newIdleTracker() *idleTracker {
	return &idleTracker{connMap: make(map[net.Conn]struct{})}
}
