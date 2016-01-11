// Copyright 2015 monsterqueue authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/monsterqueue/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type queueMongoDB struct {
	config   *QueueConfig
	session  *mgo.Session
	tasks    map[string]monsterqueue.Task
	tasksMut sync.RWMutex
	done     chan bool
	wg       sync.WaitGroup
}

type QueueConfig struct {
	Url              string // MongoDB connection url
	Database         string // MongoDB database name
	CollectionPrefix string // Prefix for all collections created in MongoDB
	PollingInterval  time.Duration
}

// Creates a new queue. The QueueConfig parameter will tell us how to connect
// to mongodb. This command will fail if the MongoDB server is not available.
//
// Tasks registered in this queue instance will run when `ProcessLoop` is
// called in this *same* instance.
func NewQueue(conf QueueConfig) (monsterqueue.Queue, error) {
	q := &queueMongoDB{
		config: &conf,
		tasks:  make(map[string]monsterqueue.Task),
		done:   make(chan bool),
	}
	var err error
	if conf.Url == "" {
		return nil, errors.New("setting QueueConfig.Url is required")
	}
	dialInfo, err := mgo.ParseURL(conf.Url)
	if err != nil {
		return nil, err
	}
	dialInfo.FailFast = true
	q.session, err = mgo.DialWithInfo(dialInfo)
	if err != nil {
		return nil, err
	}
	q.session.SetSyncTimeout(10 * time.Second)
	q.session.SetSocketTimeout(1 * time.Minute)
	db := q.session.DB(conf.Database)
	if db.Name == "test" {
		q.session.Close()
		return nil, errors.New("database name should be set in QueueConfig.Url or QueueConfig.Database")
	}
	return q, err
}

func (q *queueMongoDB) tasksColl() *mgo.Collection {
	s := q.session.Copy()
	name := "queue_tasks"
	if q.config.CollectionPrefix != "" {
		name = fmt.Sprintf("%s_%s", q.config.CollectionPrefix, name)
	}
	return s.DB(q.config.Database).C(name)
}

func (q *queueMongoDB) RegisterTask(task monsterqueue.Task) error {
	q.tasksMut.Lock()
	defer q.tasksMut.Unlock()
	if _, isRegistered := q.tasks[task.Name()]; isRegistered {
		return errors.New("task already registered")
	}
	q.tasks[task.Name()] = task
	return nil
}

func (q *queueMongoDB) Enqueue(taskName string, params monsterqueue.JobParams) (monsterqueue.Job, error) {
	coll := q.tasksColl()
	defer coll.Database.Session.Close()
	j := q.initialJob(taskName, params)
	err := coll.Insert(j)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (q *queueMongoDB) getDoneJob(jobId bson.ObjectId) (*jobMongoDB, error) {
	coll := q.tasksColl()
	defer coll.Database.Session.Close()
	var resultJob jobMongoDB
	err := coll.Find(bson.M{"_id": jobId, "resultmessage.done": true, "waited": false}).One(&resultJob)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &resultJob, nil
}

func (q *queueMongoDB) EnqueueWait(taskName string, params monsterqueue.JobParams, timeout time.Duration) (monsterqueue.Job, error) {
	j := q.initialJob(taskName, params)
	j.Waited = true
	coll := q.tasksColl()
	defer coll.Database.Session.Close()
	err := coll.Insert(j)
	if err != nil {
		return nil, err
	}
	timeoutCh := time.After(timeout)
out:
	for {
		job, err := q.getDoneJob(j.Id)
		if err != nil {
			log.Errorf("error trying to get job %s: %s", j.Id, err.Error())
		}
		if job != nil {
			return job, nil
		}
		select {
		case <-timeoutCh:
			break out
		case <-time.After(200 * time.Millisecond):
		}
	}
	err = coll.Update(bson.M{
		"_id":    j.Id,
		"waited": true,
	}, bson.M{"$set": bson.M{"waited": false}})
	var resultJob *jobMongoDB
	if err == mgo.ErrNotFound {
		resultJob, err = q.getDoneJob(j.Id)
	}
	if err != nil {
		return &j, err
	}
	if resultJob != nil {
		return resultJob, nil
	}
	return &j, monsterqueue.ErrQueueWaitTimeout
}

func (q *queueMongoDB) ProcessLoop() {
	interval := q.config.PollingInterval
	if interval == 0 {
		interval = 1 * time.Second
	}
	for {
		q.wg.Add(1)
		hasMessage, err := q.waitForMessage()
		if err != nil {
			log.Debugf("error getting message from queue: %s", err.Error())
		}
		if hasMessage {
			select {
			case <-q.done:
				return
			default:
			}
			continue
		}
		select {
		case <-time.After(interval):
		case <-q.done:
			return
		}
	}
}

func (q *queueMongoDB) Stop() {
	q.done <- true
	q.Wait()
}

func (q *queueMongoDB) Wait() {
	q.wg.Wait()
}

func (q *queueMongoDB) ResetStorage() error {
	coll := q.tasksColl()
	defer coll.Database.Session.Close()
	defer q.session.Close()
	return coll.DropCollection()
}

func (q *queueMongoDB) RetrieveJob(jobId string) (monsterqueue.Job, error) {
	coll := q.tasksColl()
	defer coll.Database.Session.Close()
	var job jobMongoDB
	err := coll.FindId(bson.ObjectIdHex(jobId)).One(&job)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, monsterqueue.ErrNoSuchJob
		}
		return nil, err
	}
	return &job, err
}

