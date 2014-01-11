// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rec provides types and functions for logging user actions, for
// auditing and statistics.
package rec

import (
	"errors"
	"github.com/globocom/tsuru/db"
	"time"
)

var (
	// Error returned when a user is not provided to the Log function.
	ErrMissingUser = errors.New("Missing user")

	// Error returned when an action is not provided to the Log function.
	ErrMissingAction = errors.New("Missing action")
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
		if user == "" {
			ch <- ErrMissingUser
			return
		}
		if action == "" {
			ch <- ErrMissingAction
			return
		}
		conn, err := db.NewStorage()
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
