package log

import (
	"bytes"
	. "launchpad.net/gocheck"
	stdlog "log"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestLogPanic(c *C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	Target = stdlog.New(buf, "", 0)
	defer func() {
		c.Assert(recover(), Equals, "log anything")
	}()
	Panic("log anything")
}

func (s *S) TestLogPrint(c *C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	Target = stdlog.New(buf, "", 0)
	Print("log anything")
	c.Assert(buf.String(), Equals, "log anything\n")
}

func (s *S) TestLogPrintf(c *C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	Target = stdlog.New(buf, "", 0)
	Printf("log anything %d", 1)
	c.Assert(buf.String(), Equals, "log anything 1\n")
}

func (s *S) TestLogFatalWithoutTarget(c *C) {
	Target = nil
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	Fatal("log anything")
}

func (s *S) TestLogPanicWithoutTarget(c *C) {
	Target = nil
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	Panic("log anything")
}

func (s *S) TestLogPrintWithoutTarget(c *C) {
	Target = nil
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	Print("log anything")
}

func (s *S) TestLogPrintfWithoutTarget(c *C) {
	Target = nil
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	Printf("log anything %d", 1)
}
