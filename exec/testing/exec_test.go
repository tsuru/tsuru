// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bytes"
	"github.com/tsuru/tsuru/exec"
	"launchpad.net/gocheck"
	"testing"
)

type S struct{}

var _ = gocheck.Suite(&S{})

func Test(t *testing.T) { gocheck.TestingT(t) }

func (s *S) TestCommandGetName(c *gocheck.C) {
	cmd := command{name: "docker", args: []string{"run", "some/img"}}
	c.Assert(cmd.GetName(), gocheck.Equals, cmd.name)
}

func (s *S) TestCommandGetArgs(c *gocheck.C) {
	cmd := command{name: "docker", args: []string{"run", "some/img"}}
	c.Assert(cmd.GetArgs(), gocheck.DeepEquals, cmd.args)
}

func (s *S) TestFakeExecutorImplementsExecutor(c *gocheck.C) {
	var _ exec.Executor = &FakeExecutor{}
}

func (s *S) TestExecute(c *gocheck.C) {
	var e FakeExecutor
	var b bytes.Buffer
	cmd := "ls"
	args := []string{"-lsa"}
	err := e.Execute(cmd, args, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	cmd = "ps"
	args = []string{"aux"}
	err = e.Execute(cmd, args, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	cmd = "ps"
	args = []string{"-ef"}
	err = e.Execute(cmd, args, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(e.ExecutedCmd("ls", []string{"-lsa"}), gocheck.Equals, true)
	c.Assert(e.ExecutedCmd("ps", []string{"aux"}), gocheck.Equals, true)
	c.Assert(e.ExecutedCmd("ps", []string{"-ef"}), gocheck.Equals, true)
}

func (s *S) TestFakeExecutorOutput(c *gocheck.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("ble")},
		},
	}
	var b bytes.Buffer
	cmd := "ls"
	args := []string{"-lsa"}
	err := e.Execute(cmd, args, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(e.ExecutedCmd("ls", []string{"-lsa"}), gocheck.Equals, true)
	c.Assert(b.String(), gocheck.Equals, "ble")
}

func (s *S) TestFakeExecutorMultipleOutputs(c *gocheck.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("bla"), []byte("ble")},
		},
	}
	var b bytes.Buffer
	err := e.Execute("ls", nil, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(b.String(), gocheck.Equals, "bla")
	b.Reset()
	err = e.Execute("ls", nil, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(b.String(), gocheck.Equals, "ble")
	b.Reset()
	err = e.Execute("ls", nil, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(b.String(), gocheck.Equals, "bla")
}

func (s *S) TestFakeExecutorMultipleOutputsDifferentCalls(c *gocheck.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*":  {[]byte("bla"), []byte("ble")},
			"-l": {[]byte("hello")},
		},
	}
	var b bytes.Buffer
	err := e.Execute("ls", []string{"-l"}, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(b.String(), gocheck.Equals, "hello")
	b.Reset()
	err = e.Execute("ls", nil, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(b.String(), gocheck.Equals, "bla")
	b.Reset()
	err = e.Execute("ls", nil, nil, &b, &b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(b.String(), gocheck.Equals, "ble")
}

func (s *S) TestFakeExecutorHasOutputForAnyArgsUsingWildCard(c *gocheck.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("ble")},
		},
	}
	args := []string{"-i", "-t"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, gocheck.Equals, true)
}

func (s *S) TestFakeExecutorHasOutputForArgsSpecifyingArgs(c *gocheck.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("ble")},
		},
	}
	args := []string{"-i", "-t"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, gocheck.Equals, true)
}

func (s *S) TestFakeExecutorDoesNotHasOutputForNoMatchingArgs(c *gocheck.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"-t -i": {[]byte("ble")},
		},
	}
	args := []string{"-d", "-f"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, gocheck.Equals, false)
}

func (s *S) TestFakeExecutorWithArgsAndWildCard(c *gocheck.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*":     {[]byte("ble")},
			"-i -t": {[]byte("bla")},
		},
	}
	args := []string{"-i", "-t"}
	has, out := e.hasOutputForArgs(args)
	c.Assert(has, gocheck.Equals, true)
	c.Assert(string(out), gocheck.Equals, "bla")
	has, out = e.hasOutputForArgs([]string{"-i", "-x"})
	c.Assert(has, gocheck.Equals, true)
	c.Assert(string(out), gocheck.Equals, "ble")
}

func (s *S) TestErrorExecutorImplementsExecutor(c *gocheck.C) {
	var _ exec.Executor = &ErrorExecutor{}
}

