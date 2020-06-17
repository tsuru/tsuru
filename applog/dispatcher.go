// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/log"
	appTypes "github.com/tsuru/tsuru/types/app"
	"golang.org/x/time/rate"
)

var (
	bulkMaxWaitMongoTime = 1 * time.Second
	bulkMaxNumberMsgs    = 1000
	bulkQueueMaxSize     = 10000

	rateLimitWarningInterval = 5 * time.Second
	globalRateLimiter        = rate.NewLimiter(rate.Inf, 1)

	buckets = append([]float64{0.1, 0.5}, prometheus.ExponentialBuckets(1, 1.6, 15)...)

	logsInQueue = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "queue_current",
		Help:      "The current number of log entries in dispatcher queue.",
	})

	logsInAppQueues = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "app_queues_current",
		Help:      "The current number of log entries in app queues.",
	}, []string{"app"})

	logsQueueBlockedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "queue_blocked_seconds_total",
		Help:      "The total time spent blocked trying to add log to queue.",
	})

	logsQueueSize = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "dispatcher_queue_size",
		Help:      "The max number of log entries in a dispatcher queue.",
	})

	logsEnqueued = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "enqueued_total",
		Help:      "The number of log entries enqueued for processing.",
	}, []string{"app"})

	logsWritten = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "write_total",
		Help:      "The number of log entries written to mongo.",
	}, []string{"app"})

	logsDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "dropped_total",
		Help:      "The number of log entries dropped due to full buffers.",
	}, []string{"app"})

	logsDroppedRateLimit = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "dropped_rate_limit_total",
		Help:      "The number of log entries dropped due to rate limit exceeded.",
	}, []string{"app"})

	logsMongoFullLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "mongo_full_duration_seconds",
		Help:      "The latency distributions for log messages to be stored in database.",
		Buckets:   buckets,
	})

	logsMongoLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "mongo_duration_seconds",
		Help:      "The latency distributions for log messages to be stored in database.",
		Buckets:   buckets,
	})
)

type logDispatcher struct {
	mu             sync.RWMutex
	dispatchers    map[string]*appLogDispatcher
	msgCh          chan *msgWithTS
	shuttingDown   int32
	doneProcessing chan struct{}
	storage        appTypes.AppLogStorage
}

type msgWithTS struct {
	msg        *appTypes.Applog
	arriveTime time.Time
}

func newlogDispatcher(chanSize int, storage appTypes.AppLogStorage) *logDispatcher {
	d := &logDispatcher{
		dispatchers:    make(map[string]*appLogDispatcher),
		msgCh:          make(chan *msgWithTS, chanSize),
		doneProcessing: make(chan struct{}),
		storage:        storage,
	}
	go d.runWriter()
	logsQueueSize.Set(float64(chanSize))
	return d
}

func (d *logDispatcher) getMessageDispatcher(msg *appTypes.Applog) *appLogDispatcher {
	appName := msg.AppName
	d.mu.RLock()
	appD, ok := d.dispatchers[appName]
	if !ok {
		d.mu.RUnlock()
		d.mu.Lock()
		appD, ok = d.dispatchers[appName]
		if !ok {
			appD = newAppLogDispatcher(appName, d.storage)
			d.dispatchers[appName] = appD
		}
		d.mu.Unlock()
	} else {
		d.mu.RUnlock()
	}
	return appD
}

