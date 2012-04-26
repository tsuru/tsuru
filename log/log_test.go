package log_test

import (
	"bytes"
	. "launchpad.net/gocheck"
	"github.com/timeredbull/tsuru/log"
	stdlog "log"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {}

var _ = Suite(&S{})

func (s *S) TestLogPanic(c *C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	log.Target = stdlog.New(buf, "", 0)
	defer func() {
		c.Assert(recover(), Equals, "log anything")
	}()
	log.Panic("log anything")
}

func (s *S) TestLogPrint(c *C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	log.Target = stdlog.New(buf, "", 0)
	log.Print("log anything")
	c.Assert(buf.String(), Equals, "log anything\n")
}

func (s *S) TestLogFatalWithoutTarget(c *C) {
	log.Target = nil
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	log.Fatal("log anything")
}

func (s *S) TestLogPanicWithoutTarget(c *C) {
	log.Target = nil
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	log.Panic("log anything")
}

func (s *S) TestLogPrintWithoutTarget(c *C) {
	log.Target = nil
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	log.Print("log anything")
}
