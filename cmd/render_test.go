// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"sort"

	"launchpad.net/gocheck"
)

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

func (s *S) TestRows(c *gocheck.C) {
	table := NewTable()
	c.Assert(table.Rows(), gocheck.Equals, 0)
	table.AddRow(Row{"One", "1"})
	c.Assert(table.Rows(), gocheck.Equals, 1)
	table.AddRow(Row{"One", "1"})
	c.Assert(table.Rows(), gocheck.Equals, 2)
	table.AddRow(Row{"One", "1"})
	table.AddRow(Row{"One", "1"})
	table.AddRow(Row{"One", "1"})
	c.Assert(table.Rows(), gocheck.Equals, 5)
}

func (s *S) TestSort(c *gocheck.C) {
	table := NewTable()
	table.AddRow(Row{"Three", "3"})
	table.AddRow(Row{"Zero", "0"})
	table.AddRow(Row{"One", "1"})
	table.AddRow(Row{"Two", "2"})
	expected := `+-------+---+
| One   | 1 |
| Three | 3 |
| Two   | 2 |
| Zero  | 0 |
+-------+---+
`
	table.Sort()
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

func (s *S) TestStringWithSeparator(c *gocheck.C) {
	table := NewTable()
	table.LineSeparator = true
	table.AddRow(Row{"One", "1"})
	table.AddRow(Row{"Two", "2"})
	table.AddRow(Row{"Three", "3"})
	expected := `+-------+---+
| One   | 1 |
+-------+---+
| Two   | 2 |
+-------+---+
| Three | 3 |
+-------+---+
`
	c.Assert(table.String(), gocheck.Equals, expected)
}

func (s *S) TestStringWithNewLine(c *gocheck.C) {
	table := NewTable()
	table.AddRow(Row{"One", "xxx\nyyy"})
	table.AddRow(Row{"Two", "2"})
	table.AddRow(Row{"Three", "3"})
	expected := `+-------+-----+
| One   | xxx |
|       | yyy |
| Two   | 2   |
| Three | 3   |
+-------+-----+
`
	c.Assert(table.String(), gocheck.Equals, expected)
}

func (s *S) TestStringWithNewLineWithSeparator(c *gocheck.C) {
	table := NewTable()
	table.LineSeparator = true
	table.AddRow(Row{"One", "xxx\nyyy\nzzzz"})
	table.AddRow(Row{"Two", "2"})
	table.AddRow(Row{"Three", "3"})
	expected := `+-------+------+
| One   | xxx  |
|       | yyy  |
|       | zzzz |
+-------+------+
| Two   | 2    |
+-------+------+
| Three | 3    |
+-------+------+
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

func (s *S) TestBytes(c *gocheck.C) {
	table := NewTable()
	table.AddRow(Row{"One", "1"})
	table.AddRow(Row{"Two", "2"})
	table.AddRow(Row{"Three", "3"})
	c.Assert(table.Bytes(), gocheck.DeepEquals, []byte(table.String()))
}

func (s *S) TestRowListAdd(c *gocheck.C) {
	l := rowSlice([]Row{{"one", "1"}})
	l.add(Row{"two", "2"})
	c.Assert(len(l), gocheck.Equals, 2)
}

func (s *S) TestRowListLen(c *gocheck.C) {
	l := rowSlice([]Row{{"one", "1"}})
	c.Assert(l.Len(), gocheck.Equals, 1)
	l.add(Row{"two", "2"})
	c.Assert(l.Len(), gocheck.Equals, 2)
}

func (s *S) TestRowListLess(c *gocheck.C) {
	l := rowSlice([]Row{{"zero", "0"}, {"one", "1"}, {"two", "2"}})
	c.Assert(l.Less(0, 1), gocheck.Equals, false)
	c.Assert(l.Less(0, 2), gocheck.Equals, false)
	c.Assert(l.Less(1, 2), gocheck.Equals, true)
	c.Assert(l.Less(1, 0), gocheck.Equals, true)
}

func (s *S) TestRowListLessDifferentCase(c *gocheck.C) {
	l := rowSlice([]Row{{"Zero", "0"}, {"one", "1"}, {"two", "2"}})
	c.Assert(l.Less(0, 1), gocheck.Equals, false)
	c.Assert(l.Less(0, 2), gocheck.Equals, false)
	c.Assert(l.Less(1, 2), gocheck.Equals, true)
	c.Assert(l.Less(1, 0), gocheck.Equals, true)
}

func (s *S) TestRowListSwap(c *gocheck.C) {
	l := rowSlice([]Row{{"zero", "0"}, {"one", "1"}, {"two", "2"}})
	l.Swap(0, 2)
	c.Assert(l.Less(0, 2), gocheck.Equals, true)
}

func (s *S) TestRowListIsSortable(c *gocheck.C) {
	var _ sort.Interface = rowSlice{}
}

func (s *S) TestColorRed(c *gocheck.C) {
	output := Colorfy("must return a red font pattern", "red", "", "")
	c.Assert(output, gocheck.Equals, "\033[0;31;10mmust return a red font pattern\033[0m")
}

func (s *S) TestColorGreen(c *gocheck.C) {
	output := Colorfy("must return a green font pattern", "green", "", "")
	c.Assert(output, gocheck.Equals, "\033[0;32;10mmust return a green font pattern\033[0m")
}

func (s *S) TestColorBoldWhite(c *gocheck.C) {
	output := Colorfy("must return a bold white font pattern", "white", "", "bold")
	c.Assert(output, gocheck.Equals, "\033[1;37;10mmust return a bold white font pattern\033[0m")
}

func (s *S) TestColorBoldYellowGreenBG(c *gocheck.C) {
	output := Colorfy("must return a bold yellow with green background", "yellow", "green", "bold")
	c.Assert(output, gocheck.Equals, "\033[1;33;42mmust return a bold yellow with green background\033[0m")
}

func (s *S) TestResizeLastColumn(c *gocheck.C) {
	t := NewTable()
	t.AddRow(Row{"1", "abcdefghijk"})
	t.AddRow(Row{"2", "1234567890"})
	sizes := t.resizeLastColumn(11)
	c.Assert(sizes, gocheck.DeepEquals, []int{1, 3})
	c.Assert(t.rows[0], gocheck.DeepEquals, Row{"1", `ab↵
cd↵
ef↵
gh↵
ij↵
k`})
	c.Assert(t.rows[1], gocheck.DeepEquals, Row{"2", `12↵
34↵
56↵
78↵
90`})
}

func (s *S) TestResizeLastColumnNoTTYSize(c *gocheck.C) {
	t := NewTable()
	t.AddRow(Row{"1", "abcdefghijk"})
	t.AddRow(Row{"2", "1234567890"})
	sizes := t.resizeLastColumn(0)
	c.Assert(sizes, gocheck.DeepEquals, []int{1, 11})
	c.Assert(t.rows[0], gocheck.DeepEquals, Row{"1", "abcdefghijk"})
	c.Assert(t.rows[1], gocheck.DeepEquals, Row{"2", "1234567890"})
}

func (s *S) TestResizeLastColumnNotEnoughSpace(c *gocheck.C) {
	t := NewTable()
	t.AddRow(Row{"1", "abcdefghijk"})
	t.AddRow(Row{"2", "1234567890"})
	sizes := t.resizeLastColumn(9)
	c.Assert(sizes, gocheck.DeepEquals, []int{1, 11})
	c.Assert(t.rows[0], gocheck.DeepEquals, Row{"1", "abcdefghijk"})
	c.Assert(t.rows[1], gocheck.DeepEquals, Row{"2", "1234567890"})
}

func (s *S) TestResizeLastColumnWithLineBreaks(c *gocheck.C) {
	t := NewTable()
	t.AddRow(Row{"1", "abcde\nfgh\ni\njklm"})
	sizes := t.resizeLastColumn(12)
	c.Assert(sizes, gocheck.DeepEquals, []int{1, 4})
	c.Assert(t.rows[0], gocheck.DeepEquals, Row{"1", `abc↵
de
fgh
i
jkl↵
m`})
}

func (s *S) TestResizeLastColumnWithColors(c *gocheck.C) {
	t := NewTable()
	color1 := Colorfy("abcdefghijk", "red", "", "")
	color2 := Colorfy("1234567890", "red", "", "")
	color3 := "123" + Colorfy("456789", "red", "", "") + "012"
	t.AddRow(Row{"1", color1})
	t.AddRow(Row{"2", color2})
	t.AddRow(Row{"3", color3})
	sizes := t.resizeLastColumn(11)
	c.Assert(sizes, gocheck.DeepEquals, []int{1, 3})
	redInit := "\033[0;31;10m"
	colorReset := "\033[0m"
	colorResetBreak := "\033[0m\n"
	c.Assert(t.rows[0], gocheck.DeepEquals, Row{"1", redInit + "ab↵" + colorResetBreak +
		redInit + "cd↵" + colorResetBreak +
		redInit + "ef↵" + colorResetBreak +
		redInit + "gh↵" + colorResetBreak +
		redInit + "ij↵" + colorResetBreak +
		redInit + "k" + colorReset})
	c.Assert(t.rows[1], gocheck.DeepEquals, Row{"2", redInit + "12↵" + colorResetBreak +
		redInit + "34↵" + colorResetBreak +
		redInit + "56↵" + colorResetBreak +
		redInit + "78↵" + colorResetBreak +
		redInit + "90" + colorReset})
	c.Assert(t.rows[2], gocheck.DeepEquals, Row{"3", "12↵\n" +
		"3" + redInit + "4↵" + colorResetBreak +
		redInit + "56↵" + colorResetBreak +
		redInit + "78↵" + colorResetBreak +
		redInit + "9" + colorReset + "0↵\n" +
		"12"})
}

func (s *S) TestResizeLastColumnUnicode(c *gocheck.C) {
	t := NewTable()
	t.AddRow(Row{"1", "åß∂¬ƒ˚©“œ¡™"})
	t.AddRow(Row{"2", "åß∂¬ƒ˚©“œ¡"})
	sizes := t.resizeLastColumn(11)
	c.Assert(sizes, gocheck.DeepEquals, []int{1, 3})
	c.Assert(t.rows[0], gocheck.DeepEquals, Row{"1", `åß↵
∂¬↵
ƒ˚↵
©“↵
œ¡↵
™`})
	c.Assert(t.rows[1], gocheck.DeepEquals, Row{"2", `åß↵
∂¬↵
ƒ˚↵
©“↵
œ¡`})
}

func (s *S) TestColoredString(c *gocheck.C) {
	table := NewTable()
	two := Colorfy("str", "red", "", "")
	two = two + " - " + two
	table.AddRow(Row{"Some large string", "1"})
	table.AddRow(Row{two, "2"})
	table.AddRow(Row{"Three", "3"})
	expected := `+-------------------+---+
| Some large string | 1 |
| ` + two + `         | 2 |
| Three             | 3 |
+-------------------+---+
`
	c.Assert(table.String(), gocheck.Equals, expected)
}
