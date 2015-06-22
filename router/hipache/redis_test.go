// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import (
	"errors"
	"sync/atomic"

	"github.com/garyburd/redigo/redis"
	"gopkg.in/check.v1"
)

// CmdType represents a type of command, it may be a send or a do.
type CmdType byte

const (
	// CmdSend represents a send call in redigo.
	CmdSend CmdType = iota
	// CmdDo represents a do call in redigo.
	CmdDo
)

func ClearRedisKeys(keysPattern string, conn redis.Conn, c *check.C) {
	result, err := conn.Do("KEYS", keysPattern)
	c.Assert(err, check.IsNil)
	keys := result.([]interface{})
	for _, key := range keys {
		keyName := string(key.([]byte))
		conn.Do("DEL", keyName)
	}
}

// RedisCommand is a command sent to the redis server.
type RedisCommand struct {
	Cmd  string
	Args []interface{}
	Type CmdType
}

// FakeRedisConn is a fake implementation of redis.Conn.
//
// It's useful for mocks only. You may use it with a pool:
//
//     fakePool := redis.NewPool(func() (redis.Conn, error) {
//         return &FakeRedisConn{}, nil
//     }, 10)
//     conn := fakePool.Get()
type FakeRedisConn struct {
	Cmds   []RedisCommand
	closed int32
}

// Close closes the connection.
func (c *FakeRedisConn) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return errors.New("connection already closed")
	}
	return nil
}

// Err returns the last error.
func (c *FakeRedisConn) Err() error {
	return nil
}

// Do executes a do command, storing the arguments in the internal slice of
// commands. It doesn't return anything.
func (c *FakeRedisConn) Do(cmd string, args ...interface{}) (interface{}, error) {
	return nil, c.cmd(CmdDo, cmd, args)
}

// Send executes a send command, storing the arguments in the internal slice of
// commands. It doesn't return anything.
func (c *FakeRedisConn) Send(cmd string, args ...interface{}) error {
	return c.cmd(CmdSend, cmd, args)
}

func (c *FakeRedisConn) cmd(tp CmdType, cmd string, args []interface{}) error {
	if cmd != "" {
		c.Cmds = append(c.Cmds, RedisCommand{Cmd: cmd, Args: args, Type: tp})
	}
	return nil
}

// Flush does not do anything in the fake connection.
func (*FakeRedisConn) Flush() error {
	return nil
}

// Receive does not do anything in the fake connection.
func (*FakeRedisConn) Receive() (interface{}, error) {
	return nil, nil
}

// GetCommands return the list of commands of the given type.
func (c *FakeRedisConn) GetCommands(tp CmdType) []RedisCommand {
	var cmds []RedisCommand
	for _, cmd := range c.Cmds {
		if cmd.Type == tp {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

// FailingFakeRedisConn is a fake connection that fails to execute commands
// (via Do and/or Send).
type FailingFakeRedisConn struct {
	FakeRedisConn
}

func (c *FailingFakeRedisConn) Do(cmd string, args ...interface{}) (interface{}, error) {
	return nil, errors.New("I can't do that.")
}

func (c *FailingFakeRedisConn) Send(cmd string, args ...interface{}) error {
	return errors.New("I can't do that.")
}

// ResultCommandRedisConn is a fake connection that returns a result in the Do
// command.
type ResultCommandRedisConn struct {
	*FakeRedisConn
	Reply        map[string]interface{}
	DefaultReply interface{}
}

// Do returns the result for a command as specified by the Reply map. If no
// command is provided, it will return DefaultReply.
func (c *ResultCommandRedisConn) Do(cmd string, args ...interface{}) (interface{}, error) {
	c.FakeRedisConn.Do(cmd, args...)
	if c.Reply == nil {
		return c.DefaultReply, nil
	}
	if reply := c.Reply[cmd]; reply != nil {
		return reply, nil
	}
	return c.DefaultReply, nil
}
