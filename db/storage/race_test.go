// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build race

package storage

import (
	"sync"
	"time"

	"launchpad.net/gocheck"
)

func (s *S) TestOpenIsThreadSafe(c *gocheck.C) {
	storage, err := Open("127.0.0.1:27017", "tsuru_db_race_tests")
	c.Assert(err, gocheck.IsNil)
	defer storage.session.Close()
	sess := conn["127.0.0.1:27017"]
	sess.used = time.Now().Add(-1 * time.Hour)
	conn["127.0.0.1:27017"] = sess
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		st1, err := Open("127.0.0.1:27017", "tsuru_db_race_tests_2")
		c.Check(err, gocheck.IsNil)
		c.Check(st1.session.LiveServers(), gocheck.DeepEquals, storage.session.LiveServers())
		wg.Done()
	}()
	go func() {
		st2, err := Open("127.0.0.1:27017", "tsuru_db_race_tests_3")
		c.Check(err, gocheck.IsNil)
		c.Check(st2.session.LiveServers(), gocheck.DeepEquals, storage.session.LiveServers())
		wg.Done()
	}()
	go func() {
		st3, err := Open("127.0.0.1:27017", "tsuru_db_race_tests_4")
		c.Check(err, gocheck.IsNil)
		c.Check(st3.session.LiveServers(), gocheck.DeepEquals, storage.session.LiveServers())
		wg.Done()
	}()
	wg.Wait()
}
