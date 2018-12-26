// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import check "gopkg.in/check.v1"

func (s *S) TestColorRed(c *check.C) {
	output := Colorfy("must return a red font pattern", "red", "", "")
	c.Assert(output, check.Equals, "\033[0;31;10mmust return a red font pattern\033[0m")
}

func (s *S) TestColorGreen(c *check.C) {
	output := Colorfy("must return a green font pattern", "green", "", "")
	c.Assert(output, check.Equals, "\033[0;32;10mmust return a green font pattern\033[0m")
}

func (s *S) TestColorBoldWhite(c *check.C) {
	output := Colorfy("must return a bold white font pattern", "white", "", "bold")
	c.Assert(output, check.Equals, "\033[1;37;10mmust return a bold white font pattern\033[0m")
}

func (s *S) TestColorBoldYellowGreenBG(c *check.C) {
	output := Colorfy("must return a bold yellow with green background", "yellow", "green", "bold")
	c.Assert(output, check.Equals, "\033[1;33;42mmust return a bold yellow with green background\033[0m")
}
