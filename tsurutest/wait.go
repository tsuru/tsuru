// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package tsurutest provides test utilities used across tsuru code base.
package tsurutest

import (
	"fmt"
	"time"
)

func WaitCondition(timeout time.Duration, cond func() bool) error {
	ok := make(chan struct{})
	exit := make(chan struct{})
	go func() {
		for {
			select {
			case <-exit:
			default:
				if cond() {
					close(ok)
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()
	select {
	case <-ok:
		return nil
	case <-time.After(timeout):
		close(exit)
		return fmt.Errorf("timed out waiting for condition after %s", timeout)
	}
}
