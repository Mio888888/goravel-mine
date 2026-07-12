package seeders

type PlatformBootstrapSeeder struct{}

func (s *PlatformBootstrapSeeder) Signature() string {
	return "platform_bootstrap_seed"
}

func (s *PlatformBootstrapSeeder) Run() error {
	for _, item := range []interface{ Run() error }{
		&PlatformDictionarySeeder{},
		&PlatformAdminSeeder{},
		&PlatformMenuSeeder{},
		&PlatformCasbinSeeder{},
	} {
		if err := item.Run(); err != nil {
			return err
		}
	}
	return nil
}
