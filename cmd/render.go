// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/crypto/ssh/terminal"
)

const (
	pattern  = "\033[%d;%d;%dm%s\033[0m"
	bgFactor = 10
)

var ignoredPatterns = []*regexp.Regexp{
	regexp.MustCompile("\033\\[\\d+;\\d+;\\d+m"),
	regexp.MustCompile("\033\\[0m"),
}

var fontColors = map[string]int{
	"black":   30,
	"red":     31,
	"green":   32,
	"yellow":  33,
	"blue":    34,
	"magenta": 35,
	"cyan":    36,
	"white":   37,
}

var fontEffects = map[string]int{
	"reset":   0,
	"bold":    1,
	"inverse": 7,
}

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

func (t *Table) addRows(rows rowSlice, sizes []int, buf *bytes.Buffer) {
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
	breakOnAny := os.Getenv("TSURU_BREAK_ANY") != ""
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
	if t.Headers == nil && len(t.rows) < 1 {
		return ""
	}
	var ttyWidth int
	terminalFd := int(os.Stdout.Fd())
	if os.Getenv("TSURU_FORCE_WRAP") != "" {
		terminalFd = int(os.Stdin.Fd())
	}
	if terminal.IsTerminal(terminalFd) {
		ttyWidth, _, _ = terminal.GetSize(terminalFd)
	}
	sizes := t.resizeLargestColumn(ttyWidth)
	buf := bytes.NewBuffer(nil)
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
	for _, p := range ignoredPatterns {
		s = p.ReplaceAllString(s, "")
	}
	return len([]rune(s))
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

func (t *Table) separator(buf *bytes.Buffer, sizes []int) {
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

func Colorfy(msg string, fontcolor string, background string, effect string) string {
	if os.Getenv("TSURU_DISABLE_COLORS") != "" {
		return msg
	}
	return fmt.Sprintf(pattern, fontEffects[effect], fontColors[fontcolor], fontColors[background]+bgFactor, msg)
}
