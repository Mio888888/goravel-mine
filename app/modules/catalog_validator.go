package modules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (c Catalog) Validate() error {
	return c.validate(true)
}

func (c Catalog) ValidateRuntime() error {
	return c.validate(false)
}

func (c Catalog) validate(checkFiles bool) error {
	checks := []duplicateCheck{
		{kind: "module id", values: c.moduleIDs()},
		{kind: "route name", values: c.routeNames()},
		{kind: "route path", values: c.routePaths()},
		{kind: "menu key", values: c.menuKeys()},
		{kind: "permission key", values: c.permissionKeys()},
	}
	var errs []error
	for _, check := range checks {
		errs = append(errs, check.duplicates()...)
	}
	errs = append(errs, c.duplicateModuleMetadataFiles()...)
	errs = append(errs, c.missingRouteInstallers()...)
	errs = append(errs, c.missingRoutePermissions()...)
	errs = append(errs, c.missingMenuPermissions()...)
	if checkFiles {
		errs = append(errs, c.missingFiles("openapi file", c.openAPIFiles())...)
		errs = append(errs, c.missingFiles("test template", c.testTemplates())...)
	}
	return errors.Join(errs...)
}

type duplicateCheck struct {
	kind   string
	values []string
}

func (c duplicateCheck) duplicates() []error {
	seen := map[string]bool{}
	var errs []error
	for _, value := range c.values {
		if value == "" {
			continue
		}
		if seen[value] {
			errs = append(errs, fmt.Errorf("duplicate %s: %s", c.kind, value))
			continue
		}
		seen[value] = true
	}
	return errs
}

func (c Catalog) moduleIDs() []string {
	return mapCatalogModules(c.Modules, func(module ModuleCatalog) string { return module.ID })
}

func (c Catalog) routeNames() []string {
	return collectCatalogValues(c.Modules, func(module ModuleCatalog) []Route { return module.Routes }, func(route Route) string {
		return route.Name
	})
}

func (c Catalog) routePaths() []string {
	return collectCatalogValues(c.Modules, func(module ModuleCatalog) []Route { return module.Routes }, func(route Route) string {
		return route.Method + " " + route.Path
	})
}

func (c Catalog) menuKeys() []string {
	return collectCatalogValues(c.Modules, func(module ModuleCatalog) []Menu { return module.Menus }, func(menu Menu) string {
		return menu.Key
	})
}

func (c Catalog) permissionKeys() []string {
	return collectCatalogValues(c.Modules, func(module ModuleCatalog) []Permission { return module.Permissions }, func(permission Permission) string {
		return permission.Key
	})
}

func (c Catalog) openAPIFiles() []string {
	return collectCatalogValues(c.Modules, func(module ModuleCatalog) []string { return module.OpenAPIFiles }, strings.TrimSpace)
}

func (c Catalog) testTemplates() []string {
	return collectCatalogValues(c.Modules, func(module ModuleCatalog) []string { return module.TestTemplates }, strings.TrimSpace)
}

func mapCatalogModules(modules []ModuleCatalog, project func(ModuleCatalog) string) []string {
	values := make([]string, 0, len(modules))
	for _, module := range modules {
		values = append(values, project(module))
	}
	return values
}

func collectCatalogValues[T any](
	modules []ModuleCatalog,
	items func(ModuleCatalog) []T,
	project func(T) string,
) []string {
	var values []string
	for _, module := range modules {
		for _, item := range items(module) {
			values = append(values, project(item))
		}
	}
	return values
}

func (c Catalog) missingFiles(kind string, files []string) []error {
	var errs []error
	for _, file := range files {
		if file == "" {
			continue
		}
		if _, err := os.Stat(resolveRepositoryPath(file)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("%s does not exist: %s", kind, file))
				continue
			}
			errs = append(errs, fmt.Errorf("%s cannot be read: %s: %w", kind, file, err))
		}
	}
	return errs
}

func (c Catalog) duplicateModuleMetadataFiles() []error {
	var errs []error
	for _, module := range c.Modules {
		errs = append(errs, duplicateCheck{kind: "openapi file", values: cleanPaths(module.OpenAPIFiles)}.duplicates()...)
		errs = append(errs, duplicateCheck{kind: "test template", values: cleanPaths(module.TestTemplates)}.duplicates()...)
	}
	return errs
}

func cleanPaths(paths []string) []string {
	values := make([]string, 0, len(paths))
	for _, path := range paths {
		values = append(values, strings.TrimSpace(path))
	}
	return values
}

func resolveRepositoryPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	for {
		candidate := filepath.Join(cwd, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return candidate
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return path
		}
		cwd = parent
	}
}

func (c Catalog) permissionSet() map[string]bool {
	permissions := map[string]bool{}
	for _, value := range c.permissionKeys() {
		permissions[value] = true
	}
	return permissions
}

func (c Catalog) missingRoutePermissions() []error {
	permissions := c.permissionSet()
	var errs []error
	for _, module := range c.Modules {
		for _, route := range module.Routes {
			for _, permission := range route.PermissionKeys() {
				if !permissions[permission] {
					errs = append(errs, fmt.Errorf("route %s references missing permission: %s", routeLabel(route), permission))
				}
			}
		}
	}
	return errs
}

func (c Catalog) missingRouteInstallers() []error {
	var errs []error
	for _, module := range c.Modules {
		for _, route := range module.Routes {
			if route.Install == nil {
				errs = append(errs, fmt.Errorf("route %s has no installer", routeLabel(route)))
			}
		}
	}
	return errs
}

func (c Catalog) missingMenuPermissions() []error {
	permissions := c.permissionSet()
	var errs []error
	for _, module := range c.Modules {
		for _, menu := range module.Menus {
			if menu.Permission != "" && !permissions[menu.Permission] {
				errs = append(errs, fmt.Errorf("menu %s references missing permission: %s", menu.Key, menu.Permission))
			}
		}
	}
	return errs
}

func routeLabel(route Route) string {
	if route.Name != "" {
		return route.Name
	}
	return route.Method + " " + route.Path
}
