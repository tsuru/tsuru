// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows,!darwin

package cmd

import "github.com/tsuru/tsuru/exec"

func open(url string) error {
	var opts exec.ExecuteOptions
	opts = exec.ExecuteOptions{
		Cmd:  "xdg-open",
		Args: []string{url},
	}
	return executor().Execute(opts)
}
