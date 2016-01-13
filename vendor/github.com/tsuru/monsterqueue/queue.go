// Copyright 2015 monsterqueue authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package monsterqueue

import (
	"errors"
	"time"
)

var ErrNoSuchJob = errors.New("no job found")
var ErrNoJobResult = errors.New("no result available")
var ErrQueueWaitTimeout = errors.New("timeout waiting for result")
var ErrNoJobResultSet = errors.New("task didn't set job result")

const (
	JobStateEnqueued = "enqueued"
	JobStateRunning  = "running"
	JobStateDone     = "done"

	StackTraceLimit = 2048
)

type JobParams map[string]interface{}
type JobResult interface{}

// A Job represents a enqueued job and it's returned by Queue.Enqueue() and
// Queue.EnqueueWait() and received by Task.Run().
//
// Every Task implementation should call either Job.Success() or Job.Error()
// to notify the queue of the job's result.
type Job interface {

	// Notify task's success with intended result. Returns a boolean
	// indicating if there are clients waiting for its results with (callers
	// of Queue.EnqueueWait()) and an error.
	Success(result JobResult) (bool, error)

	// Notify task's error. The return values are the same return by
	// Job.Success().
	Error(jobErr error) (bool, error)

	// Returns a job's result. An error will be returned if Job.Error() as
	// called by the task.
	Result() (JobResult, error)

	// Returns a string ID for the Job which can be user with
	// Queue.RetrieveJob().
	ID() string

	// Returns parameters sent to Queue.Enqueue{Wait}()
	Parameters() JobParams

	// Returns the task name for this Job.
	TaskName() string

	// Returns the Queue instance used by this Job. It's useful if the Task
	// wants to Enqueue more jobs.
	Queue() Queue

	// Returns a struct with information about the job's state and timestamps
	// for state changes.
	Status() JobStatus

	// Returns the current stack when queue.Enqueue() was called.
	EnqueueStack() string
}

type JobStatus struct {

	// Possible return values are: JobStateEnqueued, JobStateRunning or
	// JobStateDone.
	State string

	// Time job was added to queue.
	Enqueued time.Time

	// Time job started executing, only available if state is JobStateRunning or
	// JobStateDone.
	Started time.Time

	// Time job was finished, only available if state is JobStateDone.
	Done time.Time
}

// The application using this library is responsible for implementing the Task
// interface. A variable of a type implementing this interface should be used
// when calling Queue.RegisterTask().
type Task interface {

	// This function is responsible for implementing task's intended behavior.
	// It'll receive a Job instance which will be used to notify task's result
	// and read enqueued parameters.
	Run(job Job)

	// Task's name which will be used when calling Queue.Enqueue() or
	// Queue.EnqueueWait()
	Name() string
}

// Queue interface is implemented in mongodb and redis packages. Both packages
// have a NewQueue function which return a Queue.
type Queue interface {

	// Register task to run when Queue.ProcessLoop() is called.
	RegisterTask(task Task) error

	// Enqueue a new task with received parameters, please note that JobParams
	// will be serialized by underlying implementation, so this value must be
	// serializable.
	Enqueue(taskName string, params JobParams) (Job, error)

	// Enqueue a new task and wait for its results to be available. If the
	// results aren't available when the timeout is reached a
	// ErrQueueWaitTimeout error will be returned. Note that the task will
	// keep running even after this error is returned.
	//
	// If the returned error is nil, the Job will have a result available,
	// which can be retrieved calling Job.Result().
	EnqueueWait(taskName string, params JobParams, timeout time.Duration) (Job, error)

	// Process existing tasks in queue and wait for new tasks to be available.
	// This function will block until Stop is called.
	ProcessLoop()

	// Stop processing loop and wait for pending tasks to complete.
	Stop()

	// Wait for running tasks to complete running but it doesn't try to stop
	// the processing loop, if new tasks arrive this method will keep waiting.
	Wait()

	// Retrieves an existing Job with its results, if already available.
	RetrieveJob(jobId string) (Job, error)

	// Completely erases storage removing enqueued, processing and finished
	// tasks.
	ResetStorage() error

	// List all existing jobs
	ListJobs() ([]Job, error)

	// Delete a job from storage, please note that it's only safe to call this
	// method if a job's state is JobStateDone. Otherwise, the behavior is
	// undefined.
	DeleteJob(jobId string) error
}

type JobList []Job

func (l JobList) Len() int           { return len(l) }
func (l JobList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l JobList) Less(i, j int) bool { return l[i].Status().Enqueued.Before(l[j].Status().Enqueued) }
