// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

var registry []Command

func RegisterExtraCmd(cmd Command) {
	registry = append(registry, cmd)
}

func ExtraCmds() []Command {
	return registry
}
