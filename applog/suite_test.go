// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "applog_pkg_tests")
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *S) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

type ServiceSuite struct {
	svcFunc func() (appTypes.AppLogService, error)
	svc     appTypes.AppLogService
}

func (s *ServiceSuite) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "applog_pkg_service_suite_tests")
}

func (s *ServiceSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *ServiceSuite) SetUpTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	s.svc, err = s.svcFunc()
	c.Assert(err, check.IsNil)
	c.Logf("Service: %T", s.svc)
}

func compareLogsNoDate(c *check.C, logs1 []appTypes.Applog, logs2 []appTypes.Applog) {
	compareLogsDate(c, logs1, logs2, false)
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
