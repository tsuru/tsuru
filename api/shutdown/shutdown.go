// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shutdown

import (
	"context"
	"fmt"
	"io"
	"sync"
)

type Shutdownable interface {
	Shutdown()
}

var (
	registered []Shutdownable
	lock       sync.Mutex
)

// Register registers an item as shutdownable
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

// Do shutdowns All registered Shutdownable items
func Do(ctx context.Context, w io.Writer) error {
	lock.Lock()
	defer lock.Unlock()
	done := make(chan bool)
	go func() {
		wg := sync.WaitGroup{}
		for _, h := range registered {
			wg.Add(1)
			go func(h Shutdownable) {
				defer wg.Done()
				fmt.Fprintf(w, "running shutdown for %v...\n", h)
				h.Shutdown()
				fmt.Fprintf(w, "running shutdown for %v. DONE.\n", h)
			}(h)
		}
		wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	return nil
}
