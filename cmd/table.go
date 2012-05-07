package cmd

type Table struct{}

func NewTable() *Table {
	return &Table{}
}

func (t *Table) String() string {
	return ""
}
