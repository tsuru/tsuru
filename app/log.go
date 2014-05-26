// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/queue"
	"labix.org/v2/mgo/bson"
)

var UnsupportedPubSubErr = fmt.Errorf("Your queue backend doesn't support pubsub required for log streaming. Using redis as queue is recommended.")

type LogListener struct {
	C <-chan Applog
	q queue.PubSubQ
}

func logQueueName(appName string) string {
	return "pubsub:" + appName
}

func NewLogListener(a *App) (*LogListener, error) {
	factory, err := queue.Factory()
	if err != nil {
		return nil, err
	}
	q, err := factory.Get(logQueueName(a.Name))
	if err != nil {
		return nil, err
	}
	pubSubQ, ok := q.(queue.PubSubQ)
	if !ok {
		return nil, UnsupportedPubSubErr
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
			c <- applog
		}
	}()
	l := LogListener{C: c, q: pubSubQ}
	return &l, nil
}

func (l *LogListener) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recovered panic closing listener (possible double close): %#v", r)
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
	q, err := factory.Get(logQueueName(appName))
	if err != nil {
		log.Errorf("Error on logs notify: %s", err.Error())
		return
	}
	pubSubQ, ok := q.(queue.PubSubQ)
	if !ok {
		log.Debugf("Queue does not support pubsub, logs only in database.")
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

// LogRemove removes the app log.
func LogRemove(a *App) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if a != nil {
		_, err = conn.Logs().RemoveAll(bson.M{"appname": a.Name})
	} else {
		_, err = conn.Logs().RemoveAll(nil)
	}
	return err
}
