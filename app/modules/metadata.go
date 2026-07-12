package modules

import "strings"

type Dependency struct {
	ID                string
	VersionConstraint string
	Required          bool
}

type Lifecycle struct {
	Install              string
	Uninstall            string
	Upgrade              string
	Rollback             string
	DestructiveCheck     string
	SupportsHotDisable   bool
	RequiresRestart      bool
	BreakingChangePolicy string
}

type SeedStrategy struct {
	Mode       string
	Idempotent bool
	Command    string
	Notes      string
}

type FrontendArtifact struct {
	ModulePath  string
	ApiFiles    []string
	RouteFiles  []string
	LocaleFiles []string
	TypeFiles   []string
	TestFiles   []string
}

type Metadata struct {
	Name            string
	Version         string
	Compatible      string
	Dependencies    []Dependency
	Lifecycle       Lifecycle
	SeedStrategy    SeedStrategy
	Frontend        FrontendArtifact
	DestructiveNote string
	Overrides       MetadataOverrides
}

type MetadataOverrides struct {
	RequiresRestart    *bool
	SupportsHotDisable *bool
	SeedIdempotent     *bool
}

type MetadataProvider interface {
	Metadata() Metadata
}

type ModuleState struct {
	ID       string
	Enabled  bool
	Reason   string
	Metadata Metadata
}

func ModuleMetadata(module Module) Metadata {
	metadata := Metadata{
		Name:       module.ID(),
		Version:    "0.0.0",
		Compatible: ">=1.0.0",
		Lifecycle: Lifecycle{
			Install:              "manual",
			Uninstall:            "manual",
			Upgrade:              "manual",
			Rollback:             "migration rollback",
			DestructiveCheck:     "module:manifest:check",
			RequiresRestart:      true,
			BreakingChangePolicy: "manual review",
		},
		SeedStrategy: SeedStrategy{
			Mode:       "none",
			Idempotent: true,
			Command:    "go run . artisan db:seed",
		},
	}
	if provider, ok := module.(MetadataProvider); ok {
		provided := provider.Metadata()
		if strings.TrimSpace(provided.Name) != "" {
			metadata.Name = provided.Name
		}
		if strings.TrimSpace(provided.Version) != "" {
			metadata.Version = provided.Version
		}
		if strings.TrimSpace(provided.Compatible) != "" {
			metadata.Compatible = provided.Compatible
		}
		metadata.Dependencies = provided.Dependencies
		metadata.Lifecycle = mergeLifecycle(metadata.Lifecycle, provided.Lifecycle, provided.Overrides)
		metadata.SeedStrategy = mergeSeedStrategy(metadata.SeedStrategy, provided.SeedStrategy, provided.Overrides)
		metadata.Frontend = provided.Frontend
		metadata.DestructiveNote = provided.DestructiveNote
		metadata.Overrides = provided.Overrides
	}

	return metadata
}

func mergeLifecycle(base Lifecycle, override Lifecycle, overrides MetadataOverrides) Lifecycle {
	if override.Install != "" {
		base.Install = override.Install
	}
	if override.Uninstall != "" {
		base.Uninstall = override.Uninstall
	}
	if override.Upgrade != "" {
		base.Upgrade = override.Upgrade
	}
	if override.Rollback != "" {
		base.Rollback = override.Rollback
	}
	if override.DestructiveCheck != "" {
		base.DestructiveCheck = override.DestructiveCheck
	}
	if override.BreakingChangePolicy != "" {
		base.BreakingChangePolicy = override.BreakingChangePolicy
	}
	if overrides.SupportsHotDisable != nil {
		base.SupportsHotDisable = *overrides.SupportsHotDisable
	}
	if overrides.RequiresRestart != nil {
		base.RequiresRestart = *overrides.RequiresRestart
	}

	return base
}

func mergeSeedStrategy(base SeedStrategy, override SeedStrategy, overrides MetadataOverrides) SeedStrategy {
	if override.Mode != "" {
		base.Mode = override.Mode
	}
	if override.Command != "" {
		base.Command = override.Command
	}
	if override.Notes != "" {
		base.Notes = override.Notes
	}
	if overrides.SeedIdempotent != nil {
		base.Idempotent = *overrides.SeedIdempotent
	}

	return base
}

func Bool(value bool) *bool {
	return &value
}
