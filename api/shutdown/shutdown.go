// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shutdown

import (
	"sync"
)

type Shutdownable interface {
	Shutdown()
}

var (
	registered []Shutdownable
	lock       sync.Mutex
)

func Register(s Shutdownable) {
	lock.Lock()
	defer lock.Unlock()
	registered = append(registered, s)
}

func All() []Shutdownable {
	lock.Lock()
	defer lock.Unlock()
	return registered
}
