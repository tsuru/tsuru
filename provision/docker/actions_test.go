// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"launchpad.net/gocheck"
)

func (s *S) TestCreateContainerName(c *gocheck.C) {
	c.Assert(createContainer.Name, gocheck.Equals, "create-container")
}
