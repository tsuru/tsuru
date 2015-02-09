// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package exectest

import (
	"bytes"
	"testing"

	"github.com/tsuru/tsuru/exec"
	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) TestCommandGetName(c *check.C) {
	cmd := command{name: "docker", args: []string{"run", "some/img"}}
	c.Assert(cmd.GetName(), check.Equals, cmd.name)
}

func (s *S) TestCommandGetArgs(c *check.C) {
	cmd := command{name: "docker", args: []string{"run", "some/img"}}
	c.Assert(cmd.GetArgs(), check.DeepEquals, cmd.args)
}

func (s *S) TestCommandGetEnvs(c *check.C) {
	cmd := command{name: "docker", envs: []string{"BLA=ble"}}
	c.Assert(cmd.GetEnvs(), check.DeepEquals, cmd.envs)
}

func (s *S) TestFakeExecutorImplementsExecutor(c *check.C) {
	var _ exec.Executor = &FakeExecutor{}
}

func (s *S) TestExecute(c *check.C) {
	var e FakeExecutor
	var b bytes.Buffer
	opts := exec.ExecuteOptions{
		Cmd:    "ls",
		Args:   []string{"-lsa"},
		Stdout: &b,
		Stderr: &b,
	}
	err := e.Execute(opts)
	c.Assert(err, check.IsNil)
	opts = exec.ExecuteOptions{
		Cmd:    "ps",
		Args:   []string{"aux"},
		Stdout: &b,
		Stderr: &b,
	}
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	opts = exec.ExecuteOptions{
		Cmd:    "ps",
		Args:   []string{"-ef"},
		Stdout: &b,
		Stderr: &b,
	}
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(e.ExecutedCmd("ls", []string{"-lsa"}), check.Equals, true)
	c.Assert(e.ExecutedCmd("ps", []string{"aux"}), check.Equals, true)
	c.Assert(e.ExecutedCmd("ps", []string{"-ef"}), check.Equals, true)
}

func (s *S) TestFakeExecutorOutput(c *check.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("ble")},
		},
	}
	var b bytes.Buffer
	opts := exec.ExecuteOptions{
		Cmd:    "ls",
		Args:   []string{"-lsa"},
		Stdout: &b,
		Stderr: &b,
	}
	err := e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(e.ExecutedCmd("ls", []string{"-lsa"}), check.Equals, true)
	c.Assert(b.String(), check.Equals, "ble")
}

func (s *S) TestFakeExecutorMultipleOutputs(c *check.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("bla"), []byte("ble")},
		},
	}
	var b bytes.Buffer
	opts := exec.ExecuteOptions{
		Cmd:    "ls",
		Stdout: &b,
		Stderr: &b,
	}
	err := e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Equals, "bla")
	b.Reset()
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Equals, "ble")
	b.Reset()
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Equals, "bla")
}

func (s *S) TestFakeExecutorMultipleOutputsDifferentCalls(c *check.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*":  {[]byte("bla"), []byte("ble")},
			"-l": {[]byte("hello")},
		},
	}
	var b bytes.Buffer
	opts := exec.ExecuteOptions{
		Cmd:    "ls",
		Args:   []string{"-l"},
		Stdout: &b,
		Stderr: &b,
	}
	err := e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Equals, "hello")
	b.Reset()
	opts = exec.ExecuteOptions{
		Cmd:    "ls",
		Stdout: &b,
		Stderr: &b,
	}
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Equals, "bla")
	b.Reset()
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Equals, "ble")
}

func (s *S) TestFakeExecutorHasOutputForAnyArgsUsingWildCard(c *check.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("ble")},
		},
	}
	args := []string{"-i", "-t"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, check.Equals, true)
}

func (s *S) TestFakeExecutorHasOutputForArgsSpecifyingArgs(c *check.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*": {[]byte("ble")},
		},
	}
	args := []string{"-i", "-t"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, check.Equals, true)
}

func (s *S) TestFakeExecutorDoesNotHasOutputForNoMatchingArgs(c *check.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"-t -i": {[]byte("ble")},
		},
	}
	args := []string{"-d", "-f"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, check.Equals, false)
}

func (s *S) TestFakeExecutorWithArgsAndWildCard(c *check.C) {
	e := FakeExecutor{
		Output: map[string][][]byte{
			"*":     {[]byte("ble")},
			"-i -t": {[]byte("bla")},
		},
	}
	args := []string{"-i", "-t"}
	has, out := e.hasOutputForArgs(args)
	c.Assert(has, check.Equals, true)
	c.Assert(string(out), check.Equals, "bla")
	has, out = e.hasOutputForArgs([]string{"-i", "-x"})
	c.Assert(has, check.Equals, true)
	c.Assert(string(out), check.Equals, "ble")
}

func (s *S) TestErrorExecutorImplementsExecutor(c *check.C) {
	var _ exec.Executor = &ErrorExecutor{}
}

