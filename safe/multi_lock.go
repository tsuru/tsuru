// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import "sync"

type multiLocker struct {
	m   map[string]*sync.Mutex
	mut sync.Mutex
}

func MultiLocker() *multiLocker {
	return &multiLocker{m: make(map[string]*sync.Mutex)}
}

func (l *multiLocker) Lock(name string) {
	l.mut.Lock()
	mutex, ok := l.m[name]
	if !ok {
		mutex = new(sync.Mutex)
		l.m[name] = mutex
	}
	l.mut.Unlock()
	mutex.Lock()
}

func (l *multiLocker) Unlock(name string) {
	l.mut.Lock()
	mutex := l.m[name]
	l.mut.Unlock()
	mutex.Unlock()
}
