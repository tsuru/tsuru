// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/api/shutdown"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestFactory(c *check.C) {
	f, err := Factory()
	c.Assert(err, check.IsNil)
	_, ok := f.(*redisPubSubFactory)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestFactoryConfigUndefined(c *check.C) {
	f, err := Factory()
	c.Assert(err, check.IsNil)
	_, ok := f.(*redisPubSubFactory)
	c.Assert(ok, check.Equals, true)
}

func (s *S) SetUpTest(c *check.C) {
	config.Set("queue:mongo-database", "test-queue")
	ResetQueue()
}

type testTask struct {
	callCount int
}

func (t *testTask) Run(j monsterqueue.Job) {
	t.callCount++
	j.Success("result")
}

func (t *testTask) Name() string {
	return "test-task"
}

func (s *S) TestQueue(c *check.C) {
	c.Assert(shutdown.All(), check.HasLen, 0)
	q, err := Queue()
	c.Assert(err, check.IsNil)
	task := &testTask{}
	err = q.RegisterTask(task)
	c.Assert(err, check.IsNil)
	j, err := q.EnqueueWait(task.Name(), nil, time.Minute)
	c.Assert(err, check.IsNil)
	result, err := j.Result()
	c.Assert(err, check.IsNil)
	c.Assert(result, check.Equals, "result")
	c.Assert(shutdown.All(), check.HasLen, 1)
}
