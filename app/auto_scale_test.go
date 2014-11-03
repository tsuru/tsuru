// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"launchpad.net/gocheck"
)

func (s *S) TestAutoScale(c *gocheck.C) {
	newApp := App{Name: "myApp", Platform: "Django"}
	err := scaleApplicationIfNeeded(&newApp)
	c.Assert(err, gocheck.IsNil)
}