func (q *queueMongoDB) ListJobs() ([]monsterqueue.Job, error) {
	coll := q.tasksColl()
	defer coll.Database.Session.Close()
	var mongodbJobs []jobMongoDB
	err := coll.Find(nil).All(&mongodbJobs)
	if err != nil {
		return nil, err
	}
	jobs := make([]monsterqueue.Job, len(mongodbJobs))
	for i := range mongodbJobs {
		jobs[i] = &mongodbJobs[i]
	}
	return jobs, nil
}

func (q *queueMongoDB) DeleteJob(jobId string) error {
	coll := q.tasksColl()
	defer coll.Database.Session.Close()
	return coll.RemoveId(bson.ObjectIdHex(jobId))
}

func (q *queueMongoDB) initialJob(taskName string, params monsterqueue.JobParams) jobMongoDB {
	buf := make([]byte, monsterqueue.StackTraceLimit)
	buf = buf[:runtime.Stack(buf, false)]
	return jobMongoDB{
		Id:        bson.NewObjectId(),
		Task:      taskName,
		Params:    params,
		Timestamp: time.Now().UTC(),
		Stack:     string(buf),
		queue:     q,
	}
}

func (q *queueMongoDB) waitForMessage() (bool, error) {
	coll := q.tasksColl()
	defer coll.Database.Session.Close()
	var job jobMongoDB
	hostname, _ := os.Hostname()
	ownerData := jobOwnership{
		Name:      fmt.Sprintf("%s_%d", hostname, os.Getpid()),
		Owned:     true,
		Timestamp: time.Now().UTC(),
	}
	q.tasksMut.RLock()
	taskNames := make([]string, 0, len(q.tasks))
	for taskName := range q.tasks {
		taskNames = append(taskNames, taskName)
	}
	q.tasksMut.RUnlock()
	_, err := coll.Find(bson.M{
		"task":               bson.M{"$in": taskNames},
		"owner.owned":        false,
		"resultmessage.done": false,
	}).Sort("_id").Apply(mgo.Change{
		Update: bson.M{
			"$set": bson.M{"owner": ownerData},
		},
	}, &job)
	if err != nil {
		q.wg.Done()
		if err == mgo.ErrNotFound {
			return false, nil
		}
		return false, err
	}
	job.queue = q
	if err != nil {
		q.moveToResult(&job, nil, err)
		q.wg.Done()
		return true, err
	}
	q.tasksMut.RLock()
	task, _ := q.tasks[job.Task]
	q.tasksMut.RUnlock()
	if task == nil {
		err := fmt.Errorf("unregistered task name %q", job.Task)
		q.moveToResult(&job, nil, err)
		q.wg.Done()
		return true, err
	}
	go func() {
		defer q.wg.Done()
		task.Run(&job)
		if !job.ResultMessage.Done {
			q.moveToResult(&job, nil, monsterqueue.ErrNoJobResultSet)
		}
	}()
	return true, nil
}

func (q *queueMongoDB) moveToResult(job *jobMongoDB, result monsterqueue.JobResult, jobErr error) error {
	var resultMsg jobResultMessage
	resultMsg.Result = result
	resultMsg.Timestamp = time.Now().UTC()
	resultMsg.Done = true
	if jobErr != nil {
		resultMsg.Error = jobErr.Error()
	}
	job.ResultMessage = resultMsg
	coll := q.tasksColl()
	defer coll.Database.Session.Close()
	return coll.UpdateId(job.Id, bson.M{"$set": bson.M{"resultmessage": resultMsg, "owner.owned": false}})
}

func (q *queueMongoDB) publishResult(job *jobMongoDB) (bool, error) {
	coll := q.tasksColl()
	defer coll.Database.Session.Close()
	err := coll.Update(bson.M{"_id": job.Id, "waited": true}, bson.M{"$set": bson.M{"waited": false}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
