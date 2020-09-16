// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"fmt"
	"strings"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/log"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type logListener struct {
	c       <-chan appTypes.Applog
	logConn *logStorage
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

func newLogListener(svc appTypes.AppLogStorage, args appTypes.ListLogArgs) (*logListener, error) {
	conn, err := logConn()
	if err != nil {
		return nil, err
	}
	c := make(chan appTypes.Applog, 10)
	quit := make(chan struct{})
	coll := conn.appLogCollection(args.AppName)
	var lastLog appTypes.Applog
	err = coll.Find(nil).Sort("-_id").Limit(1).One(&lastLog)
	if err == mgo.ErrNotFound {
		// Tail cursors do not work correctly if the collection is empty (the
		// Next() call wouldn't block). So if the collection is empty we insert
		// the very first log line in it. This is quite rare in the real world
		// though so the impact of this extra log message is really small.
		err = svc.InsertApp(args.AppName, &appTypes.Applog{
			Date:    time.Now().In(time.UTC),
			Message: "Logs initialization",
			Source:  "tsuru",
			AppName: args.AppName,
		})
		if err != nil {
			conn.Close()
			return nil, err
		}
		err = coll.Find(nil).Sort("-_id").Limit(1).One(&lastLog)
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	lastId := lastLog.MongoID
	mkQuery := func() bson.M {
		m := makeQuery(args)
		m["_id"] = bson.M{"$gt": lastId}
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
			var applog appTypes.Applog
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
	l := logListener{c: c, logConn: conn, quit: quit}
	return &l, nil
}

func (l *logListener) Chan() <-chan appTypes.Applog {
	return l.c
}

func (l *logListener) Close() {
	l.logConn.Close()
	if l.quit != nil {
		close(l.quit)
		l.quit = nil
	}
}
