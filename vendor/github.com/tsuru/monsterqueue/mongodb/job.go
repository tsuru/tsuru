// Copyright 2015 monsterqueue authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"errors"
	"time"

	"github.com/tsuru/monsterqueue"
	"gopkg.in/mgo.v2/bson"
)

type jobResultMessage struct {
	Error     string
	Result    monsterqueue.JobResult
	Done      bool
	Timestamp time.Time
}

type jobOwnership struct {
	Owned     bool
	Name      string
	Timestamp time.Time
}

type jobMongoDB struct {
	Id            bson.ObjectId `bson:"_id"`
	Task          string
	Params        monsterqueue.JobParams
	Timestamp     time.Time
	Owner         jobOwnership
	ResultMessage jobResultMessage
	Waited        bool
	Stack         string
	queue         *queueMongoDB
}

func (j *jobMongoDB) ID() string {
	return j.Id.Hex()
}

func (j *jobMongoDB) Parameters() monsterqueue.JobParams {
	return j.Params
}

func (j *jobMongoDB) TaskName() string {
	return j.Task
}

func (j *jobMongoDB) Queue() monsterqueue.Queue {
	return j.queue
}

func (j *jobMongoDB) EnqueueStack() string {
	return j.Stack
}

func (j *jobMongoDB) Status() (status monsterqueue.JobStatus) {
	if j.Owner.Owned {
		status.State = monsterqueue.JobStateRunning
	} else if j.ResultMessage.Done {
		status.State = monsterqueue.JobStateDone
	} else {
		status.State = monsterqueue.JobStateEnqueued
	}
	status.Enqueued = j.Timestamp
	status.Started = j.Owner.Timestamp
	status.Done = j.ResultMessage.Timestamp
	return
}

func (j *jobMongoDB) Success(result monsterqueue.JobResult) (bool, error) {
	err := j.queue.moveToResult(j, result, nil)
	if err != nil {
		return false, err
	}
	received, err := j.queue.publishResult(j)
	return received, err
}

func (j *jobMongoDB) Error(jobErr error) (bool, error) {
	err := j.queue.moveToResult(j, nil, jobErr)
	if err != nil {
		return false, err
	}
	received, err := j.queue.publishResult(j)
	return received, err
}

func (j *jobMongoDB) Result() (monsterqueue.JobResult, error) {
	if !j.ResultMessage.Done {
		return nil, monsterqueue.ErrNoJobResult
	}
	var err error
	if j.ResultMessage.Error != "" {
		err = errors.New(j.ResultMessage.Error)
	}
	return j.ResultMessage.Result, err
}
