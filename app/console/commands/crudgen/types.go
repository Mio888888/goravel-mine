package crudgen

import (
	"errors"
	"strings"
)

var ErrFileExists = errors.New("crud generator file exists")

type Options struct {
	Table  string
	Module string
	Force  bool
}

type Table struct {
	Name       string
	StructName string
	VarName    string
	FileName   string
	RouteName  string
	Module     string
	Columns    []Column
}

type Column struct {
	Name       string
	FieldName  string
	GoType     string
	JSONName   string
	DataType   string
	Nullable   bool
	IsPrimary  bool
	IsSearch   bool
	IsWritable bool
}

func (c Column) IsStringSearch() bool {
	return c.IsSearch && c.GoType == "string"
}

func (c Column) IsExactSearch() bool {
	return c.IsSearch && c.GoType != "string"
}

func (c Column) IsTime() bool {
	return strings.Contains(c.GoType, "time.Time")
}

func (t Table) HasTime() bool {
	for _, column := range t.Columns {
		if column.IsTime() {
			return true
		}
	}
	return false
}
