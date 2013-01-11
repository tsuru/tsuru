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
	for _, name := range names {
		go func(qName string) {
			var err error
			var msg *queue.Message
			for err == nil {
				if msg, err = queue.Get(qName, 1e6); err == nil {
					err = msg.Delete()
				}
			}
			wg.Done()
		}(name)
	}
	wg.Wait()
}
