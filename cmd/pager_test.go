// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"io"
	"os"

	"gopkg.in/check.v1"
)

func (s *S) TestNewPagerWriter(c *check.C) {
	var writer io.Writer = &bytes.Buffer{}
	pager := newPagerWriter(writer)
	c.Assert(pager, check.Equals, writer)
	pager = newPagerWriter(os.Stdout)
	c.Assert(pager, check.Not(check.Equals), writer)
	p, ok := pager.(*pagerWriter)
	c.Assert(ok, check.Equals, true)
	c.Assert(p.baseWriter, check.Equals, os.Stdout)
	c.Assert(p.buf.String(), check.Equals, "")
	c.Assert(p.height > 0, check.Equals, true)
	c.Assert(p.pager, check.Equals, "less -R")
}

func (s *S) TestNewPagerWriterWithTSURU_PAGER(c *check.C) {
	defer os.Unsetenv("TSURU_PAGER")
	os.Setenv("TSURU_PAGER", "")
	w := newPagerWriter(os.Stdout)
	c.Assert(w, check.Equals, os.Stdout)

	os.Setenv("TSURU_PAGER", "lolcat")
	pager, ok := newPagerWriter(os.Stdout).(*pagerWriter)
	c.Assert(ok, check.Equals, true)
	c.Assert(pager.pager, check.Equals, "lolcat")
}
