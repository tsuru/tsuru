// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"github.com/globocom/tsuru/queue"
	"sync"
)

// CleanQueues cleans the given queues.
func CleanQueues(names ...string) {
	var wg sync.WaitGroup
	wg.Add(len(names))
	factory, err := queue.Factory()
	if err != nil {
		panic(err)
	}
	for _, name := range names {
		go func(qName string) {
			var msg *queue.Message
			q, err := factory.Get(qName)
			for err == nil {
				if msg, err = q.Get(1e6); err == nil {
					err = q.Delete(msg)
				}
			}
			wg.Done()
		}(name)
	}
	wg.Wait()
}
