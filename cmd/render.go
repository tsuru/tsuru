// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const (
	pattern  = "\033[%d;%d;%dm%s\033[0m"
	bgFactor = 10
)

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

func (t *Table) SortByColumn(column int) {
	sort.Sort(rowSliceByColumn{rowSlice: t.rows, column: column})
}

func (t *Table) addRows(rows rowSlice, sizes []int, result string) string {
	for _, row := range rows {
		extraRows := rowSlice{}
		for column, field := range row {
			parts := strings.Split(field, "\n")
			field = parts[0]
			for i := range parts[1:] {
				newRow := Row(make([]string, len(row)))
				newRow[column] = parts[i+1]
				extraRows.add(newRow)
			}
			result += "| " + field
			result += strings.Repeat(" ", sizes[column]+1-len(field))
		}
		result += "|\n"
		result = t.addRows(extraRows, sizes, result)
		ptr1 := reflect.ValueOf(rows).Pointer()
		ptr2 := reflect.ValueOf(t.rows).Pointer()
		if ptr1 == ptr2 && t.LineSeparator {
			result += t.separator()
		}
	}
	return result
}

func (t *Table) String() string {
	if t.Headers == nil && len(t.rows) < 1 {
		return ""
	}
	sizes := t.columnsSize()
	result := t.separator()
	if t.Headers != nil {
		for column, header := range t.Headers {
			result += "| " + header
			result += strings.Repeat(" ", sizes[column]+1-len(header))
		}
		result += "|\n"
		result += t.separator()
	}
	result = t.addRows(t.rows, sizes, result)
	if !t.LineSeparator {
		result += t.separator()
	}
	return result
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
				if len(part) > sizes[i] {
					sizes[i] = len(part)
				}
			}
		}
	}
	if t.Headers != nil {
		for i, header := range t.Headers {
			if len(header) > sizes[i] {
				sizes[i] = len(header)
			}
		}
	}
	return sizes
}

func (t *Table) separator() string {
	result := ""
	sizes := t.columnsSize()
	for i := 0; i < len(sizes); i++ {
		result = result + "+" + strings.Repeat("-", sizes[i]+2)
	}
	result = result + "+" + "\n"
	return result
}

type rowSlice []Row

type rowSliceByColumn struct {
	rowSlice
	column int
}

func (l rowSliceByColumn) Len() int {
	return len(l.rowSlice)
}

func (l rowSliceByColumn) Less(i, j int) bool {
	return strings.ToLower(l.rowSlice[i][l.column]) < strings.ToLower(l.rowSlice[j][l.column])
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
	return fmt.Sprintf(pattern, fontEffects[effect], fontColors[fontcolor], fontColors[background]+bgFactor, msg)
}
