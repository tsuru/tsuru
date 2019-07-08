// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/tsuru/config"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func (s *S) TestLogDispatcherSend(c *check.C) {
	logsInQueue.Set(0)
	svc, err := storageAppLogService()
	c.Assert(err, check.IsNil)
	listener, err := svc.Watch("myapp1", "", "")
	c.Assert(err, check.IsNil)
	defer listener.Close()
	dispatcher := newlogDispatcher(2000000, svc.(*storageLogService).storage)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := appTypes.Applog{
		Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1",
	}
	dispatcher.send(&logMsg)
	dispatcher.shutdown(context.Background())
	logs, err := svc.List(appTypes.ListLogArgs{AppName: "myapp1", Limit: 1})
	c.Assert(err, check.IsNil)
	compareLogs(c, logs, []appTypes.Applog{logMsg})
	err = dispatcher.send(&logMsg)
	c.Assert(err, check.ErrorMatches, `log dispatcher is shutting down`)
	var dtoMetric dto.Metric
	logsInQueue.Write(&dtoMetric)
	c.Assert(dtoMetric.Gauge.GetValue(), check.Equals, 0.0)
	ch := listener.Chan()
	recvMsg := <-ch
	recvMsg.Date = baseTime
	compareLogs(c, []appTypes.Applog{recvMsg}, []appTypes.Applog{logMsg})
}

func (s *S) TestLogDispatcherSendConcurrent(c *check.C) {
	svc, err := storageAppLogService()
	c.Assert(err, check.IsNil)
	dispatcher := newlogDispatcher(2000000, svc.(*storageLogService).storage)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := []appTypes.Applog{
		{Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1"},
		{Date: baseTime, Message: "msg2", Source: "web", AppName: "myapp2", Unit: "unit1"},
	}
	nConcurrent := 100
	wg := sync.WaitGroup{}
	for i := 0; i < nConcurrent; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			dispatcher.send(&logMsg[i%len(logMsg)])
		}(i)
	}
	wg.Wait()
	dispatcher.shutdown(context.Background())
	logs, err := svc.List(appTypes.ListLogArgs{AppName: "myapp1", Limit: nConcurrent / 2})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, nConcurrent/2)
	logs, err = svc.List(appTypes.ListLogArgs{AppName: "myapp2", Limit: nConcurrent / 2})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, nConcurrent/2)
}

