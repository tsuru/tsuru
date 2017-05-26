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

	bulkMaxWaitMongoTime = 2 * time.Second
	bulkMaxWaitRedisTime = 500 * time.Millisecond
	bulkMaxNumberMsgs    = 1000
	notifyGoroutines     = 10

	buckets = append([]float64{0.1, 0.5}, prometheus.ExponentialBuckets(1, 1.6, 15)...)

	logsInQueue = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsuru_logs_queue_current",
		Help: "The current number of log entries in dispatcher queue.",
	})

	logsInNotifyQueue = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsuru_logs_notify_queue_current",
		Help: "The current number of log entries in notify queue.",
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

	logsPublished = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_logs_published_total",
		Help: "The number of log entries published to redis.",
	})

	logsPublishLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "tsuru_logs_publish_duration_seconds",
		Help:    "The latency distributions for log messages to be published.",
		Buckets: buckets,
	})

	logsPublishFullLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "tsuru_logs_publish_full_duration_seconds",
		Help:    "The latency distributions for log messages to be published since being sent by app.",
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
	prometheus.MustRegister(logsInNotifyQueue)
	prometheus.MustRegister(logsQueueSize)
	prometheus.MustRegister(logsEnqueued)
	prometheus.MustRegister(logsWritten)
	prometheus.MustRegister(logsPublished)
	prometheus.MustRegister(logsQueueBlockedTotal)
	prometheus.MustRegister(logsPublishLatency)
	prometheus.MustRegister(logsPublishFullLatency)
	prometheus.MustRegister(logsMongoLatency)
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
	pubSubQ := queue.Factory().PubSub(logQueueName(a.Name))
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
	pubSubQ := queue.Factory().PubSub(logQueueName(appName))
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
	msgCh          chan *msgWithTS
	notifyMsgCh    chan *msgWithTS
	shuttingDown   int32
	doneProcessing chan struct{}
	doneNotifying  chan struct{}
}

type msgWithTS struct {
	msg        *Applog
	arriveTime time.Time
}

func NewlogDispatcher(chanSize int) *LogDispatcher {
	d := &LogDispatcher{
		dispatchers:    make(map[string]*appLogDispatcher),
		msgCh:          make(chan *msgWithTS, chanSize),
		notifyMsgCh:    make(chan *msgWithTS, chanSize),
		doneProcessing: make(chan struct{}),
		doneNotifying:  make(chan struct{}),
	}
	go d.runWriter()
	d.runNotifier()
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

func (d *LogDispatcher) runNotifier() {
	wg := sync.WaitGroup{}
	closeAll := make(chan struct{})
	loopNotifier := func() {
		nb := newNotifyBulk()
		defer func() {
			nb.stopWait()
			wg.Done()
		}()
		for {
			var msgExtra *msgWithTS
			select {
			case msgExtra = <-d.notifyMsgCh:
				logsInNotifyQueue.Dec()
			case <-closeAll:
				return
			}
			if msgExtra == nil {
				close(closeAll)
				return
			}
			nb.send(msgExtra)
		}
	}
	for i := 0; i < notifyGoroutines; i++ {
		wg.Add(1)
		go loopNotifier()
	}
	go func() {
		wg.Wait()
		logsInNotifyQueue.Set(0)
		close(d.doneNotifying)
	}()
}

func (d *LogDispatcher) runWriter() {
	defer close(d.doneProcessing)
	for msgExtra := range d.msgCh {
		logsInNotifyQueue.Inc()
		d.notifyMsgCh <- msgExtra
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
	logsEnqueued.Inc()
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

func (d *LogDispatcher) Shutdown() {
	atomic.StoreInt32(&d.shuttingDown, 1)
	d.msgCh <- nil
	<-d.doneProcessing
	<-d.doneNotifying
	logsInQueue.Set(0)
	for _, appD := range d.dispatchers {
		appD.stopWait()
	}
}

type notifyBulk struct {
	*bulkProcessor
	pubsubQ queue.PubSubQ
}

func newNotifyBulk() *notifyBulk {
	nb := &notifyBulk{
		bulkProcessor: initBulkProcessor(bulkMaxWaitRedisTime, bulkMaxNumberMsgs),
		pubsubQ:       queue.Factory().PubSub(""),
	}
	nb.bulkProcessor.flushable = nb
	go nb.run()
	return nb
}

func (nb *notifyBulk) flush(msgs []interface{}, lastMessage *msgWithTS) bool {
	pubMsgs := make([]queue.PubMsg, len(msgs))
	for i, msg := range msgs {
		appLogMsg := msg.(*Applog)
		data, err := json.Marshal(appLogMsg)
		if err != nil {
			log.Errorf("Error on logs notify marshal: %s", err)
			continue
		}
		pubMsgs[i] = queue.PubMsg{
			Message: data,
			Name:    logQueueName(appLogMsg.AppName),
		}
	}
	err := nb.pubsubQ.PubMulti(pubMsgs)
	if err != nil {
		log.Errorf("Error on logs notify publish: %s", err)
		return false
	}
	logsPublishLatency.Observe(time.Since(lastMessage.arriveTime).Seconds())
	logsPublishFullLatency.Observe(time.Since(lastMessage.msg.Date).Seconds())
	logsPublished.Add(float64(len(msgs)))
	return true
}

type appLogDispatcher struct {
	appName string
	*bulkProcessor
}

func newAppLogDispatcher(appName string) *appLogDispatcher {
	d := &appLogDispatcher{
		bulkProcessor: initBulkProcessor(bulkMaxWaitMongoTime, bulkMaxNumberMsgs),
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
	}
	logsWritten.Add(float64(len(msgs)))
	return true
}

type bulkProcessor struct {
	maxWaitTime time.Duration
	bulkSize    int
	quit        chan struct{}
	finished    chan struct{}
	ch          chan *msgWithTS
	flushable   interface {
		flush([]interface{}, *msgWithTS) bool
	}
}

func initBulkProcessor(maxWait time.Duration, bulkSize int) *bulkProcessor {
	return &bulkProcessor{
		maxWaitTime: maxWait,
		bulkSize:    bulkSize,
		quit:        make(chan struct{}),
		finished:    make(chan struct{}),
		ch:          make(chan *msgWithTS),
	}
}

func (p *bulkProcessor) send(msg *msgWithTS) {
	p.ch <- msg
}

func (p *bulkProcessor) stopWait() {
	close(p.quit)
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
		case <-p.quit:
			flush = pos > 0
			shouldReturn = true
		case msgExtra := <-p.ch:
			if pos == p.bulkSize {
				flush = true
				break
			}
			lastMessage = msgExtra
			bulkBuffer[pos] = msgExtra.msg
			pos++
			flush = p.bulkSize == pos
		case <-t.C:
			flush = pos > 0
			t.Reset(p.maxWaitTime)
		}
		if flush {
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
