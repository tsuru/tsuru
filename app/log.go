// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/queue"
)

var LogPubSubQueuePrefix = "pubsub:"
var bulkMaxWaitTime = time.Second

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
			err = fmt.Errorf("Recovered panic closing listener (possible double close): %v", r)
		}
	}()
	err = l.q.UnSub()
	return
}

func notify(appName string, messages []interface{}) {
	factory, err := queue.Factory()
	if err != nil {
		log.Errorf("Error on logs notify: %s", err.Error())
		return
	}
	pubSubQ, err := factory.PubSub(logQueueName(appName))
	if err != nil {
		log.Errorf("Error on logs notify: %s", err.Error())
		return
	}
	for _, msg := range messages {
		bytes, err := json.Marshal(msg)
		if err != nil {
			log.Errorf("Error on logs notify: %s", err.Error())
			continue
		}
		err = pubSubQ.Pub(bytes)
		if err != nil {
			log.Errorf("Error on logs notify: %s", err.Error())
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
	return d
}

func (d *logDispatcher) runWriter() {
	notifyMessages := make([]interface{}, 1)
	for msgWithDispatcher := range d.msgCh {
		notifyMessages[0] = msgWithDispatcher.msg
		notify(msgWithDispatcher.msg.AppName, notifyMessages)
		select {
		case msgWithDispatcher.dispatcher.toFlush <- msgWithDispatcher.msg:
		case <-msgWithDispatcher.dispatcher.done:
			return
		}
	}
}

func (d *logDispatcher) Send(msg *Applog) error {
	appName := msg.AppName
	appD, ok := d.dispatchers[appName]
	if !ok {
		appD = newAppLogDispatcher(appName)
		d.dispatchers[appName] = appD
	}
	msgWithDispatcher := &msgLog{dispatcher: appD, msg: msg}
	select {
	case d.msgCh <- msgWithDispatcher:
	case err := <-appD.errCh:
		delete(d.dispatchers, appName)
		return err
	}
	return nil
}

func (d *logDispatcher) Stop() error {
	var finalErr error
	for appName, appD := range d.dispatchers {
		delete(d.dispatchers, appName)
		close(appD.done)
		err := <-appD.errCh
		if err != nil {
			if finalErr == nil {
				finalErr = err
			} else {
				finalErr = fmt.Errorf("%s, %s", finalErr, err)
			}
		}
	}
	close(d.msgCh)
	return finalErr
}

type appLogDispatcher struct {
	appName string
	errCh   chan error
	done    chan bool
	toFlush chan *Applog
}

func newAppLogDispatcher(appName string) *appLogDispatcher {
	d := &appLogDispatcher{
		appName: appName,
		errCh:   make(chan error),
		done:    make(chan bool),
		toFlush: make(chan *Applog),
	}
	go d.runFlusher()
	return d
}

func (d *appLogDispatcher) runFlusher() {
	defer close(d.errCh)
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
			bulkBuffer[pos] = msg
			pos++
			flush = sz == pos
			if flush {
				t.Stop()
			} else {
				t.Reset(bulkMaxWaitTime)
			}
		case <-t.C:
			flush = pos > 0
		}
		if flush {
			conn, err := db.LogConn()
			if err != nil {
				d.errCh <- err
				return
			}
			coll := conn.Logs(d.appName)
			err = coll.Insert(bulkBuffer[:pos]...)
			coll.Close()
			if err != nil {
				d.errCh <- err
				return
			}
			pos = 0
		}
	}
}
