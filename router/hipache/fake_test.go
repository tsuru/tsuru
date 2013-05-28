// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import "errors"

type command struct {
	cmd  string
	args []interface{}
}

type fakeConn struct {
	cmds []command
}

func (c *fakeConn) Close() error {
	return nil
}

func (c *fakeConn) Err() error {
	return nil
}

func (c *fakeConn) Do(cmd string, args ...interface{}) (interface{}, error) {
	c.cmds = append(c.cmds, command{cmd: cmd, args: args})
	return nil, nil
}

func (c *fakeConn) Send(cmd string, args ...interface{}) error {
	return nil
}

func (c *fakeConn) Flush() error {
	return nil
}

func (c *fakeConn) Receive() (interface{}, error) {
	return nil, nil
}

type failingFakeConn struct {
	fakeConn
}

func (c *failingFakeConn) Do(cmd string, args ...interface{}) (interface{}, error) {
	return nil, errors.New("I can't do that.")
}

type resultCommandConn struct {
	*fakeConn
	reply        map[string]interface{}
	defaultReply interface{}
}

func (c *resultCommandConn) Do(cmd string, args ...interface{}) (interface{}, error) {
	c.fakeConn.Do(cmd, args...)
	if c.defaultReply != nil {
		return c.defaultReply, nil
	}
	return c.reply[cmd], nil
}
