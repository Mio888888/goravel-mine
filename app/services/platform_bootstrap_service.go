package services

import (
	"github.com/goravel/framework/support/env"

	"goravel/app/facades"
	"goravel/database/seeders"
)

type PlatformBootstrapService struct{}

func NewPlatformBootstrapService() *PlatformBootstrapService {
	return &PlatformBootstrapService{}
}

func (s *PlatformBootstrapService) EnsureLocalDefaults() error {
	if !s.shouldAutoSeed() || !s.hasPlatformBootstrapTables() {
		return nil
	}
	exists, err := s.platformAdminExists()
	if err != nil {
		return err
	}
	restore := seeders.SetConnection(PlatformConnection())
	defer restore()
	if !exists {
		return s.seedPlatformAccessDefaults()
	}
	return s.syncPlatformAccessDefaults()
}

func (s *PlatformBootstrapService) platformAdminExists() (bool, error) {
	count, err := OrmForConnection(PlatformConnection()).
		Query().
		Table("platform_user").
		Where("username", "admin").
		Where("user_type", "900").
		Count()
	return count > 0, err
}

func (s *PlatformBootstrapService) hasPlatformBootstrapTables() bool {
	for _, table := range []string{
		"platform_user",
		"platform_role",
		"platform_user_belongs_role",
		"platform_menu",
		"platform_role_belongs_menu",
		"platform_casbin_rule",
	} {
		if !facades.Schema().HasTable(table) {
			return false
		}
	}
	return true
}

func (s *PlatformBootstrapService) seedPlatformAccessDefaults() error {
	for _, item := range []interface{ Run() error }{
		&seeders.PlatformAdminSeeder{},
		&seeders.PlatformMenuSeeder{},
		&seeders.PlatformCasbinSeeder{},
	} {
		if err := item.Run(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PlatformBootstrapService) syncPlatformAccessDefaults() error {
	for _, item := range []interface{ Run() error }{
		&seeders.PlatformMenuSeeder{},
		&seeders.PlatformCasbinSeeder{},
	} {
		if err := item.Run(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PlatformBootstrapService) shouldAutoSeed() bool {
	appEnv := facades.Config().GetString("app.env")
	return env.IsTesting() || appEnv == "local"
}
