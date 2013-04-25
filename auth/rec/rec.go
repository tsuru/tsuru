// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rec provides types and functions for logging user actions, for
// auditing and statistics.
package rec

type userAction struct {
	User   string
	Action string
	Extra  []interface{}
}

// Log stores an action in the database. It launches a goroutine, and may
// return an error in a channel.
func Log(user string, action string, extra ...interface{}) <-chan error {
	return nil
}
