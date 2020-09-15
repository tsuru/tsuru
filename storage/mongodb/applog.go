// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"errors"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/types/app"
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

func (s *applogStorage) List(ctx context.Context, args app.ListLogArgs) ([]app.Applog, error) {
	if args.AppName == "" {
		return nil, errors.New("unable to list logs with empty app name")
	}
	conn, err := db.LogConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	logs := []app.Applog{}
	q := makeQuery(args)
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

func (s *applogStorage) Watch(ctx context.Context, args app.ListLogArgs) (app.LogWatcher, error) {
	listener, err := newLogListener(s, args)
	if err != nil {
		return nil, err
	}
	return listener, nil
}

func makeQuery(args app.ListLogArgs) bson.M {
	q := bson.M{}
	if args.Source != "" {
		var sourceFilter interface{} = args.Source
		if args.InvertSource {
			sourceFilter = bson.M{"$ne": args.Source}
		}
		q["source"] = sourceFilter
	}
	if len(args.Units) > 0 {
		q["unit"] = bson.M{"$in": args.Units}
	}
	return q
}
