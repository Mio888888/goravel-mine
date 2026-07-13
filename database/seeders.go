package database

import (
	"github.com/goravel/framework/contracts/database/seeder"

	"goravel/database/seeders"
)

func Seeders(moduleSeeders []seeder.Seeder) []seeder.Seeder {
	items := []seeder.Seeder{
		&seeders.TenantPlanSeeder{},
		&seeders.TenantSeeder{},
		&seeders.PlatformBootstrapSeeder{},
		&seeders.PlatformDictionarySeeder{},
		&seeders.PlatformAdminSeeder{},
		&seeders.PlatformMenuSeeder{},
		&seeders.PlatformCasbinSeeder{},
		&seeders.AdminSeeder{},
		&seeders.MenuSeeder{},
		&seeders.DictionarySeeder{},
		&seeders.DepartmentSeeder{},
		&seeders.CasbinSeeder{},
	}

	return append(items, moduleSeeders...)
}
