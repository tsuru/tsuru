// Copyright 2014 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"fmt"
	"os/exec"
	"path"

	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/fs"
)

var bare string

func bareLocation() string {
	if bare != "" {
		return bare
	}
	var err error
	bare, err = config.GetString("git:bare:location")
	if err != nil {
		panic("You should configure a git:bare:location for gandalf.")
	}
	return bare
}

func barePath(name string) string {
	return path.Join(bareLocation(), name+".git")
}

func newBare(name string) error {
	args := []string{"init", barePath(name), "--bare"}
	if bareTempl, err := config.GetString("git:bare:template"); err == nil {
		args = append(args, "--template="+bareTempl)
	}
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Could not create git bare repository: %s. %s", err, string(out))
	}
	return nil
}

func removeBare(name string) error {
	err := fs.Filesystem().RemoveAll(barePath(name))
	if err != nil {
		return fmt.Errorf("Could not remove git bare repository: %s", err)
	}
	return nil
}
