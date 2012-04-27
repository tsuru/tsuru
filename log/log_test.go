package log

import (
	"bytes"
	"github.com/timeredbull/tsuru/log"
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

func (s *S) TestLogPrintf(c *C) {
	buf := &bytes.Buffer{}
	defer buf.Reset()
	log.Target = stdlog.New(buf, "", 0)
	log.Printf("log anything %d", 1)
	c.Assert(buf.String(), Equals, "log anything 1\n")
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

func (s *S) TestLogPrintfWithoutTarget(c *C) {
	log.Target = nil
	defer func() {
		c.Assert(recover(), IsNil)
	}()
	log.Printf("log anything %d", 1)
}
