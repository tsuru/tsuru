// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/queue"
)

var (
	LogPubSubQueuePrefix = "pubsub:"

	bulkMaxWaitTime = time.Second

	dispatchersCurrent = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsuru_logs_dispatchers_current",
		Help: "The current number of log dispatchers running.",
	})

	logsInQueue = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsuru_logs_queue_current",
		Help: "The current number of log entries in all queues.",
	})

	logsQueueBlockedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_logs_queue_blocked_seconds_total",
		Help: "The total time spent blocked trying to add log to queue.",
	})

	logsQueueSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsuru_logs_dispatcher_queue_size",
		Help: "The max number of log entries in a dispatcher queue.",
	})

	logsWritten = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_logs_write_total",
		Help: "The number of log entries written to mongo.",
	})
)

func init() {
	prometheus.MustRegister(logsInQueue)
	prometheus.MustRegister(dispatchersCurrent)
	prometheus.MustRegister(logsQueueSize)
	prometheus.MustRegister(logsWritten)
	prometheus.MustRegister(logsQueueBlockedTotal)
}

type LogListener struct {
	c <-chan Applog
	q queue.PubSubQ
}

func logQueueName(appName string) string {
	return LogPubSubQueuePrefix + appName
}

func NewLogListener(a *App, filterLog Applog) (*LogListener, error) {
	factory, err := queue.Factory()
	if err != nil {
		return nil, err
	}
	pubSubQ, err := factory.PubSub(logQueueName(a.Name))
	if err != nil {
		return nil, err
	}
	subChan, err := pubSubQ.Sub()
	if err != nil {
		return nil, err
	}
	c := make(chan Applog, 10)
	go func() {
		defer close(c)
		for msg := range subChan {
			applog := Applog{}
			err := json.Unmarshal(msg, &applog)
			if err != nil {
				log.Errorf("Unparsable log message, ignoring: %s", string(msg))
				continue
			}
			if (filterLog.Source == "" || filterLog.Source == applog.Source) &&
				(filterLog.Unit == "" || filterLog.Unit == applog.Unit) {
				c <- applog
			}
		}
	}()
	l := LogListener{c: c, q: pubSubQ}
	return &l, nil
}

func (l *LogListener) ListenChan() <-chan Applog {
	return l.c
}

func (l *LogListener) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("Recovered panic closing listener (possible double close): %v", r)
		}
	}()
	err = l.q.UnSub()
	return
}

func notify(appName string, messages []interface{}) {
	factory, err := queue.Factory()
	if err != nil {
		log.Errorf("Error on logs notify: %s", err)
		return
	}
	pubSubQ, err := factory.PubSub(logQueueName(appName))
	if err != nil {
		log.Errorf("Error on logs notify: %s", err)
		return
	}
	for _, msg := range messages {
		bytes, err := json.Marshal(msg)
		if err != nil {
			log.Errorf("Error on logs notify: %s", err)
			continue
		}
		err = pubSubQ.Pub(bytes)
		if err != nil {
			log.Errorf("Error on logs notify: %s", err)
		}
	}
}

type logDispatcher struct {
	dispatchers map[string]*appLogDispatcher
	msgCh       chan *msgLog
}

type msgLog struct {
	dispatcher *appLogDispatcher
	msg        *Applog
}

func NewlogDispatcher(chanSize, numberGoroutines int) *logDispatcher {
	d := &logDispatcher{
		dispatchers: make(map[string]*appLogDispatcher),
		msgCh:       make(chan *msgLog, chanSize),
	}
	for i := 0; i < numberGoroutines; i++ {
		go d.runWriter()
	}
	logsQueueSize.Set(float64(chanSize))
	dispatchersCurrent.Inc()
	return d
}

func (d *logDispatcher) runWriter() {
	notifyMessages := make([]interface{}, 1)
	for msgWithDispatcher := range d.msgCh {
		logsInQueue.Dec()
		notifyMessages[0] = msgWithDispatcher.msg
		notify(msgWithDispatcher.msg.AppName, notifyMessages)
		select {
		case msgWithDispatcher.dispatcher.toFlush <- msgWithDispatcher.msg:
		case <-msgWithDispatcher.dispatcher.done:
			return
		}
	}
}

func (d *logDispatcher) Send(msg *Applog) {
	logsInQueue.Inc()
	appName := msg.AppName
	appD, ok := d.dispatchers[appName]
	if !ok {
		appD = newAppLogDispatcher(appName)
		d.dispatchers[appName] = appD
	}
	msgWithDispatcher := &msgLog{dispatcher: appD, msg: msg}
	select {
	case d.msgCh <- msgWithDispatcher:
	default:
		t0 := time.Now()
		d.msgCh <- msgWithDispatcher
		logsQueueBlockedTotal.Add(time.Since(t0).Seconds())
	}
}

func (d *logDispatcher) Stop() {
	for appName, appD := range d.dispatchers {
		delete(d.dispatchers, appName)
		close(appD.done)
	}
	close(d.msgCh)
	dispatchersCurrent.Dec()
}

type appLogDispatcher struct {
	appName string
	done    chan bool
	toFlush chan *Applog
}

func newAppLogDispatcher(appName string) *appLogDispatcher {
	d := &appLogDispatcher{
		appName: appName,
		done:    make(chan bool),
		toFlush: make(chan *Applog),
	}
	go d.runFlusher()
	return d
}

func (d *appLogDispatcher) runFlusher() {
	t := time.NewTimer(bulkMaxWaitTime)
	pos := 0
	sz := 200
	bulkBuffer := make([]interface{}, sz)
	for {
		var flush bool
		select {
		case <-d.done:
			return
		case msg := <-d.toFlush:
			if pos == sz {
				flush = true
				break
			}
			bulkBuffer[pos] = msg
			pos++
			flush = sz == pos
		case <-t.C:
			flush = pos > 0
			t.Reset(bulkMaxWaitTime)
		}
		if flush {
			conn, err := db.LogConn()
			if err != nil {
				log.Errorf("[log flusher] unable to connect to mongodb: %s", err)
				continue
			}
			coll := conn.Logs(d.appName)
			err = coll.Insert(bulkBuffer[:pos]...)
			coll.Close()
			if err != nil {
				log.Errorf("[log flusher] unable to insert logs: %s", err)
				continue
			}
			logsWritten.Add(float64(pos))
			pos = 0
		}
	}
}
