// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmdtest

import (
	"fmt"
	"os"
	"os/exec"

	"launchpad.net/gocheck"
)

func SetTargetFile(c *gocheck.C, target []byte) []string {
	return writeHomeFile(c, ".tsuru_target", target)
}

func SetTokenFile(c *gocheck.C, token []byte) []string {
	return writeHomeFile(c, ".tsuru_token", token)
}

func RollbackFile(rollbackCmds []string) {
	exec.Command(rollbackCmds[0], rollbackCmds[1:]...).Run()
}

func writeHomeFile(c *gocheck.C, filename string, content []byte) []string {
	file := os.Getenv("HOME") + "/" + filename
	_, err := os.Stat(file)
	var recover []string
	if err == nil {
		var old string
		for i := 0; err == nil; i++ {
			old = file + fmt.Sprintf(".old-%d", i)
			_, err = os.Stat(old)
		}
		recover = []string{"mv", old, file}
		exec.Command("mv", file, old).Run()
	} else {
		recover = []string{"rm", file}
	}
	f, err := os.Create(file)
	c.Assert(err, gocheck.IsNil)
	f.Write(content)
	f.Close()
	return recover
}
