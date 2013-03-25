// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"sync"
)

const (
	closed int32 = iota
	open
)

var listeners = struct {
	m map[string][]*LogListener
	sync.RWMutex
}{
	m: make(map[string][]*LogListener),
}

type LogListener struct {
	C     <-chan Applog
	c     chan Applog
	state int32
}

func NewLogListener(a *App) *LogListener {
	c := make(chan Applog)
	l := LogListener{C: c, c: c, state: open}
	listeners.Lock()
	list := listeners.m[a.Name]
	list = append(list, &l)
	listeners.m[a.Name] = list
	listeners.Unlock()
	return &l
}
