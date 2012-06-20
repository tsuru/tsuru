package main

import "strings"

type Table struct {
	Headers Row
	rows    []Row
}

type Row []string

func NewTable() *Table {
	return &Table{}
}

func (t *Table) String() string {
	sizes := t.columnsSize()
	result := t.separator()
	if t.Headers != nil {
		for column, header := range t.Headers {
			result = result + "| " + header
			result = result + strings.Repeat(" ", sizes[column]+1-len(header))
		}
		result = result + "|\n"
		result = result + t.separator()
	}
	for _, row := range t.rows {
		for column, field := range row {
			result = result + "| " + field
			result = result + strings.Repeat(" ", sizes[column]+1-len(field))
		}
		result = result + "|\n"
	}
	result = result + t.separator()
	return result
}

func (t *Table) Bytes() []byte {
	return []byte(t.String())
}

func (t *Table) AddRow(row Row) {
	t.rows = append(t.rows, row)
}

func (t *Table) columnsSize() []int {
	columns := len(t.rows[0])
	sizes := make([]int, columns)
	for _, row := range t.rows {
		for i := 0; i < columns; i++ {
			if len(row[i]) > sizes[i] {
				sizes[i] = len(row[i])
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
