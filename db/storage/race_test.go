// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build race

package storage

import (
	"sync"

	"gopkg.in/check.v1"
)

func (s *S) TestOpenIsThreadSafe(c *check.C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_db_race_tests")
	c.Assert(err, check.IsNil)
	defer storage.session.Close()
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		st1, err := Open("127.0.0.1:27017", "tsuru_db_race_tests_2")
		c.Check(err, check.IsNil)
		c.Check(st1.session.LiveServers(), check.DeepEquals, storage.session.LiveServers())
		wg.Done()
	}()
	go func() {
		st2, err := Open("127.0.0.1:27018", "tsuru_db_race_tests_3")
		c.Check(err, check.IsNil)
		c.Check(st2.session.LiveServers(), check.DeepEquals, storage.session.LiveServers())
		wg.Done()
	}()
	go func() {
		st3, err := Open("127.0.0.1:27017", "tsuru_db_race_tests_4")
		c.Check(err, check.IsNil)
		c.Check(st3.session.LiveServers(), check.DeepEquals, storage.session.LiveServers())
		wg.Done()
	}()
	wg.Wait()
}
