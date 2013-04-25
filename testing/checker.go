// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"github.com/globocom/tsuru/db"
	"launchpad.net/gocheck"
)

type Action struct {
	User   string
	Action string
	Extra  []interface{}
}

type isRecordedChecker struct{}

func (isRecordedChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "IsRecorded", Params: []string{"action"}}
}

func (isRecordedChecker) Check(params []interface{}, names []string) (bool, string) {
	var a Action
	switch params[0].(type) {
	case Action:
		a = params[0].(Action)
	case *Action:
		a = *params[0].(*Action)
	default:
		return false, "First parameter must be of type Action or *Action"
	}
	conn, err := db.Conn()
	if err != nil {
		panic("Could not connect to the database: " + err.Error())
	}
	defer conn.Close()
	ct, err := conn.UserActions().Find(a).Count()
	if err != nil || ct == 0 {
		return false, "Action not in the database"
	}
	return true, ""
}

var IsRecorded gocheck.Checker = isRecordedChecker{}
