// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsurutest

import (
	"net/http"
	"runtime"
	"strings"
	"sync"

	check "gopkg.in/check.v1"
)

func (s *S) TestSafeResponseRecorderWrite(c *check.C) {
	recorder := NewSafeResponseRecorder()
	recorder.Write([]byte("some test"))
	c.Assert(recorder.Body.String(), check.Equals, "some test")
}

func (s *S) TestSafeResponseRecorderWriteHeader(c *check.C) {
	recorder := NewSafeResponseRecorder()
	recorder.WriteHeader(http.StatusOK)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "")
}

func (s *S) TestSafeResponseRecorderWriteIsSafe(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(8)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	recorder := NewSafeResponseRecorder()
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			recorder.Write([]byte("test\n"))
			wg.Done()
		}()
	}
	wg.Wait()
	c.Assert(recorder.Body.String(), check.Equals, strings.Repeat("test\n", 1000))
}