func (d *logDispatcher) runWriter() {
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

func (d *logDispatcher) send(msg *appTypes.Applog) error {
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

func (d *logDispatcher) shutdown(ctx context.Context) error {
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
	storage appTypes.AppLogStorage
	*bulkProcessor
}

func newAppLogDispatcher(appName string, storage appTypes.AppLogStorage) *appLogDispatcher {
	d := &appLogDispatcher{
		bulkProcessor: initBulkProcessor(bulkMaxWaitMongoTime, bulkMaxNumberMsgs, appName),
		appName:       appName,
		storage:       storage,
	}
	d.flushable = d
	go d.run()
	return d
}

func (d *appLogDispatcher) flush(msgs []*appTypes.Applog, lastMessage *msgWithTS) error {
	err := d.storage.InsertApp(d.appName, msgs...)
	if err != nil {
		return err
	}
	if lastMessage != nil {
		logsMongoLatency.Observe(time.Since(lastMessage.arriveTime).Seconds())
		logsMongoFullLatency.Observe(time.Since(lastMessage.msg.Date).Seconds())
	}
	logsWritten.WithLabelValues(d.appName).Add(float64(len(msgs)))
	return nil
}

type bulkProcessor struct {
	appName     string
	maxWaitTime time.Duration
	bulkSize    int
	finished    chan struct{}
	ch          chan *msgWithTS
	nextNotify  *time.Timer
	flushable   interface {
		flush([]*appTypes.Applog, *msgWithTS) error
	}
}

func initBulkProcessor(maxWait time.Duration, bulkSize int, appName string) *bulkProcessor {
	queueSize, err := config.GetInt("log:queue-size")
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

func (p *bulkProcessor) rateLimitWarning(rateLimit int) *appTypes.Applog {
	return &appTypes.Applog{
		AppName: p.appName,
		Date:    time.Now(),
		Message: fmt.Sprintf("Log messages dropped due to exceeded rate limit. Limit: %v logs/s.", rateLimit),
		Source:  "tsuru",
		Unit:    "api",
	}
}

func (p *bulkProcessor) globalRateLimitWarning() *appTypes.Applog {
	return &appTypes.Applog{
		AppName: p.appName,
		Date:    time.Now(),
		Message: fmt.Sprintf("Log messages dropped due to exceeded global rate limit. Global Limit: %v logs/s.", globalRateLimiter.Limit()),
		Source:  "tsuru",
		Unit:    "api",
	}
}

func (p *bulkProcessor) flushErrorMessage(err error) *appTypes.Applog {
	return &appTypes.Applog{
		AppName: p.appName,
		Date:    time.Now(),
		Message: fmt.Sprintf("Log messages dropped due to mongodb insert error: %v", err),
		Source:  "tsuru",
		Unit:    "api",
	}
}

func updateLogRateLimiter(rateLimiter *rate.Limiter) *rate.Limiter {
	globalRateLimit, _ := config.GetInt("log:global-app-log-rate-limit")
	if globalRateLimit <= 0 {
		globalRateLimiter.SetLimit(rate.Inf)
	} else if globalRateLimit != int(globalRateLimiter.Limit()) {
		globalRateLimiter.SetLimitAt(time.Now().Add(-2*time.Second), rate.Limit(globalRateLimit))
	}
	rateLimit, _ := config.GetInt("log:app-log-rate-limit")
	if rateLimit <= 0 {
		return nil
	}
	if rateLimiter == nil || rateLimiter.Burst() != rateLimit {
		return rate.NewLimiter(rate.Limit(rateLimit), rateLimit)
	}
	return rateLimiter
}

func (p *bulkProcessor) run() {
	defer close(p.finished)
	t := time.NewTimer(p.maxWaitTime)
	pos := 0
	bulkBuffer := make([]*appTypes.Applog, p.bulkSize)
	shouldReturn := false
	var lastMessage *msgWithTS
	var lastRateNotice time.Time
	logsInAppQueue := logsInAppQueues.WithLabelValues(p.appName)
	logsDropped := logsDroppedRateLimit.WithLabelValues(p.appName)
	rateLimiter := updateLogRateLimiter(nil)
	for {
		var flush bool
		select {
		case msgExtra := <-p.ch:
			logsInAppQueue.Set(float64(len(p.ch)))
			if msgExtra == nil {
				flush = true
				shouldReturn = true
				break
			}
			if pos == p.bulkSize {
				pos--
			}

			globalAllow := globalRateLimiter.Allow()
			if !globalAllow || (rateLimiter != nil && !rateLimiter.Allow()) {
				logsDropped.Inc()
				if time.Since(lastRateNotice) > rateLimitWarningInterval {
					lastRateNotice = time.Now()
					var warning *appTypes.Applog
					if globalAllow {
						warning = p.rateLimitWarning(rateLimiter.Burst())
					} else {
						warning = p.globalRateLimitWarning()
					}
					bulkBuffer[pos] = warning
					pos++
				}
				flush = p.bulkSize == pos
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
			err := p.flushable.flush(bulkBuffer[:pos], lastMessage)
			if err == nil {
				pos = 0
				lastMessage = nil
			} else {
				bulkBuffer[0] = p.flushErrorMessage(err)
				pos = 1
			}
			rateLimiter = updateLogRateLimiter(rateLimiter)
		}
		if shouldReturn {
			return
		}
	}
}
