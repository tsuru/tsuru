// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmds

import (
	"github.com/tsuru/tsuru/cmd"
)

func init() {
	cmd.RegisterExtraCmd(&moveContainerCmd{})
	cmd.RegisterExtraCmd(&moveContainersCmd{})
}
