// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tablecli

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/ssh/terminal"
)

var TableConfig = struct {
	BreakOnAny   bool
	ForceWrap    bool
	UseTabWriter bool
	MaxTTYWidth  int
}{
	BreakOnAny:   false,
	ForceWrap:    false,
	UseTabWriter: false,
	MaxTTYWidth:  0,
}

var ignoredPatterns = []*regexp.Regexp{
	regexp.MustCompile("\033\\[\\d+;\\d+;\\d+m"),
	regexp.MustCompile("\033\\[0m"),
}

var ignoredPattern = regexp.MustCompile("\033\\[\\d+;\\d+;\\d+m|\033\\[0m")

type Table struct {
	Headers       Row
	LineSeparator bool
	rows          rowSlice
}

type Row []string

func NewTable() *Table {
	return &Table{}
}

// Sort sorts the rows in the table using the first column as key.
func (t *Table) Sort() {
	sort.Sort(t.rows)
}

func (t *Table) Reverse() {
	sort.Sort(sort.Reverse(t.rows))
}

func (t *Table) SortByColumn(columns ...int) {
	sort.Sort(rowSliceByColumn{rowSlice: t.rows, columns: columns})
}

func (t *Table) addRows(rows rowSlice, sizes []int, buf *strings.Builder) {
	for _, row := range rows {
		extraRows := rowSlice{}
		for column, field := range row {
			parts := strings.Split(field, "\n")
			field = parts[0]
			for i := range parts[1:] {
				var newRow Row
				if len(extraRows) > i {
					newRow = extraRows[i]
				} else {
					newRow = Row(make([]string, len(row)))
					extraRows.add(newRow)
				}
				newRow[column] = parts[i+1]
			}
			buf.WriteString("| ")
			buf.WriteString(field)
			buf.Write(bytes.Repeat([]byte(" "), sizes[column]+1-runeLen(field)))
		}
		buf.WriteString("|\n")
		t.addRows(extraRows, sizes, buf)
		ptr1 := reflect.ValueOf(rows).Pointer()
		ptr2 := reflect.ValueOf(t.rows).Pointer()
		if ptr1 == ptr2 && t.LineSeparator {
			t.separator(buf, sizes)
		}
	}
}

func splitJoinEvery(str string, n int) string {
	breakOnAny := TableConfig.BreakOnAny
	breakChars := []rune{' ', '.', ':', '='}
	n -= 1
	str = strings.TrimRightFunc(str, unicode.IsSpace)
	lines := strings.Split(str, "\n")
	var parts [][]rune
	for _, line := range lines {
		line = strings.TrimRightFunc(line, unicode.IsSpace)
		lineRunes := []rune(line)
		strLen := len(lineRunes)
		var start, end int
		for ; start < strLen; start = end {
			end = start + n
			for _, p := range ignoredPatterns {
				pos := p.FindStringIndex(string(lineRunes[start:]))
				if pos != nil && pos[0] < (end-start+1) {
					end += pos[1] - pos[0]
				}
			}
			if end > strLen {
				end = strLen
			}
			oldEnd := end
			skipSpace := false
			breakableChar := false
			if !breakOnAny && end < strLen {
				for ; end > start; end-- {
					for _, chr := range breakChars {
						if chr == lineRunes[end] {
							breakableChar = true
							if chr == ' ' {
								skipSpace = true
							} else if end < oldEnd {
								end++
							}
							break
						}
					}
					if breakableChar {
						break
					}
				}
				if !breakableChar {
					end = oldEnd
				}
			}
			part := make([]rune, end-start)
			copy(part, lineRunes[start:end])
			if breakableChar {
				padding := n - (end - start)
				if padding > 0 {
					part = append(part, []rune(strings.Repeat(" ", padding))...)
				}
				if skipSpace {
					end++
				}
			}
			if end < strLen {
				part = append(part, rune('↵'))
			}
			parts = append(parts, part)
		}
	}
	return redistributeColors(parts)
}

func redistributeColors(parts [][]rune) string {
	var result string
	var lastStartColor, nextResetStr string
	for _, part := range parts {
		nextStartColor := lastStartColor
		partStr := string(part)
		startPos := ignoredPatterns[0].FindStringIndex(partStr)
		resetPos := ignoredPatterns[1].FindStringIndex(partStr)
		if startPos != nil {
			if resetPos == nil || resetPos[0] < startPos[0] {
				nextStartColor = partStr[startPos[0]:startPos[1]]
				nextResetStr = "\033[0m"
			} else {
				nextStartColor = ""
			}
		}
		if resetPos != nil {
			if startPos == nil || resetPos[0] > startPos[0] {
				nextResetStr = ""
				nextStartColor = ""
			}
		}
		partStr = lastStartColor + partStr + nextResetStr
		lastStartColor = nextStartColor
		result += partStr + "\n"
	}
	return strings.TrimRight(result, "\n")
}

