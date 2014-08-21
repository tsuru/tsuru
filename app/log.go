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
)

type LogListener struct {
	C <-chan Applog
	q queue.PubSubQ
}

func logQueueName(appName string) string {
	return "pubsub:" + appName
}

func NewLogListener(a *App, filterLog Applog) (*LogListener, error) {
	factory, err := queue.Factory()
	if err != nil {
		return nil, err
	}
	pubSubQ, err := factory.Get(logQueueName(a.Name))
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
	pubSubQ, err := factory.Get(logQueueName(appName))
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

// LogRemove removes the app log.
func LogRemove(a *App) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if a != nil {
		return conn.Logs(a.Name).DropCollection()
	}
	colls, err := conn.LogsCollections()
	if err != nil {
		return err
	}
	for _, coll := range colls {
		err = coll.DropCollection()
		if err != nil {
			log.Errorf("Error trying to drop collection %s", coll.Name)
		}
	}
	return nil
}
