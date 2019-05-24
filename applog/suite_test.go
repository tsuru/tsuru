// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"testing"
	"time"

	"github.com/globalsign/mgo"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "app_applog_pkg_tests")
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	conn.Apps().Database.DropDatabase()
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	s.dropAppLogCollections(c)
}

func (s *S) dropAppLogCollections(c *check.C) {
	logConn, err := db.LogConn()
	c.Assert(err, check.IsNil)
	defer logConn.Close()
	logdb := logConn.AppLogCollection("myapp").Database
	colls, err := logdb.CollectionNames()
	if err != nil {
		return
	}
	for _, coll := range colls {
		if len(coll) > 5 && coll[0:5] == "logs_" {
			logdb.C(coll).DropCollection()
		}
	}
}

func createAppLogCollection(appName string) error {
	conn, err := db.LogConn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.CreateAppLogCollection(appName)
	// Ignore collection already exists error (code 48)
	if queryErr, ok := err.(*mgo.QueryError); !ok || queryErr.Code != 48 {
		return err
	}
	return nil
}

func insertLogs(appName string, logs []interface{}) error {
	conn, err := db.LogConn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.AppLogCollection(appName).Insert(logs...)
}

func compareLogsNoDate(c *check.C, logs1 []appTypes.Applog, logs2 []appTypes.Applog) {
	compareLogsDate(c, logs1, logs2, false)
}

func compareLogs(c *check.C, logs1 []appTypes.Applog, logs2 []appTypes.Applog) {
	compareLogsDate(c, logs1, logs2, true)
}

func compareLogsDate(c *check.C, logs1 []appTypes.Applog, logs2 []appTypes.Applog, compareDate bool) {
	for i := range logs1 {
		logs1[i].MongoID = ""
		logs1[i].Date = logs1[i].Date.UTC()
		if !compareDate {
			logs1[i].Date = time.Time{}
		}
	}
	for i := range logs2 {
		logs2[i].MongoID = ""
		logs2[i].Date = logs2[i].Date.UTC()
		if !compareDate {
			logs2[i].Date = time.Time{}
		}
	}
	c.Assert(logs1, check.DeepEquals, logs2)
}
