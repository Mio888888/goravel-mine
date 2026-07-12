package crudgen

import (
	"fmt"
	"strings"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

type dbColumn struct {
	ColumnName string `gorm:"column:column_name"`
	DataType   string `gorm:"column:data_type"`
	IsNullable string `gorm:"column:is_nullable"`
}

type keyColumn struct {
	ColumnName string `gorm:"column:column_name"`
}

func inspectTable(orm contractsorm.Orm, opts Options) (Table, error) {
	tableName := strings.TrimSpace(opts.Table)
	if tableName == "" {
		return Table{}, fmt.Errorf("table is required")
	}

	columns, err := readColumns(orm, tableName)
	if err != nil {
		return Table{}, err
	}
	if len(columns) == 0 {
		return Table{}, fmt.Errorf("table %s not found", tableName)
	}

	keys, err := readPrimaryKeys(orm, tableName)
	if err != nil {
		return Table{}, err
	}
	return buildTable(opts, columns, keys), nil
}

func readColumns(orm contractsorm.Orm, table string) ([]dbColumn, error) {
	columns := make([]dbColumn, 0)
	err := orm.Query().Raw(`
SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = ?
ORDER BY ordinal_position`, table).Scan(&columns)
	return columns, err
}

func readPrimaryKeys(orm contractsorm.Orm, table string) (map[string]bool, error) {
	keys := make([]keyColumn, 0)
	err := orm.Query().Raw(`
SELECT kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name
 AND tc.table_schema = kcu.table_schema
WHERE tc.table_schema = current_schema()
  AND tc.table_name = ?
  AND tc.constraint_type = 'PRIMARY KEY'`, table).Scan(&keys)
	if err != nil {
		return nil, err
	}

	out := make(map[string]bool, len(keys))
	for _, key := range keys {
		out[key.ColumnName] = true
	}
	return out, nil
}

func buildTable(opts Options, dbColumns []dbColumn, keys map[string]bool) Table {
	entity := singular(opts.Table)
	table := Table{
		Name:       opts.Table,
		StructName: pascalName(entity),
		VarName:    camelName(entity),
		FileName:   entity,
		RouteName:  kebabName(entity),
		Module:     packageName(opts.Module),
		Columns:    make([]Column, 0, len(dbColumns)),
	}
	for _, item := range dbColumns {
		column := buildColumn(item, keys[item.ColumnName])
		table.Columns = append(table.Columns, column)
	}
	return table
}

func buildColumn(item dbColumn, primary bool) Column {
	goType := postgresGoType(item.DataType)
	return Column{
		Name:       item.ColumnName,
		FieldName:  pascalName(item.ColumnName),
		GoType:     goType,
		JSONName:   item.ColumnName,
		DataType:   item.DataType,
		Nullable:   item.IsNullable == "YES",
		IsPrimary:  primary,
		IsSearch:   isSearchColumn(item.ColumnName, goType),
		IsWritable: isWritableColumn(item.ColumnName, primary),
	}
}

func postgresGoType(dataType string) string {
	switch dataType {
	case "bigint":
		return "int64"
	case "bigserial":
		return "uint64"
	case "integer", "serial":
		return "int"
	case "smallint":
		return "int16"
	case "boolean":
		return "bool"
	case "numeric", "decimal", "double precision", "real":
		return "float64"
	case "timestamp without time zone", "timestamp with time zone", "date":
		return "time.Time"
	case "json", "jsonb":
		return "models.JSONMap"
	default:
		return "string"
	}
}

func isSearchColumn(name, goType string) bool {
	if goType == "string" {
		return true
	}
	return name == "status" || strings.HasSuffix(name, "_at")
}

func isWritableColumn(name string, primary bool) bool {
	if primary {
		return false
	}
	switch name {
	case "created_at", "updated_at", "deleted_at":
		return false
	default:
		return true
	}
}
