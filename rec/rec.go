// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rec provides types and functions for logging user actions, for
// auditing and statistics.
package rec

import (
	"github.com/globocom/tsuru/db"
	"time"
)

type userAction struct {
	User   string
	Action string
	Extra  []interface{}
	Date   time.Time
}

// Log stores an action in the database. It launches a goroutine, and may
// return an error in a channel.
func Log(user string, action string, extra ...interface{}) <-chan error {
	ch := make(chan error, 1)
	go func() {
		conn, err := db.Conn()
		if err != nil {
			ch <- err
			return
		}
		defer conn.Close()
		action := userAction{User: user, Action: action, Extra: extra, Date: time.Now().In(time.UTC)}
		if err := conn.UserActions().Insert(action); err != nil {
			ch <- err
		}
		close(ch)
	}()
	return ch
}
