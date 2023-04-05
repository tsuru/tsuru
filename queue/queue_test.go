// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/api/shutdown"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct{}

var _ = check.Suite(&S{})

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
	c.Assert(queueData.instance, check.NotNil)
	shutdown.Do(context.Background(), io.Discard)
	c.Assert(queueData.instance, check.IsNil)
}
