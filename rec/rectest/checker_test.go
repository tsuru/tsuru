// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rectest

import (
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type CheckerSuite struct{}

var _ = check.Suite(CheckerSuite{})

func (CheckerSuite) SetUpTest(c *check.C) {
	config.Set("database:url", "localhost:27017")
	config.Set("database:name", "tsuru_rectest_test")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	action := map[string]interface{}{
		"user":   "glenda@tsuru.io",
		"action": "run-command",
		"extra":  []interface{}{"rm", "-rf", "/"},
		"date":   time.Now(),
	}
	err = conn.UserActions().Insert(action)
	c.Assert(err, check.IsNil)
	actionNoDate := map[string]interface{}{
		"user":   "glenda@tsuru.io",
		"action": "list-apps",
		"extra":  nil,
		"date":   nil,
	}
	err = conn.UserActions().Insert(actionNoDate)
	c.Assert(err, check.IsNil)
}

func (CheckerSuite) TearDownTest(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (CheckerSuite) TestIsRecordedInfo(c *check.C) {
	expected := &check.CheckerInfo{
		Name:   "IsRecorded",
		Params: []string{"action"},
	}
	c.Assert(isRecordedChecker{}.Info(), check.DeepEquals, expected)
}

func (CheckerSuite) TestIsRecordedCheckInvalidParameter(c *check.C) {
	result, error := isRecordedChecker{}.Check([]interface{}{"action"}, []string{"action"})
	c.Assert(result, check.Equals, false)
	c.Assert(error, check.Equals, "First parameter must be of type Action or *Action")
}

func (CheckerSuite) TestIsRecordedCheckWithValue(c *check.C) {
	action := Action{
		User:   "glenda@tsuru.io",
		Action: "run-command",
		Extra:  []interface{}{"rm", "-rf", "/"},
	}
	result, error := isRecordedChecker{}.Check([]interface{}{action}, []string{})
	c.Assert(result, check.Equals, true)
	c.Assert(error, check.Equals, "")
}

func (CheckerSuite) TestIsRecordedCheckWithReference(c *check.C) {
	action := Action{
		User:   "glenda@tsuru.io",
		Action: "run-command",
		Extra:  []interface{}{"rm", "-rf", "/"},
	}
	result, error := isRecordedChecker{}.Check([]interface{}{&action}, []string{})
	c.Assert(result, check.Equals, true)
	c.Assert(error, check.Equals, "")
}

func (CheckerSuite) TestIsRecordedNotInDatabase(c *check.C) {
	action := Action{
		User:   "glenda@tsuru.io",
		Action: "run-command",
		Extra:  []interface{}{"rm", "-rf", "/home"},
	}
	result, error := isRecordedChecker{}.Check([]interface{}{action}, []string{})
	c.Assert(result, check.Equals, false)
	c.Assert(error, check.Equals, "Action not in the database")
}

func (CheckerSuite) TestIsRecordedWithoutDate(c *check.C) {
	action := Action{
		User:   "glenda@tsuru.io",
		Action: "list-apps",
	}
	result, error := isRecordedChecker{}.Check([]interface{}{action}, []string{})
	c.Assert(result, check.Equals, false)
	c.Assert(error, check.Equals, "Action was not recorded using rec.Log")
}
