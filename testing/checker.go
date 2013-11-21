// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"github.com/globocom/tsuru/db"
	"launchpad.net/gocheck"
	"runtime"
	"time"
)

type Action struct {
	User   string
	Action string
	Extra  []interface{}
}

type userAction struct {
	Action
	Date time.Time
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
	query := map[string]interface{}{
		"user":   a.User,
		"action": a.Action,
	}
	if len(a.Extra) > 0 {
		query["extra"] = a.Extra
	}
	done := make(chan userAction, 1)
	quit := make(chan int8)
	defer close(quit)
	go func() {
		for {
			select {
			case <-quit:
				runtime.Goexit()
			default:
				var a userAction
				if err := conn.UserActions().Find(query).One(&a); err == nil {
					done <- a
					return
				}
				runtime.Gosched()
			}
		}
	}()
	var got userAction
	select {
	case got = <-done:
	case <-time.After(2e9):
		return false, "Action not in the database"
	}
	var empty time.Time
	if got.Date.Sub(empty.In(time.UTC)) == 0 {
		return false, "Action was not recorded using rec.Log"
	}
	return true, ""
}

var IsRecorded gocheck.Checker = isRecordedChecker{}
