package modulecatalog

import (
	"strings"
	"time"

	"goravel/app/modules"
)

type compatibilityCheck func(modules.Package, string) (bool, error)

type compatibilityProjector struct {
	now    func() time.Time
	check  compatibilityCheck
	mapper dtoMapper
}

func newCompatibilityProjector(now func() time.Time, check compatibilityCheck) compatibilityProjector {
	return compatibilityProjector{now: now, check: check}
}

func (p compatibilityProjector) project(catalog modules.Catalog, frameworkVersion string) CompatibilityMatrix {
	frameworkVersion = strings.TrimSpace(frameworkVersion)
	status := "passed"
	items := make([]CompatibilityMatrixModule, 0, len(catalog.Modules))
	for _, module := range catalog.Modules {
		compatible, err := p.check(module.Package, frameworkVersion)
		if module.Enabled && (!compatible || err != nil) {
			status = "failed"
		}
		items = append(items, p.mapper.compatibilityItem(compatibilityProjection{
			module: module, compatible: compatible, err: err,
		}))
	}
	return CompatibilityMatrix{
		Status: status, FrameworkVersion: frameworkVersion, GeneratedAt: p.now().UTC(), Modules: items,
	}
}
