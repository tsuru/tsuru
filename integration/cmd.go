// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/pkg/errors"
)

const (
	integrationEnvID = "TSURU_INTEGRATION_"
)

type Command struct {
	Command string
	Args    []string
	Input   string
	Timeout time.Duration
}

type Result struct {
	Cmd      *exec.Cmd
	Command  *Command
	ExitCode int
	Error    error
	Timeout  bool
	Stdout   bytes.Buffer
	Stderr   bytes.Buffer
	Env      *Environment
}

type Expected struct {
	ExitCode int
	Timeout  bool
	Err      string
	Stderr   string
	Stdout   string
}

type Environment struct {
	data map[string][]string
}

func NewEnvironment() *Environment {
	e := Environment{
		data: make(map[string][]string),
	}
	envs := os.Environ()
	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.HasPrefix(parts[0], integrationEnvID) {
			key := strings.Replace(parts[0], integrationEnvID, "", 1)
			e.data[key] = strings.Split(parts[1], ",")
		}
	}
	return &e
}

func (e *Environment) String() string {
	ret := fmt.Sprintln("Env vars:")
	for k, v := range e.data {
		ret += fmt.Sprintf("  %s: %#v\n", k, v)
	}
	return ret[:len(ret)-1]
}

func (e *Environment) flatData() map[string]string {
	ret := map[string]string{}
	for k, v := range e.data {
		if len(v) > 0 {
			ret[k] = v[0]
		}
	}
	return ret
}

func (e *Environment) Set(k string, v ...string) {
	e.data[k] = v
}

func (e *Environment) Add(k string, v string) {
	e.data[k] = append(e.data[k], v)
}

func (e *Environment) All(k string) []string {
	return e.data[k]
}

func (e *Environment) Get(k string) string {
	if len(e.data[k]) > 0 {
		return e.data[k][0]
	}
	return ""
}

func (e *Environment) Has(k string) bool {
	return len(e.data[k]) > 0
}

func (e *Environment) IsDry() bool {
	return len(e.data["dryrun"]) > 0
}

func (e *Environment) IsVerbose() bool {
	return len(e.data["verbose"]) > 0
}

func (r *Result) SetError(err error) {
	if err == nil {
		return
	}
	r.Error = err
	if exiterr, ok := r.Error.(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			r.ExitCode = status.ExitStatus()
		}
	}
}

func (r *Result) String() string {
	return fmt.Sprintf(`--- Command %v ---
ExitCode: %d
Error: %v
Timeout: %v
Stdout: %q
Stderr: %q
%v
----------
`, r.Cmd.Args,
		r.ExitCode,
		r.Error,
		r.Timeout,
		r.Stdout.String(),
		r.Stderr.String(),
		r.Env)
}

func (r *Result) Compare(expected Expected) error {
	if expected.Timeout && !r.Timeout {
		return errors.New("expected to timeout")
	}
	if expected.ExitCode != r.ExitCode {
		return errors.Errorf("expected exitcode %d, got %d", expected.ExitCode, r.ExitCode)
	}
	matchRegex := func(exp, curr, field string) error {
		if exp == "" {
			return nil
		}
		re, err := regexp.Compile(exp)
		if err != nil {
			return err
		}
		v := strings.TrimRight(curr, "\n")
		if !re.MatchString(v) {
			return errors.Errorf("expected %s to match %q: %q", field, exp, v)
		}
		return nil
	}
	var err error
	if expected.Err != "" {
		var errorStr string
		if r.Error != nil {
			errorStr = r.Error.Error()
		}
		err = matchRegex(expected.Err, errorStr, "err")
		if err != nil {
			return err
		}
	}
	err = matchRegex(expected.Stderr, r.Stderr.String(), "stderr")
	if err != nil {
		return err
	}
	return matchRegex(expected.Stdout, r.Stdout.String(), "stdout")
}

func NewCommand(cmd string, args ...string) *Command {
	return &Command{Command: cmd, Args: args, Timeout: time.Minute}
}

func (c *Command) WithArgs(args ...string) *Command {
	c2 := *c
	c2.Args = append([]string{}, append(c.Args, args...)...)
	return &c2
}

func (c *Command) WithInput(input string) *Command {
	c2 := *c
	c2.Input = input
	return &c2
}

func transformArgTemplate(e *Environment, val string) (string, error) {
	tpl, err := template.New("tpl").Parse(val)
	if err != nil {
		return "", err
	}
	out := &bytes.Buffer{}
	err = tpl.Execute(out, e.flatData())
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

func (c *Command) Run(e *Environment) *Result {
	res := &Result{Command: c, Env: e}
	args := c.Args
	input := c.Input
	if e != nil {
		args = nil
		for i := range c.Args {
			transformed, err := transformArgTemplate(e, c.Args[i])
			if err != nil {
				res.SetError(err)
				return res
			}
			args = append(args, strings.Split(transformed, " ")...)
		}
		var err error
		input, err = transformArgTemplate(e, c.Input)
		if err != nil {
			res.SetError(err)
			return res
		}
	}
	execCmd := exec.Command(c.Command, args...)
	execCmd.Stdin = strings.NewReader(input)
	execCmd.Stdout = &res.Stdout
	execCmd.Stderr = &res.Stderr
	res.Cmd = execCmd
	done := make(chan error, 1)
	if e.IsDry() {
		close(done)
		fmt.Printf("Would run: %+v\n", execCmd.Args)
	} else {
		if e.IsVerbose() {
			fmt.Printf("Running: %+v\n", execCmd.Args)
		}
		err := res.Cmd.Start()
		if err != nil {
			res.SetError(err)
			return res
		}
		if c.Timeout == 0 {
			res.SetError(res.Cmd.Wait())
			return res
		}
		go func() {
			done <- res.Cmd.Wait()
		}()
	}
	select {
	case <-time.After(c.Timeout):
		killErr := res.Cmd.Process.Kill()
		if killErr != nil {
			fmt.Printf("failed to kill (pid=%d): %v\n", res.Cmd.Process.Pid, killErr)
		}
		res.Timeout = true
	case err := <-done:
		res.SetError(err)
	}
	return res
}
