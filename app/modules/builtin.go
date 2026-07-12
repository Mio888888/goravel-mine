package modules

func BuiltinPackage(id string, owner string) Package {
	return Package{
		ImportPath:    "goravel/app/modules/" + packageName(id),
		RegistryKey:   id,
		Version:       "1.0.0",
		Owner:         owner,
		ReleaseTrack:  "internal",
		Compatibility: []string{">=1.17.0 <2.0.0"},
	}
}

func BuiltinMetadata(name string, dependencies ...Dependency) Metadata {
	return Metadata{
		Name:         name,
		Version:      "1.0.0",
		Compatible:   ">=1.17.0",
		Dependencies: dependencies,
		Lifecycle: Lifecycle{
			Install:              "go run . artisan migrate && go run . artisan db:seed",
			Uninstall:            "manual data review required",
			Upgrade:              "go run . artisan migrate",
			Rollback:             "go run . artisan migrate:rollback",
			DestructiveCheck:     "go run . artisan module:manifest:check",
			BreakingChangePolicy: "document in release notes and block without review",
		},
		SeedStrategy: SeedStrategy{
			Mode:    "idempotent",
			Command: "go run . artisan db:seed",
		},
		Overrides: MetadataOverrides{
			RequiresRestart: Bool(true),
			SeedIdempotent:  Bool(true),
		},
	}
}

func RequiredDependency(id string) Dependency {
	return Dependency{ID: id, VersionConstraint: ">=1.0.0", Required: true}
}
