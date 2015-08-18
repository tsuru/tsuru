// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockertest

import "sync"

type fakeLocker struct {
	locks map[string]struct{}
	mut   sync.Mutex
}

func NewFakeLocker() *fakeLocker {
	return &fakeLocker{locks: make(map[string]struct{})}
}

func (l *fakeLocker) Lock(appName string) bool {
	l.mut.Lock()
	defer l.mut.Unlock()
	if _, ok := l.locks[appName]; ok {
		return false
	}
	l.locks[appName] = struct{}{}
	return true
}

func (l *fakeLocker) Unlock(appName string) {
	l.mut.Lock()
	defer l.mut.Unlock()
	delete(l.locks, appName)
}
