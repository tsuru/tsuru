// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build race

package safe

import (
	"sync"

	"gopkg.in/check.v1"
)

func (s *S) TestSafeBufferIsThreadSafe(c *check.C) {
	var buf Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		buf.Write([]byte("something"))
		wg.Done()
	}()
	go func() {
		var p [4]byte
		buf.Read(p[:])
		wg.Done()
	}()
	buf.Reset()
	wg.Wait()
}
