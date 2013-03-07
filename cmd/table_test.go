// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import "launchpad.net/gocheck"

func (s *S) TestAddOneRow(c *gocheck.C) {
	table := NewTable()
	table.AddRow(Row{"Three", "foo"})
	c.Assert(table.String(), gocheck.Equals, "+-------+-----+\n| Three | foo |\n+-------+-----+\n")
}

func (s *S) TestAddRows(c *gocheck.C) {
	table := NewTable()
	table.AddRow(Row{"One", "1"})
	table.AddRow(Row{"Two", "2"})
	table.AddRow(Row{"Three", "3"})
	expected := `+-------+---+
| One   | 1 |
| Two   | 2 |
| Three | 3 |
+-------+---+
`
	c.Assert(table.String(), gocheck.Equals, expected)
}

func (s *S) TestColumnsSize(c *gocheck.C) {
	table := NewTable()
	table.AddRow(Row{"One", "1"})
	table.AddRow(Row{"Two", "2"})
	table.AddRow(Row{"Three", "3"})
	c.Assert(table.columnsSize(), gocheck.DeepEquals, []int{5, 1})
}

func (s *S) TestSeparator(c *gocheck.C) {
	table := NewTable()
	table.AddRow(Row{"One", "1"})
	table.AddRow(Row{"Two", "2"})
	table.AddRow(Row{"Three", "3"})
	expected := "+-------+---+\n"
	c.Assert(table.separator(), gocheck.Equals, expected)
}

func (s *S) TestHeadings(c *gocheck.C) {
	table := NewTable()
	table.Headers = Row{"Word", "Number"}
	table.AddRow(Row{"One", "1"})
	table.AddRow(Row{"Two", "2"})
	table.AddRow(Row{"Three", "3"})
	expected := `+-------+--------+
| Word  | Number |
+-------+--------+
| One   | 1      |
| Two   | 2      |
| Three | 3      |
+-------+--------+
`
	c.Assert(table.String(), gocheck.Equals, expected)
}

func (s *S) TestString(c *gocheck.C) {
	table := NewTable()
	table.AddRow(Row{"One", "1"})
	table.AddRow(Row{"Two", "2"})
	table.AddRow(Row{"Three", "3"})
	expected := `+-------+---+
| One   | 1 |
| Two   | 2 |
| Three | 3 |
+-------+---+
`
	c.Assert(table.String(), gocheck.Equals, expected)
}

func (s *S) TestRenderNoRows(c *gocheck.C) {
	table := NewTable()
	table.Headers = Row{"Word", "Number"}
	expected := `+------+--------+
| Word | Number |
+------+--------+
+------+--------+
`
	c.Assert(table.String(), gocheck.Equals, expected)
}

func (s *S) TestRenderEmpty(c *gocheck.C) {
	table := NewTable()
	c.Assert(table.String(), gocheck.Equals, "")
}
