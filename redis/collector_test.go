// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redis

import (
	"github.com/prometheus/client_golang/prometheus"
	check "gopkg.in/check.v1"
	redis "gopkg.in/redis.v3"
)

func (s *S) TestCollectorAdd(c *check.C) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	collector.Add("my-pool", client)
	c.Assert(collector.clients["my-pool"], check.DeepEquals, client)
}

func (s *S) TestCollectorDescribe(c *check.C) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	collector.Add("my-pool", client)
	ch := make(chan *prometheus.Desc, 10)
	collector.Describe(ch)
	c.Assert(len(ch), check.Equals, 6)
}

func (s *S) TestCollectorCollect(c *check.C) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	collector.Add("my-pool", client)
	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	c.Assert(len(ch), check.Equals, 6)
}
