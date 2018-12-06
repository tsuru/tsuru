// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
)

var (
	bulkMaxWaitMongoTime = 1 * time.Second
	bulkMaxNumberMsgs    = 1000
	bulkQueueMaxSize     = 10000

	buckets = append([]float64{0.1, 0.5}, prometheus.ExponentialBuckets(1, 1.6, 15)...)

	logsInQueue = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsuru_logs_queue_current",
		Help: "The current number of log entries in dispatcher queue.",
	})

	logsInAppQueues = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tsuru_logs_app_queues_current",
		Help: "The current number of log entries in app queues.",
	}, []string{"app"})

	logsQueueBlockedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_logs_queue_blocked_seconds_total",
		Help: "The total time spent blocked trying to add log to queue.",
	})

	logsQueueSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsuru_logs_dispatcher_queue_size",
		Help: "The max number of log entries in a dispatcher queue.",
	})

	logsEnqueued = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_logs_enqueued_total",
		Help: "The number of log entries enqueued for processing.",
	}, []string{"app"})

	logsWritten = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_logs_write_total",
		Help: "The number of log entries written to mongo.",
	}, []string{"app"})

	logsDropped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_logs_dropped_total",
		Help: "The number of log entries dropped due to full buffers.",
	}, []string{"app"})

	logsMongoFullLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "tsuru_logs_mongo_full_duration_seconds",
		Help:    "The latency distributions for log messages to be stored in database.",
		Buckets: buckets,
	})

	logsMongoLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "tsuru_logs_mongo_duration_seconds",
		Help:    "The latency distributions for log messages to be stored in database.",
		Buckets: buckets,
	})
)

func init() {
	prometheus.MustRegister(logsInQueue)
	prometheus.MustRegister(logsInAppQueues)
	prometheus.MustRegister(logsQueueSize)
	prometheus.MustRegister(logsEnqueued)
	prometheus.MustRegister(logsWritten)
	prometheus.MustRegister(logsDropped)
	prometheus.MustRegister(logsQueueBlockedTotal)
	prometheus.MustRegister(logsMongoFullLatency)
	prometheus.MustRegister(logsMongoLatency)
}

type LogListener struct {
	c       <-chan Applog
	logConn *db.LogStorage
	quit    chan struct{}
}

func isCappedPositionLost(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "CappedPositionLost")
}

func isSessionClosed(r interface{}) bool {
	return fmt.Sprintf("%v", r) == "Session already closed"
}

func NewLogListener(a *App, filterLog Applog) (*LogListener, error) {
	conn, err := db.LogConn()
	if err != nil {
		return nil, err
	}
	c := make(chan Applog, 10)
	quit := make(chan struct{})
	coll := conn.Logs(a.Name)
	var lastLog Applog
	err = coll.Find(nil).Sort("-_id").Limit(1).One(&lastLog)
	if err == mgo.ErrNotFound {
		// Tail cursors do not work correctly if the collection is empty (the
		// Next() call wouldn't block). So if the collection is empty we insert
		// the very first log line in it. This is quite rare in the real world
		// though so the impact of this extra log message is really small.
		err = a.Log("Logs initialization", "tsuru", "")
		if err != nil {
			return nil, err
		}
		err = coll.Find(nil).Sort("-_id").Limit(1).One(&lastLog)
	}
	if err != nil {
		return nil, err
	}
	lastId := lastLog.MongoID
	mkQuery := func() bson.M {
		m := bson.M{
			"_id": bson.M{"$gt": lastId},
		}
		if filterLog.Source != "" {
			m["source"] = filterLog.Source
		}
		if filterLog.Unit != "" {
			m["unit"] = filterLog.Unit
		}
		return m
	}
	query := coll.Find(mkQuery())
	tailTimeout := 10 * time.Second
	iter := query.Sort("$natural").Tail(tailTimeout)
	go func() {
		defer close(c)
		defer func() {
			if r := recover(); r != nil {
				if isSessionClosed(r) {
					return
				}
				panic(err)
			}
		}()
		for {
			var applog Applog
			for iter.Next(&applog) {
				lastId = applog.MongoID
				select {
				case c <- applog:
				case <-quit:
					iter.Close()
					return
				}
			}
			if iter.Timeout() {
				continue
			}
			if err := iter.Err(); err != nil {
				if !isCappedPositionLost(err) {
					log.Errorf("error tailing logs: %v", err)
					iter.Close()
					return
				}
			}
			iter.Close()
			query = coll.Find(mkQuery())
			iter = query.Sort("$natural").Tail(tailTimeout)
		}
	}()
	l := LogListener{c: c, logConn: conn, quit: quit}
	return &l, nil
}

func (l *LogListener) ListenChan() <-chan Applog {
	return l.c
}

func (l *LogListener) Close() {
	l.logConn.Close()
	if l.quit != nil {
		close(l.quit)
		l.quit = nil
	}
}

type LogDispatcher struct {
	mu             sync.RWMutex
	dispatchers    map[string]*appLogDispatcher
	msgCh          chan *msgWithTS
	shuttingDown   int32
	doneProcessing chan struct{}
}

type msgWithTS struct {
	msg        *Applog
	arriveTime time.Time
}

