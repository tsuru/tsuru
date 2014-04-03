// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gocheck"
	"os"
	"os/exec"
	"testing"
)

type S struct {
	recover []string
}

var manager *cmd.Manager

func (s *S) SetUpSuite(c *gocheck.C) {
	var stdout, stderr bytes.Buffer
	manager = cmd.NewManager("glb", version, header, &stdout, &stderr, os.Stdin, nil)
	targetFile := os.Getenv("HOME") + "/.tsuru_target"
	_, err := os.Stat(targetFile)
	if err == nil {
		old := targetFile + ".old"
		s.recover = []string{"mv", old, targetFile}
		exec.Command("mv", targetFile, old).Run()
	} else {
		s.recover = []string{"rm", targetFile}
	}
	f, err := os.Create(targetFile)
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("http://localhost"))
	f.Close()
}

func (s *S) TearDownSuite(c *gocheck.C) {
	exec.Command(s.recover[0], s.recover[1:]...).Run()
}

var _ = gocheck.Suite(&S{})

func Test(t *testing.T) { gocheck.TestingT(t) }
