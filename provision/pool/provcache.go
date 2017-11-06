// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"sync"

	"github.com/tsuru/tsuru/provision"
)

type poolProvCache struct {
	sync.RWMutex
	entries map[string]provision.Provisioner
}

var poolCache poolProvCache

func ResetCache() {
	poolCache.Lock()
	defer poolCache.Unlock()
	poolCache.entries = nil
}

func (c *poolProvCache) Get(name string) provision.Provisioner {
	c.RLock()
	defer c.RUnlock()
	if c.entries == nil {
		return nil
	}
	return c.entries[name]
}

func (c *poolProvCache) Set(name string, prov provision.Provisioner) {
	c.Lock()
	defer c.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]provision.Provisioner)
	}
	c.entries[name] = prov
}