func NewlogDispatcher(chanSize int) *LogDispatcher {
	d := &LogDispatcher{
		dispatchers:    make(map[string]*appLogDispatcher),
		msgCh:          make(chan *msgWithTS, chanSize),
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
	for msgExtra := range d.msgCh {
		if msgExtra == nil {
			break
		}
		logsInQueue.Dec()
		appD := d.getMessageDispatcher(msgExtra.msg)
		appD.send(msgExtra)
	}
}

func (d *LogDispatcher) Send(msg *Applog) error {
	if atomic.LoadInt32(&d.shuttingDown) == 1 {
		return errors.New("log dispatcher is shutting down")
	}
	logsInQueue.Inc()
	logsEnqueued.WithLabelValues(msg.AppName).Inc()
	msgExtra := &msgWithTS{msg: msg, arriveTime: time.Now()}
	select {
	case d.msgCh <- msgExtra:
	default:
		t0 := time.Now()
		d.msgCh <- msgExtra
		logsQueueBlockedTotal.Add(time.Since(t0).Seconds())
	}
	return nil
}

func (a *LogDispatcher) String() string {
	return "log dispatcher"
}

func (d *LogDispatcher) Shutdown(ctx context.Context) error {
	atomic.StoreInt32(&d.shuttingDown, 1)
	d.msgCh <- nil
	<-d.doneProcessing
	logsInQueue.Set(0)
	for _, appD := range d.dispatchers {
		appD.stopWait()
	}
	return nil
}

type appLogDispatcher struct {
	appName string
	*bulkProcessor
}

func newAppLogDispatcher(appName string) *appLogDispatcher {
	d := &appLogDispatcher{
		bulkProcessor: initBulkProcessor(bulkMaxWaitMongoTime, bulkMaxNumberMsgs, appName),
		appName:       appName,
	}
	d.flushable = d
	go d.run()
	return d
}

func (d *appLogDispatcher) flush(msgs []interface{}, lastMessage *msgWithTS) bool {
	conn, err := db.LogConn()
	if err != nil {
		log.Errorf("[log flusher] unable to connect to mongodb: %s", err)
		return false
	}
	coll := conn.Logs(d.appName)
	err = coll.Insert(msgs...)
	coll.Close()
	if err != nil {
		log.Errorf("[log flusher] unable to insert logs: %s", err)
		return false
	}
	if lastMessage != nil {
		logsMongoLatency.Observe(time.Since(lastMessage.arriveTime).Seconds())
		logsMongoFullLatency.Observe(time.Since(lastMessage.msg.Date).Seconds())
	}
	logsWritten.WithLabelValues(d.appName).Add(float64(len(msgs)))
	return true
}

type bulkProcessor struct {
	appName     string
	maxWaitTime time.Duration
	bulkSize    int
	finished    chan struct{}
	ch          chan *msgWithTS
	nextNotify  *time.Timer
	flushable   interface {
		flush([]interface{}, *msgWithTS) bool
	}
}

func initBulkProcessor(maxWait time.Duration, bulkSize int, appName string) *bulkProcessor {
	queueSize, err := config.GetInt("logs:queue-size")
	if err != nil || queueSize == 0 {
		queueSize = bulkQueueMaxSize
	}
	return &bulkProcessor{
		appName:     appName,
		maxWaitTime: maxWait,
		bulkSize:    bulkSize,
		finished:    make(chan struct{}),
		ch:          make(chan *msgWithTS, queueSize),
		nextNotify:  time.NewTimer(0),
	}
}

func (p *bulkProcessor) send(msg *msgWithTS) {
	select {
	case p.ch <- msg:
		logsInAppQueues.WithLabelValues(p.appName).Set(float64(len(p.ch)))
	default:
		logsDropped.WithLabelValues(p.appName).Inc()
		select {
		case <-p.nextNotify.C:
			log.Errorf("dropping log messages to mongodb due to full channel buffer. app: %q, len: %d", msg.msg.AppName, len(p.ch))
			p.nextNotify.Reset(time.Minute)
		default:
		}
	}
}

func (p *bulkProcessor) stopWait() {
	p.ch <- nil
	<-p.finished
}

func (p *bulkProcessor) run() {
	defer close(p.finished)
	t := time.NewTimer(p.maxWaitTime)
	pos := 0
	bulkBuffer := make([]interface{}, p.bulkSize)
	shouldReturn := false
	var lastMessage *msgWithTS
	for {
		var flush bool
		select {
		case msgExtra := <-p.ch:
			logsInAppQueues.WithLabelValues(p.appName).Set(float64(len(p.ch)))
			if msgExtra == nil {
				flush = true
				shouldReturn = true
				break
			}
			if pos == p.bulkSize {
				flush = true
				break
			}
			lastMessage = msgExtra
			bulkBuffer[pos] = msgExtra.msg
			pos++
			flush = p.bulkSize == pos
		case <-t.C:
			flush = true
			t.Reset(p.maxWaitTime)
		}
		if flush && pos > 0 {
			if p.flushable.flush(bulkBuffer[:pos], lastMessage) {
				lastMessage = nil
				pos = 0
			}
		}
		if shouldReturn {
			return
		}
	}
}