func (t *Table) resizeLargestColumn(ttyWidth int) []int {
	sizes := t.columnsSize()
	if ttyWidth == 0 {
		return sizes
	}
	fullSize := 0
	maxIdx, maxVal := -1, -1
	for i, sz := range sizes {
		fullSize += sz
		if sz > maxVal {
			maxVal = sz
			maxIdx = i
		}
	}
	fullSize += len(sizes)*3 + 1
	available := ttyWidth - (fullSize - maxVal)
	if fullSize > ttyWidth && available > 1 {
		for _, row := range t.rows {
			row[maxIdx] = splitJoinEvery(row[maxIdx], available)
		}
	}
	return t.columnsSize()
}

func (t *Table) String() string {
	if TableConfig.UseTabWriter {
		buf := bytes.NewBuffer(nil)
		w := tabwriter.NewWriter(buf, 10, 4, 3, ' ', 0)
		if len(t.Headers) > 0 {
			fmt.Fprintln(w, strings.Join(t.Headers, "\t"))
		}
		for _, row := range t.rows {
			newRow := make([]string, len(row))
			for i, column := range row {
				newRow[i] = strings.Replace(column, "\n", "|", -1)
			}
			fmt.Fprintln(w, strings.Join(newRow, "\t"))
		}
		w.Flush()
		return buf.String()
	}
	if t.Headers == nil && len(t.rows) < 1 {
		return ""
	}
	var ttyWidth int
	terminalFd := int(os.Stdout.Fd())
	if TableConfig.ForceWrap {
		terminalFd = int(os.Stdin.Fd())
	}
	if terminal.IsTerminal(terminalFd) {
		ttyWidth, _, _ = terminal.GetSize(terminalFd)
	}
	if TableConfig.MaxTTYWidth > 0 && (ttyWidth == 0 || ttyWidth > TableConfig.MaxTTYWidth) {
		ttyWidth = TableConfig.MaxTTYWidth
	}
	sizes := t.resizeLargestColumn(ttyWidth)
	buf := &strings.Builder{}
	t.separator(buf, sizes)
	if t.Headers != nil {
		for column, header := range t.Headers {
			buf.WriteString("| ")
			buf.WriteString(header)
			buf.Write(bytes.Repeat([]byte(" "), sizes[column]+1-len(header)))
		}
		buf.WriteString("|\n")
		t.separator(buf, sizes)
	}
	t.addRows(t.rows, sizes, buf)
	if !t.LineSeparator {
		t.separator(buf, sizes)
	}
	return buf.String()
}

func (t *Table) Bytes() []byte {
	return []byte(t.String())
}

func (t *Table) AddRow(row Row) {
	t.rows.add(row)
}

func (t *Table) Rows() int {
	return t.rows.Len()
}

func runeLen(s string) int {
	if strings.IndexByte(s, '\033') == -1 {
		return utf8.RuneCountInString(s)
	}
	positions := ignoredPattern.FindAllStringIndex(s, -1)
	if len(positions) == 0 {
		return utf8.RuneCountInString(s)
	}
	var count int
	start := 0
	for _, pos := range positions {
		count += utf8.RuneCountInString(s[start:pos[0]])
		start = pos[1]
	}
	return count
}

func (t *Table) columnsSize() []int {
	var columns int
	if t.Headers != nil {
		columns = len(t.Headers)
	} else {
		columns = len(t.rows[0])
	}
	sizes := make([]int, columns)
	for _, row := range t.rows {
		for i := 0; i < columns; i++ {
			rowParts := strings.Split(row[i], "\n")
			for _, part := range rowParts {
				partLen := runeLen(part)
				if partLen > sizes[i] {
					sizes[i] = partLen
				}
			}
		}
	}
	if t.Headers != nil {
		for i, header := range t.Headers {
			headerLen := runeLen(header)
			if headerLen > sizes[i] {
				sizes[i] = headerLen
			}
		}
	}
	return sizes
}

func (t *Table) separator(buf *strings.Builder, sizes []int) {
	for _, sz := range sizes {
		buf.WriteString("+")
		buf.Write(bytes.Repeat([]byte("-"), sz+2))
	}
	buf.WriteString("+\n")
}

type rowSlice []Row

type rowSliceByColumn struct {
	rowSlice
	columns []int
}

func (l rowSliceByColumn) Len() int {
	return len(l.rowSlice)
}

func (l rowSliceByColumn) Less(i, j int) bool {
	for _, c := range l.columns {
		v1, v2 := strings.ToLower(l.rowSlice[i][c]), strings.ToLower(l.rowSlice[j][c])
		if v1 == v2 {
			continue
		}
		return v1 < v2
	}
	return false
}

func (l rowSliceByColumn) Swap(i, j int) {
	l.rowSlice[i], l.rowSlice[j] = l.rowSlice[j], l.rowSlice[i]
}

func (l *rowSlice) add(r Row) {
	*l = append(*l, r)
}

func (l rowSlice) Len() int {
	return len(l)
}

func (l rowSlice) Less(i, j int) bool {
	return strings.ToLower(l[i][0]) < strings.ToLower(l[j][0])
}

func (l rowSlice) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
