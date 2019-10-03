// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"errors"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/types/app"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type applogStorage struct{}

var _ app.AppLogStorage = &applogStorage{}

func (s *applogStorage) InsertApp(appName string, msgs ...*app.Applog) error {
	conn, err := db.LogConn()
	if err != nil {
		log.Errorf("[log insert] unable to connect to mongodb: %s", err)
		return err
	}
	defer conn.Close()
	coll, err := conn.CreateAppLogCollection(appName)
	if err != nil && !db.IsCollectionExistsError(err) {
		log.Errorf("[log insert] unable to create collection in mongodb: %s", err)
		return err
	}
	unsafeWrite, _ := config.GetBool("log:unsafe-write")
	if unsafeWrite {
		coll.Database.Session.SetSafe(nil)
	}
	msgsIface := make([]interface{}, len(msgs))
	for i := range msgs {
		msgsIface[i] = msgs[i]
	}
	err = coll.Insert(msgsIface...)
	if err != nil {
		log.Errorf("[log insert] unable to insert logs: %s", err)
		return err
	}
	return nil
}

func (s *applogStorage) List(args app.ListLogArgs) ([]app.Applog, error) {
	if args.AppName == "" {
		return nil, errors.New("unable to list logs with empty app name")
	}
	conn, err := db.LogConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	logs := []app.Applog{}
	q := bson.M{}
	if args.Source != "" {
		q["source"] = args.Source
	}
	if args.Unit != "" {
		q["unit"] = args.Unit
	}
	if args.Level > 0 {
		q["level"] = args.Level
	}
	if args.InvertFilters {
		for k, v := range q {
			q[k] = bson.M{"$ne": v}
		}
	}
	err = conn.AppLogCollection(args.AppName).Find(q).Sort("-$natural").Limit(args.Limit).All(&logs)
	if err != nil {
		return nil, err
	}
	l := len(logs)
	for i := 0; i < l/2; i++ {
		logs[i], logs[l-1-i] = logs[l-1-i], logs[i]
	}
	return logs, nil
}

func (s *applogStorage) Watch(appName, source, unit string) (app.LogWatcher, error) {
	listener, err := newLogListener(s, appName, appTypes.Applog{Source: source, Unit: unit})
	if err != nil {
		return nil, err
	}
	return listener, nil
}
