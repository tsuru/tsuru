// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows,!darwin

package cmd

import (
	"fmt"
	"strings"

	"github.com/tsuru/tsuru/exec"
	"golang.org/x/sys/unix"
)

func isWSL() bool {
	var u unix.Utsname
	err := unix.Uname(&u)
	if err != nil {
		fmt.Println(err)
		return false
	}
	release := strings.ToLower(string(u.Release[:]))
	return strings.Contains(release, "microsoft")
}

func open(url string) error {

	cmd := "xdg-open"
	args := []string{url}

	if isWSL() {
		cmd = "powershell.exe"
		args = []string{"-c", "start", url}
	}

	opts := exec.ExecuteOptions{
		Cmd:  cmd,
		Args: args,
	}
	return executor().Execute(opts)
}