func (s *S) TestLogDispatcherShutdownConcurrent(c *check.C) {
	svc, err := storageAppLogService()
	c.Assert(err, check.IsNil)
	logsInQueue.Set(0)
	dispatcher := newlogDispatcher(2000000, svc.(*storageLogService).storage)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := []appTypes.Applog{
		{Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1"},
		{Date: baseTime, Message: "msg2", Source: "web", AppName: "myapp2", Unit: "unit1"},
	}
	nConcurrent := 100
	for i := 0; i < nConcurrent; i++ {
		go func(i int) {
			dispatcher.send(&logMsg[i%len(logMsg)])
		}(i)
	}
	dispatcher.shutdown(context.Background())
	var dtoMetric dto.Metric
	logsInQueue.Write(&dtoMetric)
	c.Assert(dtoMetric.Gauge.GetValue(), check.Equals, 0.0)
}

func (s *S) TestLogDispatcherSendDBFailure(c *check.C) {
	svc, err := storageAppLogService()
	c.Assert(err, check.IsNil)
	dispatcher := newlogDispatcher(2000000, svc.(*storageLogService).storage)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	oldDbURL, err := config.Get("database:url")
	c.Assert(err, check.IsNil)
	var count int32
	dbOk := make(chan bool)
	config.Set("database:url", func() interface{} {
		val := atomic.AddInt32(&count, 1)
		if val == 1 {
			close(dbOk)
			return "localhost:44556"
		}
		return oldDbURL
	})
	defer config.Set("database:url", oldDbURL)
	dispatcher.send(&appTypes.Applog{
		Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1",
	})
	<-dbOk
	dispatcher.send(&appTypes.Applog{
		Date: baseTime, Message: "msg2", Source: "web", AppName: "myapp1", Unit: "unit1",
	})
	timeout := time.After(10 * time.Second)
	var logs []appTypes.Applog
	var logsErr error
loop:
	for {
		logs, logsErr = svc.List(appTypes.ListLogArgs{AppName: "myapp1", Limit: 2})
		c.Assert(logsErr, check.IsNil)
		if len(logs) == 2 {
			break
		}
		select {
		case <-timeout:
			c.Fatalf("timeout waiting for all logs, last count: %d", len(logs))
			break loop
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	compareLogsNoDate(c, logs, []appTypes.Applog{
		{
			Source:  "tsuru",
			AppName: "myapp1",
			Unit:    "api",
			Message: "Log messages dropped due to mongodb insert error: no reachable servers",
		},
		{
			Source:  "web",
			AppName: "myapp1",
			Unit:    "unit1",
			Message: "msg2",
		},
	})
	dispatcher.shutdown(context.Background())
}

func (s *S) TestBulkProcessorQueueSizeDefault(c *check.C) {
	processor := initBulkProcessor(time.Second, 100, "")
	c.Assert(cap(processor.ch), check.Equals, bulkQueueMaxSize)
}

func (s *S) TestBulkProcessorCustomQueueSize(c *check.C) {
	config.Set("log:queue-size", 10)
	defer config.Unset("log:queue-size")
	processor := initBulkProcessor(time.Second, 100, "")
	c.Assert(cap(processor.ch), check.Equals, 10)
}

func (s *S) TestLogDispatcherSendRateLimit(c *check.C) {
	config.Set("log:app-log-rate-limit", 1)
	defer config.Unset("log:app-log-rate-limit")
	svc, err := storageAppLogService()
	c.Assert(err, check.IsNil)
	listener, err := svc.Watch("myapp1", "", "")
	c.Assert(err, check.IsNil)
	defer listener.Close()
	dispatcher := newlogDispatcher(2000000, svc.(*storageLogService).storage)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := appTypes.Applog{
		Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1",
	}
	dispatcher.send(&logMsg)
	dispatcher.send(&logMsg)
	dispatcher.shutdown(context.Background())
	logs, err := svc.List(appTypes.ListLogArgs{AppName: "myapp1", Limit: 2})
	c.Assert(err, check.IsNil)
	compareLogsNoDate(c, logs, []appTypes.Applog{
		logMsg,
		{
			Message: "Log messages dropped due to exceeded rate limit. Limit: 1 logs/s.",
			Source:  "tsuru",
			AppName: "myapp1",
			Unit:    "api",
		},
	})
}

func (s *S) TestLogDispatcherSendGlobalRateLimit(c *check.C) {
	config.Set("log:global-app-log-rate-limit", 1)
	defer config.Unset("log:global-app-log-rate-limit")
	svc, err := storageAppLogService()
	c.Assert(err, check.IsNil)
	listener, err := svc.Watch("myapp1", "", "")
	c.Assert(err, check.IsNil)
	defer listener.Close()
	dispatcher := newlogDispatcher(2000000, svc.(*storageLogService).storage)
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := appTypes.Applog{
		Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1",
	}
	dispatcher.send(&logMsg)
	dispatcher.send(&logMsg)
	dispatcher.shutdown(context.Background())
	logs, err := svc.List(appTypes.ListLogArgs{AppName: "myapp1", Limit: 2})
	c.Assert(err, check.IsNil)
	compareLogsNoDate(c, logs, []appTypes.Applog{
		logMsg,
		{
			Message: "Log messages dropped due to exceeded global rate limit. Global Limit: 1 logs/s.",
			Source:  "tsuru",
			AppName: "myapp1",
			Unit:    "api",
		},
	})
}

type fakeFlusher struct {
	counter int
}

func (f *fakeFlusher) flush(msgs []*appTypes.Applog, lastMsg *msgWithTS) error {
	f.counter += len(msgs)
	return nil
}

func (s *S) BenchmarkBulkProcessorRun(c *check.C) {
	c.StopTimer()
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := &msgWithTS{
		msg: &appTypes.Applog{
			Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1",
		},
		arriveTime: time.Now(),
	}

	processor := initBulkProcessor(time.Second, 1000, "myapp1")
	flusher := &fakeFlusher{}
	processor.flushable = flusher
	go processor.run()
	c.StartTimer()
	for i := 0; i < c.N; i++ {
		processor.ch <- logMsg
	}
	processor.stopWait()
	c.StopTimer()
	c.Assert(flusher.counter, check.Equals, c.N)
}

func (s *S) BenchmarkBulkProcessorRunRateLimited(c *check.C) {
	config.Set("log:app-log-rate-limit", 100)
	defer config.Unset("log:app-log-rate-limit")
	c.StopTimer()
	baseTime, err := time.Parse(time.RFC3339, "2015-06-16T15:00:00.000Z")
	c.Assert(err, check.IsNil)
	baseTime = baseTime.Local()
	logMsg := &msgWithTS{
		msg: &appTypes.Applog{
			Date: baseTime, Message: "msg1", Source: "web", AppName: "myapp1", Unit: "unit1",
		},
		arriveTime: time.Now(),
	}

	processor := initBulkProcessor(time.Second, 1000, "myapp1")
	flusher := &fakeFlusher{}
	processor.flushable = flusher
	go processor.run()
	c.StartTimer()
	for i := 0; i < c.N; i++ {
		processor.ch <- logMsg
	}
	processor.stopWait()
	c.StopTimer()
	c.Assert(flusher.counter <= c.N, check.Equals, true)
}
