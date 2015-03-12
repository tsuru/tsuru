// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"strings"

	"github.com/tsuru/tsuru/exec"
)

func open(url string) error {
	var opts exec.ExecuteOptions
	url = strings.Replace(url, "&", "^&", -1)
	opts = exec.ExecuteOptions{
		Cmd:  "cmd",
		Args: []string{"/c", "start", "", url},
	}
	return executor().Execute(opts)
}
