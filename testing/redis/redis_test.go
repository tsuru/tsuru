// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redis

import (
	"testing"

	"github.com/garyburd/redigo/redis"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestClose(c *gocheck.C) {
	conn := FakeRedisConn{}
	err := conn.Close()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestDoubleClose(c *gocheck.C) {
	conn := FakeRedisConn{}
	err := conn.Close()
	c.Assert(err, gocheck.IsNil)
	err = conn.Close()
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "connection already closed")
}

func (s *S) TestErr(c *gocheck.C) {
	conn := FakeRedisConn{}
	c.Assert(conn.Err(), gocheck.IsNil)
}

func (s *S) TestDo(c *gocheck.C) {
	conn := FakeRedisConn{}
	result, err := conn.Do("GET", "something")
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.IsNil)
	expected := []RedisCommand{
		{Cmd: "GET", Args: []interface{}{"something"}, Type: CmdDo},
	}
	c.Assert(conn.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestDoEmptyCommand(c *gocheck.C) {
	conn := FakeRedisConn{}
	result, err := conn.Do("", "something", "otherthing")
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.IsNil)
	c.Assert(conn.Cmds, gocheck.IsNil)
}

func (s *S) TestSend(c *gocheck.C) {
	conn := FakeRedisConn{}
	err := conn.Send("GET", "something")
	c.Assert(err, gocheck.IsNil)
	expected := []RedisCommand{
		{Cmd: "GET", Args: []interface{}{"something"}, Type: CmdSend},
	}
	c.Assert(conn.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestSendEmptyCommand(c *gocheck.C) {
	conn := FakeRedisConn{}
	err := conn.Send("", "something", "otherthing")
	c.Assert(err, gocheck.IsNil)
	c.Assert(conn.Cmds, gocheck.IsNil)
}

func (s *S) TestFlush(c *gocheck.C) {
	conn := FakeRedisConn{}
	c.Assert(conn.Flush(), gocheck.IsNil)
}

func (s *S) TestReceive(c *gocheck.C) {
	conn := FakeRedisConn{}
	result, err := conn.Receive()
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.IsNil)
}

func (s *S) TestGetCommands(c *gocheck.C) {
	doCommands := []RedisCommand{
		{Cmd: "GET", Args: []interface{}{"foo"}, Type: CmdDo},
		{Cmd: "SET", Args: []interface{}{"foo", "bar"}, Type: CmdDo},
		{Cmd: "EXEC", Type: CmdDo},
	}
	sendCommands := []RedisCommand{
		{Cmd: "MULTI", Type: CmdSend},
		{Cmd: "SET", Args: []interface{}{"foo", "bar"}, Type: CmdSend},
	}
	conn := FakeRedisConn{
		Cmds: append(doCommands, sendCommands...),
	}
	c.Assert(conn.GetCommands(CmdDo), gocheck.DeepEquals, doCommands)
	c.Assert(conn.GetCommands(CmdSend), gocheck.DeepEquals, sendCommands)
	c.Assert(conn.GetCommands(5), gocheck.HasLen, 0)
}

func (s *S) TestFakeConnImplementsRedisConn(c *gocheck.C) {
	var _ redis.Conn = &FakeRedisConn{}
}

func (s *S) TestFailingFakeRedisConnDo(c *gocheck.C) {
	conn := FailingFakeRedisConn{}
	result, err := conn.Do("GET", "foo")
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "I can't do that.")
}

func (s *S) TestFailingFakeRedisConnSend(c *gocheck.C) {
	conn := FailingFakeRedisConn{}
	err := conn.Send("GET", "foo")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "I can't do that.")
}

func (s *S) TestResultCommandRedisConnDo(c *gocheck.C) {
	conn := ResultCommandRedisConn{
		FakeRedisConn: &FakeRedisConn{},
		Reply:         map[string]interface{}{"GET": "something interesting"},
	}
	result, err := conn.Do("GET", "wat")
	c.Assert(err, gocheck.IsNil)
	c.Assert(result.(string), gocheck.Equals, "something interesting")
	result, err = conn.Do("MGET", "wat", "wot")
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.IsNil)
}

func (s *S) TestResultCommandRedisConnDoDefaultReply(c *gocheck.C) {
	conn := ResultCommandRedisConn{
		FakeRedisConn: &FakeRedisConn{},
		Reply:         map[string]interface{}{"GET": "something interesting"},
		DefaultReply:  "other interesting thing",
	}
	result, err := conn.Do("GET", "wat")
	c.Assert(err, gocheck.IsNil)
	c.Assert(result.(string), gocheck.Equals, "something interesting")
	result, err = conn.Do("MGET", "wat", "wot")
	c.Assert(err, gocheck.IsNil)
	c.Assert(result.(string), gocheck.Equals, "other interesting thing")
}

func (s *S) TestResultCommandRedisConnDoNoReply(c *gocheck.C) {
	conn := ResultCommandRedisConn{
		FakeRedisConn: &FakeRedisConn{},
	}
	result, err := conn.Do("GET", "wat")
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.IsNil)
}
