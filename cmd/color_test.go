// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestRed(c *C) {
	output := Colorfy("must return a red font pattern","red","","")
	c.Assert(output, Equals, "\033[0;31;10mmust return a red font pattern\033[0m")
}

func (s *S) TestGreen(c *C) {
	output := Colorfy("must return a green font pattern","green","","")
	c.Assert(output, Equals, "\033[0;32;10mmust return a green font pattern\033[0m")
}

func (s *S) TestBoldWhite(c *C) {
	output := Colorfy("must return a bold white font pattern","white", "", "bold")
	c.Assert(output, Equals, "\033[1;37;10mmust return a bold white font pattern\033[0m")
}

func (s *S) TestBoldYellowGreenBG(c *C) {
	output := Colorfy("must return a bold yellow with green background", "yellow", "green", "bold")
	c.Assert(output, Equals, "\033[1;33;42mmust return a bold yellow with green background\033[0m")
}
