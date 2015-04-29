// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rectest

import (
	"time"

	"github.com/tsuru/tsuru/db"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
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

func (isRecordedChecker) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "IsRecorded", Params: []string{"action"}}
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
	errs := make(chan error, 1)
	quit := make(chan int8)
	defer close(quit)
	go func() {
		for {
			select {
			case <-quit:
				return
			default:
				var a userAction
				if err := conn.UserActions().Find(query).One(&a); err == nil {
					done <- a
					return
				} else if err != mgo.ErrNotFound {
					errs <- err
					return
				}
			}
		}
	}()
	var got userAction
	select {
	case got = <-done:
	case err := <-errs:
		return false, err.Error()
	case <-time.After(2e9):
		return false, "Action not in the database"
	}
	if got.Date.IsZero() {
		return false, "Action was not recorded using rec.Log"
	}
	return true, ""
}

var IsRecorded check.Checker = isRecordedChecker{}
