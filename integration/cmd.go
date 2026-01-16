// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	shellwords "github.com/mattn/go-shellwords"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/safe"
)

const (
	integrationEnvID = "TSURU_INTEGRATION_"
)

type safeWriter struct {
	io.Writer
	mu sync.Mutex
}

func (w *safeWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Writer.Write(data)
}

type prefixWriter struct {
	io.Writer
	prefix string
}

func (w *prefixWriter) Write(data []byte) (int, error) {
	newData := bytes.TrimSpace(data)
	newData = bytes.ReplaceAll(newData, []byte("\r"), []byte("\n"))
	newData = bytes.ReplaceAll(newData, []byte("\n"), []byte("\n"+w.prefix))
	newData = append([]byte(w.prefix), append(newData, '\n')...)
	_, err := w.Writer.Write(newData)
	return len(data), err
}

var (
	safeStdout = &safeWriter{Writer: os.Stdout}
	safeStderr = &safeWriter{Writer: os.Stderr}
)

type Command struct {
	Command  string
	Args     []string
	Input    string
	Timeout  time.Duration
	PWD      string
	NoExpand bool
}

type Result struct {
	Cmd      *exec.Cmd
	Command  *Command
	ExitCode int
	Error    error
	Timeout  bool
	Stdout   safe.Buffer
	Stderr   safe.Buffer
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
	mu    *sync.Mutex
	data  map[string][]string
	local map[string][]string
}

func NewEnvironment() *Environment {
	e := Environment{
		mu:    &sync.Mutex{},
		data:  make(map[string][]string),
		local: make(map[string][]string),
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

func (e *Environment) Clone() *Environment {
	newEnv := *e
	newEnv.local = make(map[string][]string)
	return &newEnv
}

func (e *Environment) String() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	ret := fmt.Sprintln("Env vars:")
	for k, v := range e.data {
		ret += fmt.Sprintf("  %s: %#v\n", k, v)
	}
	ret += fmt.Sprintln("Local vars:")
	for k, v := range e.local {
		ret += fmt.Sprintf("  %s: %#v\n", k, v)
	}
	return ret[:len(ret)-1]
}

func (e *Environment) flatData() map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	ret := map[string]string{}
	for _, m := range []map[string][]string{e.data, e.local} {
		for k, v := range m {
			if len(v) > 0 {
				ret[k] = v[0]
			}
		}
	}
	return ret
}

func (e *Environment) SetLocal(k string, v ...string) {
	e.local[k] = v
}

func (e *Environment) Set(k string, v ...string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.data[k] = v
}

func (e *Environment) Add(k string, v string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.data[k] = append(e.data[k], v)
}

func (e *Environment) All(k string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append(e.local[k], e.data[k]...)
}

func (e *Environment) Get(k string) string {
	if len(e.local[k]) > 0 {
		return e.local[k][0]
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.data[k]) > 0 {
		return e.data[k][0]
	}
	return ""
}

func (e *Environment) Has(k string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.local[k]) > 0 || len(e.data[k]) > 0
}

func (e *Environment) IsDry() bool {
	return e.Get("dryrun") != ""
}

func (e *Environment) VerboseLevel() int {
	v, _ := strconv.Atoi(e.Get("verbose"))
	return v
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
	var args []string
	if r.Cmd != nil {
		args = r.Cmd.Args
	}
	var input string
	if r.Command != nil {
		input = r.Command.Input
	}
	return fmt.Sprintf(`--- Command %v ---
ExitCode: %d
Error: %v
Timeout: %v
Stdin: %q
Stdout: %q
Stderr: %q
%v
----------
`, args,
		r.ExitCode,
		r.Error,
		r.Timeout,
		input,
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
		var err error
		exp, err = transformArgTemplate(r.Env, exp)
		if err != nil {
			return err
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
	defaultTimeout, _ := strconv.Atoi(os.Getenv("TSURU_INTEGRATION_COMMAND_TIMEOUT"))
	if defaultTimeout == 0 {
		defaultTimeout = 15 * 60
	}
	return &Command{Command: cmd, Args: args, Timeout: time.Duration(defaultTimeout) * time.Second}
}

func (c *Command) WithArgs(args ...string) *Command {
	c2 := *c
	c2.Args = append([]string{}, append(c.Args, args...)...)
	return &c2
}

func (c *Command) WithPWD(pwd string) *Command {
	c2 := *c
	c2.PWD = pwd
	return &c2
}

func (c *Command) WithInput(input string) *Command {
	c2 := *c
	c2.Input = input
	return &c2
}

func (c *Command) WithTimeout(timeout time.Duration) *Command {
	c2 := *c
	c2.Timeout = timeout
	return &c2
}

func (c *Command) WithNoExpand() *Command {
	c2 := *c
	c2.NoExpand = true
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
			parts := []string{transformed}
			if !c.NoExpand {
				parts, _ = shellwords.Parse(transformed)
			}
			args = append(args, parts...)
		}
		var err error
		input, err = transformArgTemplate(e, c.Input)
		if err != nil {
			res.SetError(err)
			return res
		}
	}
	execCmd := exec.Command(c.Command, args...)

	if c.PWD != "" {
		execCmd.Dir = c.PWD
	}

	execCmd.Stdin = strings.NewReader(input)
	var stdout, stderr io.Writer
	stdout = &res.Stdout
	stderr = &res.Stderr
	if e.VerboseLevel() > 1 {
		prefix := fmt.Sprintf("%+v: ", execCmd.Args)
		stdout = io.MultiWriter(stdout, &prefixWriter{safeStdout, prefix})
		stderr = io.MultiWriter(stderr, &prefixWriter{safeStderr, prefix})
	}
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr
	res.Cmd = execCmd
	done := make(chan error, 1)
	if e.IsDry() {
		close(done)
		fmt.Fprintf(safeStdout, "Would run: %+v\n", execCmd.Args)
	} else {
		if e.VerboseLevel() > 0 {
			fmt.Fprintf(safeStdout, "Running: %+v\n", execCmd.Args)
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
			fmt.Fprintf(safeStderr, "failed to kill (pid=%d): %v\n", res.Cmd.Process.Pid, killErr)
		}
		res.Timeout = true
	case err := <-done:
		res.SetError(err)
	}
	return res
}

type RetryOptions struct {
	CheckResult func(r *Result) bool
}

func (c *Command) Retry(timeout time.Duration, env *Environment, options RetryOptions) (*Result, bool) {
	res := new(Result)
	fn := func() bool {
		res = c.Run(env)
		ok, reason := checkOk(res, nil)
		if !ok {
			fmt.Printf("DEBUG: Failed to run command: %s\n", reason)
			return false
		}
		if options.CheckResult != nil {
			return options.CheckResult(res)
		}
		return true
	}
	ok := retry(timeout, fn)
	return res, ok
}

func retry(timeout time.Duration, fn func() bool) bool {
	return retryWait(timeout, 5*time.Second, fn)
}

func retryWait(timeout, wait time.Duration, fn func() bool) bool {
	timeoutTimer := time.After(timeout)
	for {
		if fn() {
			return true
		}
		select {
		case <-time.After(wait):
		case <-timeoutTimer:
			return false
		}
	}
}
