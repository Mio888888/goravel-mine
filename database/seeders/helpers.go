package seeders

import (
	"sync"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
)

var seederConnection = struct {
	sync.RWMutex
	name string
}{}

func SetConnection(name string) func() {
	seederConnection.Lock()
	previous := seederConnection.name
	seederConnection.name = name
	seederConnection.Unlock()

	return func() {
		seederConnection.Lock()
		seederConnection.name = previous
		seederConnection.Unlock()
	}
}

func orm() contractsorm.Orm {
	seederConnection.RLock()
	name := seederConnection.name
	seederConnection.RUnlock()
	if name == "" {
		return facades.Orm()
	}
	return facades.Orm().Connection(name)
}

func exec(sql string, values ...any) error {
	_, err := orm().Query().Exec(sql, values...)

	return err
}

func query(sql string, dest any, values ...any) error {
	return orm().Query().Raw(sql, values...).Scan(dest)
}

func syncSequence(table, column string) error {
	_, err := orm().Query().Exec(`
		SELECT setval(
			pg_get_serial_sequence(?, ?),
			COALESCE((SELECT MAX(id) FROM `+table+`), 1),
			true
		)
	`, table, column)

	return err
}
