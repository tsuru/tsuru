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

// Shutdownable is an interface representing the ability to
// shutdown a particular resource
type Shutdownable interface {
	Shutdown(ctx context.Context) error
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

// Do shutdowns All registered Shutdownable items
func Do(ctx context.Context, w io.Writer) error {
	lock.Lock()
	defer lock.Unlock()
	done := make(chan bool)
	wg := sync.WaitGroup{}
	for _, h := range registered {
		wg.Add(1)
		go func(h Shutdownable) {
			defer wg.Done()
			var name string
			if _, ok := h.(fmt.Stringer); ok {
				name = fmt.Sprintf("%s", h)
			} else {
				name = fmt.Sprintf("%T", h)
			}
			fmt.Fprintf(w, "running shutdown for %s...\n", name)
			err := h.Shutdown(ctx)
			if err != nil {
				fmt.Fprintf(w, "running shutdown for %s. ERROED: %v", name, err)
				return
			}
			fmt.Fprintf(w, "running shutdown for %s. DONE.\n", name)
		}(h)
	}
	go func() {
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