func (s *S) TestErrorExecute(c *check.C) {
	var e ErrorExecutor
	var b bytes.Buffer
	opts := exec.ExecuteOptions{
		Cmd:    "ls",
		Args:   []string{"-lsa"},
		Stdout: &b,
		Stderr: &b,
	}
	err := e.Execute(opts)
	c.Assert(err, check.NotNil)
	opts = exec.ExecuteOptions{
		Cmd:    "ps",
		Args:   []string{"aux"},
		Stdout: &b,
		Stderr: &b,
	}
	err = e.Execute(opts)
	c.Assert(err, check.NotNil)
	opts = exec.ExecuteOptions{
		Cmd:    "ps",
		Args:   []string{"-ef"},
		Stdout: &b,
		Stderr: &b,
	}
	err = e.Execute(opts)
	c.Assert(err, check.NotNil)
	c.Assert(e.ExecutedCmd("ls", []string{"-lsa"}), check.Equals, true)
	c.Assert(e.ExecutedCmd("ps", []string{"aux"}), check.Equals, true)
	c.Assert(e.ExecutedCmd("ps", []string{"-ef"}), check.Equals, true)
}

func (s *S) TestErrorExecutorOutput(c *check.C) {
	e := ErrorExecutor{
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"*": {[]byte("ble")},
			},
		},
	}
	var b bytes.Buffer
	opts := exec.ExecuteOptions{
		Cmd:    "ls",
		Args:   []string{"-lsa"},
		Stdout: &b,
		Stderr: &b,
	}
	err := e.Execute(opts)
	c.Assert(err, check.NotNil)
	c.Assert(e.ExecutedCmd("ls", []string{"-lsa"}), check.Equals, true)
	c.Assert(b.String(), check.Equals, "ble")
}

func (s *S) TestErrorExecutorHasOutputForAnyArgsUsingWildCard(c *check.C) {
	e := ErrorExecutor{
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"*": {[]byte("ble")},
			},
		},
	}
	args := []string{"-i", "-t"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, check.Equals, true)
}

func (s *S) TestErrorExecutorHasOutputForArgsSpecifyingArgs(c *check.C) {
	e := ErrorExecutor{
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"-i -t": {[]byte("ble")},
			},
		},
	}
	args := []string{"-i", "-t"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, check.Equals, true)
}

func (s *S) TestErrorExecutorDoesNotHaveOutputForNoMatchingArgs(c *check.C) {
	e := ErrorExecutor{
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"-i -t": {[]byte("ble")},
			},
		},
	}
	args := []string{"-d", "-f"}
	has, _ := e.hasOutputForArgs(args)
	c.Assert(has, check.Equals, false)
}

func (s *S) TestGetCommands(c *check.C) {
	var (
		e FakeExecutor
		b bytes.Buffer
	)
	opts := exec.ExecuteOptions{
		Cmd:    "sudo",
		Args:   []string{"ifconfig", "-a"},
		Stdout: &b,
		Stderr: &b,
		Envs:   []string{"BLA=bla"},
	}
	err := e.Execute(opts)
	c.Assert(err, check.IsNil)
	cmds := e.GetCommands("sudo")
	expected := []command{
		{
			name: "sudo",
			args: []string{"ifconfig", "-a"},
			envs: []string{"BLA=bla"},
		},
	}
	c.Assert(cmds, check.DeepEquals, expected)
}

func (s *S) TestRetryExecutor(c *check.C) {
	e := RetryExecutor{
		Failures: 2,
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"*": {[]byte("hello")},
			},
		},
	}
	var stdout, stderr bytes.Buffer
	opts := exec.ExecuteOptions{
		Cmd:    "ls",
		Args:   []string{"-lsa"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := e.Execute(opts)
	c.Assert(err, check.NotNil)
	c.Assert(stderr.String(), check.Equals, "hello")
	c.Assert(stdout.String(), check.Equals, "")
	stderr.Reset()
	err = e.Execute(opts)
	c.Assert(err, check.NotNil)
	c.Assert(stderr.String(), check.Equals, "hello")
	c.Assert(stdout.String(), check.Equals, "")
	stderr.Reset()
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "hello")
	c.Assert(stderr.String(), check.Equals, "")
	stdout.Reset()
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "hello")
	c.Assert(stderr.String(), check.Equals, "")
}

func (s *S) TestFailLaterExecutor(c *check.C) {
	e := FailLaterExecutor{
		Succeeds: 2,
		FakeExecutor: FakeExecutor{
			Output: map[string][][]byte{
				"*": {[]byte("hello!")},
			},
		},
	}
	var stdout, stderr bytes.Buffer
	opts := exec.ExecuteOptions{
		Cmd:    "ls",
		Args:   []string{"-lsa"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "hello!")
	c.Assert(stderr.String(), check.Equals, "")
	stdout.Reset()
	err = e.Execute(opts)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "hello!")
	c.Assert(stderr.String(), check.Equals, "")
	stdout.Reset()
	err = e.Execute(opts)
	c.Assert(err, check.NotNil)
	c.Assert(stdout.String(), check.Equals, "")
	c.Assert(stderr.String(), check.Equals, "hello!")
	stderr.Reset()
	err = e.Execute(opts)
	c.Assert(err, check.NotNil)
	c.Assert(stdout.String(), check.Equals, "")
	c.Assert(stderr.String(), check.Equals, "hello!")
}
