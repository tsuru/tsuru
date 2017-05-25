// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/queue"
)

var (
	LogPubSubQueuePrefix = "pubsub:"

	bulkMaxWaitTime   = time.Second
	bulkMaxNumberMsgs = 500

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

	logsEnqueued = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_logs_enqueued_total",
		Help: "The number of log entries enqueued for processing.",
	})

	logsWritten = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_logs_write_total",
		Help: "The number of log entries written to mongo.",
	})
)

func init() {
	prometheus.MustRegister(logsInQueue)
	prometheus.MustRegister(logsQueueSize)
	prometheus.MustRegister(logsEnqueued)
	prometheus.MustRegister(logsWritten)
	prometheus.MustRegister(logsQueueBlockedTotal)
}

type LogListener struct {
	c    <-chan Applog
	q    queue.PubSubQ
	quit chan struct{}
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
	quit := make(chan struct{})
	go func() {
		defer close(c)
		for {
			var msg []byte
			select {
			case msg = <-subChan:
			case <-quit:
				return
			}
			applog := Applog{}
			err := json.Unmarshal(msg, &applog)
			if err != nil {
				log.Errorf("Unparsable log message, ignoring: %s", string(msg))
				continue
			}
			if (filterLog.Source == "" || filterLog.Source == applog.Source) &&
				(filterLog.Unit == "" || filterLog.Unit == applog.Unit) {
				select {
				case c <- applog:
				case <-quit:
					return
				}
			}
		}
	}()
	l := LogListener{c: c, q: pubSubQ, quit: quit}
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
	close(l.quit)
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

type LogDispatcher struct {
	mu             sync.RWMutex
	sendMu         sync.Mutex
	dispatchers    map[string]*appLogDispatcher
	msgCh          chan *Applog
	shuttingDown   int32
	doneProcessing chan struct{}
}

func NewlogDispatcher(chanSize int) *LogDispatcher {
	d := &LogDispatcher{
		dispatchers:    make(map[string]*appLogDispatcher),
		msgCh:          make(chan *Applog, chanSize),
		doneProcessing: make(chan struct{}),
	}
	go d.runWriter()
	shutdown.Register(d)
	logsQueueSize.Set(float64(chanSize))
	return d
}

func (d *LogDispatcher) getMessageDispatcher(msg *Applog) *appLogDispatcher {
	appName := msg.AppName
	d.mu.RLock()
	appD, ok := d.dispatchers[appName]
	if !ok {
		d.mu.RUnlock()
		d.mu.Lock()
		appD, ok = d.dispatchers[appName]
		if !ok {
			appD = newAppLogDispatcher(appName)
			d.dispatchers[appName] = appD
		}
		d.mu.Unlock()
	} else {
		d.mu.RUnlock()
	}
	return appD
}

func (d *LogDispatcher) runWriter() {
	defer close(d.doneProcessing)
	notifyMessages := make([]interface{}, 1)
	for msg := range d.msgCh {
		if msg == nil {
			break
		}
		logsInQueue.Dec()
		appD := d.getMessageDispatcher(msg)
		notifyMessages[0] = msg
		notify(msg.AppName, notifyMessages)
		appD.toFlush <- msg
	}
}

func (d *LogDispatcher) Send(msg *Applog) error {
	if atomic.LoadInt32(&d.shuttingDown) == 1 {
		return errors.New("log dispatcher is shutting down")
	}
	logsInQueue.Inc()
	logsEnqueued.Inc()
	select {
	case d.msgCh <- msg:
	default:
		t0 := time.Now()
		d.msgCh <- msg
		logsQueueBlockedTotal.Add(time.Since(t0).Seconds())
	}
	return nil
}

func (a *LogDispatcher) String() string {
	return "log dispatcher"
}

func (d *LogDispatcher) Shutdown() {
	atomic.StoreInt32(&d.shuttingDown, 1)
	d.msgCh <- nil
	<-d.doneProcessing
	logsInQueue.Set(0)
	for _, appD := range d.dispatchers {
		close(appD.done)
		<-appD.finished
	}
}

type appLogDispatcher struct {
	appName  string
	done     chan struct{}
	finished chan struct{}
	toFlush  chan *Applog
}

func newAppLogDispatcher(appName string) *appLogDispatcher {
	d := &appLogDispatcher{
		appName:  appName,
		done:     make(chan struct{}),
		finished: make(chan struct{}),
		toFlush:  make(chan *Applog),
	}
	go d.runFlusher()
	return d
}

func (d *appLogDispatcher) runFlusher() {
	defer close(d.finished)
	t := time.NewTimer(bulkMaxWaitTime)
	pos := 0
	bulkBuffer := make([]interface{}, bulkMaxNumberMsgs)
	shouldReturn := false
	for {
		var flush bool
		select {
		case <-d.done:
			flush = pos > 0
			shouldReturn = true
		case msg := <-d.toFlush:
			if pos == bulkMaxNumberMsgs {
				flush = true
				break
			}
			bulkBuffer[pos] = msg
			pos++
			flush = bulkMaxNumberMsgs == pos
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
		if shouldReturn {
			return
		}
	}
}
