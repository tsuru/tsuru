// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue implements a Pub/Sub channel in tsuru. It abstracts
// which server is being used and handles connection pooling and
// data transmiting
package queue

import (
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/monsterqueue/mongodb"
	"github.com/tsuru/tsuru/api/shutdown"
)

// PubSubQ represents an implementation that allows Publishing and
// Subscribing messages.
type PubSubQ interface {
	// Publishes a message using the underlaying queue server.
	Pub(msg []byte) error

	// Returns a channel that will yield every message published to this
	// queue.
	Sub() (<-chan []byte, error)

	// Unsubscribe the queue, this should make sure the channel returned
	// by Sub() is closed.
	UnSub() error
}

// PubSubFactory manages queues. It's able to create new queue and handler
// instances.
type PubSubFactory interface {
	// PubSub returns a PubSubQ instance, identified by the given name.
	PubSub(name string) (PubSubQ, error)

	Reset()
}

var factoryInstance = &redisPubSubFactory{}

// Factory returns an instance of the PubSubFactory used in tsuru. Only redis
// pubsub is available.
func Factory() (PubSubFactory, error) {
	return factoryInstance, nil
}

type queueInstanceData struct {
	sync.RWMutex
	instance monsterqueue.Queue
}

func (q *queueInstanceData) Shutdown() {
	q.Lock()
	defer q.Unlock()
	if q.instance != nil {
		q.instance.Stop()
		q.instance = nil
	}
}

func (q *queueInstanceData) String() string {
	return "queued tasks"
}

var queueData queueInstanceData

func ResetQueue() {
	queueData.Lock()
	defer queueData.Unlock()
	if queueData.instance != nil {
		queueData.instance.Stop()
		queueData.instance.ResetStorage()
		queueData.instance = nil
	}
}

func TestingWaitQueueTasks(n int, timeout time.Duration) error {
	queueData.Lock()
	defer queueData.Unlock()
	if queueData.instance != nil {
		timeoutCh := time.After(timeout)
		for {
			jobs, _ := queueData.instance.ListJobs()
			runningCount := 0
			for _, j := range jobs {
				if j.Status().State != monsterqueue.JobStateEnqueued {
					runningCount++
				}
			}
			if n <= runningCount {
				break
			}
			select {
			case <-timeoutCh:
				return errors.Errorf("timeout waiting for task after %v", timeout)
			case <-time.After(10 * time.Millisecond):
			}
		}
		queueData.instance.Stop()
		queueData.instance.ResetStorage()
		queueData.instance = nil
	}
	return nil
}

func Queue() (monsterqueue.Queue, error) {
	queueData.RLock()
	if queueData.instance != nil {
		defer queueData.RUnlock()
		return queueData.instance, nil
	}
	queueData.RUnlock()
	queueData.Lock()
	defer queueData.Unlock()
	if queueData.instance != nil {
		return queueData.instance, nil
	}
	queueMongoUrl, _ := config.GetString("queue:mongo-url")
	if queueMongoUrl == "" {
		queueMongoUrl = "localhost:27017"
	}
	queueMongoDB, _ := config.GetString("queue:mongo-database")
	pollingInterval, _ := config.GetFloat("queue:mongo-polling-interval")
	if pollingInterval == 0.0 {
		pollingInterval = 1.0
	}
	conf := mongodb.QueueConfig{
		CollectionPrefix: "tsuru",
		Url:              queueMongoUrl,
		Database:         queueMongoDB,
		PollingInterval:  time.Duration(pollingInterval * float64(time.Second)),
	}
	var err error
	queueData.instance, err = mongodb.NewQueue(conf)
	if err != nil {
		return nil, errors.Wrap(err, "could not create queue instance, please check queue:mongo-url and queue:mongo-database config entries. error")
	}
	shutdown.Register(&queueData)
	go queueData.instance.ProcessLoop()
	return queueData.instance, nil
}
