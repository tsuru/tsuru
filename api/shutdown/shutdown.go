// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shutdown

import (
	"sync"
)

type shutdownable interface {
	Shutdown()
}

type ShutdownFunc func()

func (fn ShutdownFunc) Shutdown() {
	fn()
}

var (
	registered []shutdownable
	lock       sync.Mutex
)

func Register(s shutdownable) {
	lock.Lock()
	defer lock.Unlock()
	registered = append(registered, s)
}

func All() []shutdownable {
	lock.Lock()
	defer lock.Unlock()
	return registered
}
