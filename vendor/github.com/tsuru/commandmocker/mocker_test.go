// Copyright 2016 commandmocker authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package commandmocker

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestAddWithConcurrency(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		dir, err := Add("ssh", "msg1")
		defer Remove(dir)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		runtime.Gosched()
		cmd := exec.Command("ssh")
		output, err := cmd.Output()
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		if string(output) != "msg1" {
			t.Errorf("output should be 'msg1' but it's is %s", output)
		}
	}()
	go func() {
		defer wg.Done()
		dir, err := Add("ssh", "msg2")
		defer Remove(dir)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		runtime.Gosched()
		cmd := exec.Command("ssh")
		output, err := cmd.Output()
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		if string(output) != "msg2" {
			t.Errorf("output should be 'msg2' but it's is %s", output)
		}
	}()
	wg.Wait()
}

func TestAddMultiLineOutput(t *testing.T) {
	dir, err := Add("ssh", "success\nforever\n")
	defer Remove(dir)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	_, err = os.Stat(path.Join(dir, "ssh"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	out, err := exec.Command("ssh").Output()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	if string(out) != "success\nforever" {
		t.Errorf("should print success by running ssh, but printed %q", string(out))
	}
}

func TestAddFunctionReturnADirectoryThatIsInThePath(t *testing.T) {
	dir, err := Add("ssh", "success")
	defer Remove(dir)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	path := os.Getenv("PATH")
	if !strings.HasPrefix(path, dir) {
		t.Errorf("%s should be added to the first position in the path, but it was not.\nPATH: %s", dir, path)
	}
}

func TestAddFunctionShouldPutAnExecutableInTheReturnedDirectoryThatPrintsTheGivenOutput(t *testing.T) {
	dir, err := Add("ssh", "success")
	defer Remove(dir)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	_, err = os.Stat(path.Join(dir, "ssh"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	out, err := exec.Command("ssh").Output()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	if string(out) != "success" {
		t.Errorf("should print success by running ssh, but printed %s", out)
	}
}

func TestAddStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	dir, err := AddStderr("ssh", "success", "WARNING: do not do that in the future")
	defer Remove(dir)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	_, err = os.Stat(path.Join(dir, "ssh"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	cmd := exec.Command("ssh")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	expectedStdout := "success"
	expectedStderr := "WARNING: do not do that in the future"
	if stdout.String() != expectedStdout {
		t.Errorf("should print %q in stdout by running ssh, but printed %q", expectedStdout, stdout.String())
	}
	if stderr.String() != expectedStderr {
		t.Errorf("should print %q in stderr by running ssh, but printed %q", expectedStderr, stderr.String())
	}
}

func TestOutputFunctionReturnsOutputOfExecutedCommand(t *testing.T) {
	dir, err := Add("ssh", "$*")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	defer Remove(dir)
	_, err = exec.Command("ssh", "foo", "bar").Output()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	out := Output(dir)
	if out != "foo bar" {
		t.Errorf("Output function should return output of ssh command execution, got '%s'", out)
	}
}

func TestOutputNotRan(t *testing.T) {
	dir, err := Add("ssh", "$*")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	defer Remove(dir)
	want := ""
	got := Output(dir)
	if got != want {
		t.Errorf("Output(%q):\n\tWant %q. Got %q.", dir, want, got)
	}
}

func TestRemoveFunctionShouldRemoveTheTempDirFromPath(t *testing.T) {
	dir, _ := Add("ssh", "success")
	err := Remove(dir)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	path := os.Getenv("PATH")
	if strings.HasPrefix(path, dir) {
		t.Errorf("%s should not be in the path, but it is.\nPATH: %s", dir, path)
	}
}

func TestRemoveFunctionShouldRemoveTheTempDirFromFileSystem(t *testing.T) {
	dir, _ := Add("ssh", "success")
	Remove(dir)
	_, err := os.Stat(dir)
	if err == nil || !os.IsNotExist(err) {
		t.Errorf("Directory %s should not exist, but it does.", dir)
	}
}

func TestShouldRemoveDirectoryFromArbitraryLocationInPath(t *testing.T) {
	dir, _ := Add("ssh", "success")
	path := os.Getenv("PATH")
	os.Setenv("PATH", "/:"+path)
	err := Remove(dir)
	path = os.Getenv("PATH")
	if err != nil || strings.Contains(path, dir) {
		t.Errorf("%s should not be in $PATH, but it is.", dir)
	}
}

func TestRemoveShouldReturnErrorIfTheGivenDirectoryDoesNotStartWithSlashTmp(t *testing.T) {
	err := Remove("/some/usr/bin")
	if err == nil || err.Error() != "Remove can only remove temporary directories, tryied to remove /some/usr/bin" {
		t.Error("Should not be able to remove non-temporary directories, but it was.")
	}
}

func TestRemoveShouldReturnErrorIfTheGivenDirectoryIsNotInThePath(t *testing.T) {
	dir, _ := Add("ssh", "success")
	path := os.Getenv("PATH")
	os.Setenv("PATH", "/:"+path)
	defer Remove(dir)
	p := os.TempDir() + "/waaaaaat"
	want := fmt.Sprintf("%q is not in $PATH", p)
	err := Remove(p)
	if err == nil || err.Error() != want {
		t.Errorf("Should not be able to remove path that isn't in $PATH, but was.")
	}
}

func TestRanCheckIfTheDotRanFileExists(t *testing.T) {
	dir, err := Add("ls", "bla")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	defer Remove(dir)
	p := path.Join(dir, ".ran")
	f, err := os.Create(p)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	defer f.Close()
	table := map[string]bool{
		dir:            true,
		"/tmp/bla/bla": false,
		"/home/blabla": false,
	}
	for input, expected := range table {
		got := Ran(input)
		if got != expected {
			t.Errorf("Ran on %s?\nExpected: %q.\nGot: %q.", input, expected, got)
		}
	}
}

func TestErrorGeneratesTheFileThatReturnsExitStatusCode(t *testing.T) {
	var (
		content, p string
		b          []byte
	)
	dir, err := Error("ssh", "bla", 1)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	defer Remove(dir)
	p = path.Join(dir, "ssh")
	b, err = ioutil.ReadFile(p)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	content = string(b)
	if !strings.Contains(content, "exit 1") {
		t.Errorf(`Did not find "exit 1" in the generated file. Content: %s`, content)
	}
}

func TestErrorReturnsOutputInStderr(t *testing.T) {
	dir, err := Error("ssh", "ble", 42)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	defer Remove(dir)
	cmd := exec.Command("ssh")
	var b bytes.Buffer
	cmd.Stderr = &b
	err = cmd.Run()
	if err == nil {
		t.Error(err)
		t.FailNow()
	}
	if string(b.String()) != "ble" {
		t.Errorf("should print ble running ssh, but printed %s", b.String())
	}
}

func TestMultipleCallsAppendToOutput(t *testing.T) {
	dir, err := Add("ssh", "ble")
	if err != nil {
		t.Fatal(err)
	}
	defer Remove(dir)
	err = exec.Command("ssh").Run()
	if err != nil {
		t.Fatal(err)
	}
	err = exec.Command("ssh").Run()
	if err != nil {
		t.Fatal(err)
	}
	got := Output(dir)
	if got != "bleble" {
		t.Errorf("Output(%q): Want %q. Got %q.", dir, "bleble", got)
	}
}

func TestParameters(t *testing.T) {
	dir, err := Add("ssh", ".")
	if err != nil {
		t.Fatal(err)
	}
	defer Remove(dir)
	command := exec.Command("ssh", "-l", "me", "-o", "StrictHostKeyChecking no", "-E", "something", "-n", "localhost")
	err = command.Run()
	if err != nil {
		t.Fatal(err)
	}
	expected := command.Args[1:]
	got := Parameters(dir)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Parameters(%q):\n\tWant %#v. Got %#v.", dir, expected, got)
	}
}

func TestParametersNotRan(t *testing.T) {
	dir, err := Add("ssh", ".")
	if err != nil {
		t.Fatal(err)
	}
	defer Remove(dir)
	got := Parameters(dir)
	if got != nil {
		t.Errorf("Parameters(%q):\n\tWant %#v. Got %#v.", dir, nil, got)
	}
}

func TestEnvsReturnsProcessEnvs(t *testing.T) {
	dir, err := Add("ssh", ".")
	if err != nil {
		t.Fatal(err)
	}
	defer Remove(dir)
	command := exec.Command("ssh", ".")
	command.Env = append(os.Environ(), "MYVAR=xyz")
	err = command.Run()
	if err != nil {
		t.Fatal(err)
	}
	envs := Envs(dir)
	pattern := `(?s).*MYVAR=xyz.*`
	isMatch, _ := regexp.MatchString(pattern, envs)
	if !isMatch {
		t.Errorf("Envs(%q):\n\tWant to match %s. Got %s.", dir, pattern, envs)
	}
}