func (s *S) TestErrorExecute(c *gocheck.C) {
	var e ErrorExecutor
	var b bytes.Buffer
	cmd := "ls"
	args := []string{"-lsa"}
	err := e.Execute(cmd, args, nil, &b, &b)
	c.Assert(err, gocheck.NotNil)
	cmd = "ps"
	args = []string{"aux"}
	err = e.Execute(cmd, args, nil, &b, &b)
	c.Assert(err, gocheck.NotNil)
	cmd = "ps"
	args = []string{"-ef"}
	err = e.Execute(cmd, args, nil, &b, &b)
	c.Assert(err, gocheck.NotNil)
	c.Assert(e.ExecutedCmd("ls", []string{"-lsa"}), gocheck.Equals, true)
	c.Assert(e.ExecutedCmd("ps", []string{"aux"}), gocheck.Equals, true)
	c.Assert(e.ExecutedCmd("ps", []string{"-ef"}), gocheck.Equals, true)
}

func (s *S) TestErrorExecutorOutput(c *gocheck.C) {
	e := ErrorExecutor{
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"*": {[]byte("ble")},
			},
		},
	}
	var b bytes.Buffer
	cmd := "ls"
	args := []string{"-lsa"}
	err := e.Execute(cmd, args, nil, &b, &b)
	c.Assert(err, gocheck.NotNil)
	c.Assert(e.ExecutedCmd("ls", []string{"-lsa"}), gocheck.Equals, true)
	c.Assert(b.String(), gocheck.Equals, "ble")
}

func (s *S) TestErrorExecutorHasOutputForAnyArgsUsingWildCard(c *gocheck.C) {
	e := ErrorExecutor{
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"*": {[]byte("ble")},
			},
		},
	}
	args := []string{"-i", "-t"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, gocheck.Equals, true)
}

func (s *S) TestErrorExecutorHasOutputForArgsSpecifyingArgs(c *gocheck.C) {
	e := ErrorExecutor{
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"-i -t": {[]byte("ble")},
			},
		},
	}
	args := []string{"-i", "-t"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, gocheck.Equals, true)
}

func (s *S) TestErrorExecutorDoesNotHaveOutputForNoMatchingArgs(c *gocheck.C) {
	e := ErrorExecutor{
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"-i -t": {[]byte("ble")},
			},
		},
	}
	args := []string{"-d", "-f"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, gocheck.Equals, false)
}

func (s *S) TestGetCommands(c *gocheck.C) {
	e := FakeExecutor{}
	b := &bytes.Buffer{}
	err := e.Execute("sudo", []string{"ifconfig", "-a"}, nil, b, b)
	c.Assert(err, gocheck.IsNil)
	cmds := e.GetCommands("sudo")
	expected := []command{{name: "sudo", args: []string{"ifconfig", "-a"}}}
	c.Assert(cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestRetryExecutor(c *gocheck.C) {
	e := RetryExecutor{
		Failures: 2,
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"*": {[]byte("hello")},
			},
		},
	}
	var stdout, stderr bytes.Buffer
	args := []string{"-la"}
	err := e.Execute("ls", args, nil, &stdout, &stderr)
	c.Assert(err, gocheck.NotNil)
	c.Assert(stderr.String(), gocheck.Equals, "hello")
	c.Assert(stdout.String(), gocheck.Equals, "")
	stderr.Reset()
	err = e.Execute("ls", args, nil, &stdout, &stderr)
	c.Assert(err, gocheck.NotNil)
	c.Assert(stderr.String(), gocheck.Equals, "hello")
	c.Assert(stdout.String(), gocheck.Equals, "")
	stderr.Reset()
	err = e.Execute("ls", args, nil, &stdout, &stderr)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "hello")
	c.Assert(stderr.String(), gocheck.Equals, "")
	stdout.Reset()
	err = e.Execute("ls", args, nil, &stdout, &stderr)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "hello")
	c.Assert(stderr.String(), gocheck.Equals, "")
}

func (s *S) TestFailLaterExecutor(c *gocheck.C) {
	e := FailLaterExecutor{
		Succeeds: 2,
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"*": {[]byte("hello!")},
			},
		},
	}
	var stdout, stderr bytes.Buffer
	args := []string{"-la"}
	err := e.Execute("ls", args, nil, &stdout, &stderr)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "hello!")
	c.Assert(stderr.String(), gocheck.Equals, "")
	stdout.Reset()
	err = e.Execute("ls", args, nil, &stdout, &stderr)
	c.Assert(err, gocheck.IsNil)
	c.Assert(stdout.String(), gocheck.Equals, "hello!")
	c.Assert(stderr.String(), gocheck.Equals, "")
	stdout.Reset()
	err = e.Execute("ls", args, nil, &stdout, &stderr)
	c.Assert(err, gocheck.NotNil)
	c.Assert(stdout.String(), gocheck.Equals, "")
	c.Assert(stderr.String(), gocheck.Equals, "hello!")
	stderr.Reset()
	err = e.Execute("ls", args, nil, &stdout, &stderr)
	c.Assert(err, gocheck.NotNil)
	c.Assert(stdout.String(), gocheck.Equals, "")
	c.Assert(stderr.String(), gocheck.Equals, "hello!")
}
