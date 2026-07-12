package modulecatalog

import (
	"errors"
	"fmt"
	"strings"
)

type manifestFrontendFileSet struct {
	apiFiles []string
	views    []string
	locales  []string
}

type frontendModuleValidation struct {
	module      ManifestItem
	frontend    ManifestFrontendParity
	menus       map[string]ManifestMenu
	permissions map[string]ManifestPermission
}

type frontendFileDiff struct {
	moduleID string
	kind     string
	expected []string
	actual   []string
}

func validateManifestParity(manifest Manifest, seed ManifestSeedParity, frontend ManifestFrontendParity) error {
	var errs []error
	if len(seed.Menus) > 0 || len(seed.Permissions) > 0 {
		errs = append(errs, validateSeedParity(manifest, seed)...)
	}
	if frontend.HasEntries() {
		errs = append(errs, validateFrontendParity(manifest, frontend)...)
	}
	return errors.Join(errs...)
}

func validateSeedParity(manifest Manifest, seed ManifestSeedParity) []error {
	seedByKey := indexMenus(seed.Menus)
	permissionByKey := indexPermissions(seed.Permissions)
	var errs []error
	for _, module := range manifest.Modules {
		for _, menu := range module.Menus {
			errs = append(errs, seedMenuErrors(menu, seedByKey)...)
		}
		for _, permission := range module.Permissions {
			if _, ok := permissionByKey[permission.Key]; !ok {
				errs = append(errs, fmt.Errorf("seed permission missing: %s", permission.Key))
			}
		}
	}
	return errs
}

func seedMenuErrors(menu ManifestMenu, seedByKey map[string]ManifestMenu) []error {
	seed, ok := seedByKey[menu.Key]
	if !ok {
		return []error{fmt.Errorf("seed menu missing: %s", menu.Key)}
	}
	var errs []error
	if seed.Path != menu.Path {
		errs = append(errs, fmt.Errorf("seed menu path mismatch: %s", menu.Key))
	}
	if seed.Component != menu.Component {
		errs = append(errs, fmt.Errorf("seed menu component mismatch: %s", menu.Key))
	}
	if seed.Permission != "" && seed.Permission != menu.Permission {
		errs = append(errs, fmt.Errorf("seed menu permission mismatch: %s", menu.Key))
	}
	return errs
}

func validateFrontendParity(manifest Manifest, frontend ManifestFrontendParity) []error {
	frontendMenus := indexMenus(frontend.Menus)
	frontendPermissions := indexPermissions(frontend.Permissions)
	manifestMenus, manifestPermissions := manifestEntryIndexes(manifest)
	var errs []error
	for _, module := range manifest.Modules {
		errs = append(errs, frontendModuleErrors(frontendModuleValidation{
			module: module, frontend: frontend, menus: frontendMenus, permissions: frontendPermissions,
		})...)
	}
	errs = append(errs, extraFrontendErrors(frontend, manifestMenus, manifestPermissions)...)
	return errs
}

func frontendModuleErrors(validation frontendModuleValidation) []error {
	var errs []error
	for _, menu := range validation.module.Menus {
		errs = append(errs, frontendMenuErrors(menu, validation.menus)...)
	}
	for _, permission := range validation.module.Permissions {
		if _, ok := validation.permissions[permission.Key]; !ok {
			errs = append(errs, fmt.Errorf("frontend permission missing: %s", permission.Key))
		}
	}
	return append(errs, validateFrontendFileParity(validation.module, validation.frontend)...)
}

func frontendMenuErrors(menu ManifestMenu, menus map[string]ManifestMenu) []error {
	frontend, ok := menus[menu.Key]
	if !ok {
		return []error{fmt.Errorf("frontend menu missing: %s", menu.Key)}
	}
	var errs []error
	if frontend.Path != menu.Path {
		errs = append(errs, fmt.Errorf("frontend menu path mismatch: %s", menu.Key))
	}
	if frontend.Component != menu.Component {
		errs = append(errs, fmt.Errorf("frontend menu component mismatch: %s", menu.Key))
	}
	if frontend.Permission != menu.Permission {
		errs = append(errs, fmt.Errorf("frontend menu permission mismatch: %s", menu.Key))
	}
	return errs
}

func extraFrontendErrors(
	frontend ManifestFrontendParity,
	menus map[string]ManifestMenu,
	permissions map[string]ManifestPermission,
) []error {
	var errs []error
	for _, menu := range frontend.Menus {
		if _, ok := menus[menu.Key]; !ok {
			errs = append(errs, fmt.Errorf("frontend menu extra: %s", menu.Key))
		}
	}
	for _, permission := range frontend.Permissions {
		if _, ok := permissions[permission.Key]; !ok {
			errs = append(errs, fmt.Errorf("frontend permission extra: %s", permission.Key))
		}
	}
	return errs
}

func manifestEntryIndexes(manifest Manifest) (map[string]ManifestMenu, map[string]ManifestPermission) {
	menus := map[string]ManifestMenu{}
	permissions := map[string]ManifestPermission{}
	for _, module := range manifest.Modules {
		for _, menu := range module.Menus {
			menus[menu.Key] = menu
		}
		for _, permission := range module.Permissions {
			permissions[permission.Key] = permission
		}
	}
	return menus, permissions
}

func indexMenus(items []ManifestMenu) map[string]ManifestMenu {
	indexed := make(map[string]ManifestMenu, len(items))
	for _, item := range items {
		indexed[item.Key] = item
	}
	return indexed
}

func indexPermissions(items []ManifestPermission) map[string]ManifestPermission {
	indexed := make(map[string]ManifestPermission, len(items))
	for _, item := range items {
		indexed[item.Key] = item
	}
	return indexed
}

func validateFrontendFileParity(module ManifestItem, frontend ManifestFrontendParity) []error {
	expected := manifestFrontendFiles(module.Frontend)
	if len(expected.apiFiles) == 0 && len(expected.views) == 0 && len(expected.locales) == 0 {
		return nil
	}
	actual := manifestFrontendFileSet{
		apiFiles: frontendMineAdminWebFiles(frontend.ApiFiles),
		views:    frontendMineAdminWebFiles(frontend.Views),
		locales:  frontendMineAdminWebFiles(frontend.Locales),
	}
	var errs []error
	errs = append(errs, frontendFileDiffErrors(frontendFileDiff{
		moduleID: module.ID, kind: "api", expected: expected.apiFiles, actual: actual.apiFiles,
	})...)
	errs = append(errs, frontendFileDiffErrors(frontendFileDiff{
		moduleID: module.ID, kind: "view", expected: expected.views, actual: actual.views,
	})...)
	errs = append(errs, frontendFileDiffErrors(frontendFileDiff{
		moduleID: module.ID, kind: "locale", expected: expected.locales, actual: actual.locales,
	})...)
	return errs
}

func manifestFrontendFiles(frontend ManifestFrontend) manifestFrontendFileSet {
	return manifestFrontendFileSet{
		apiFiles: frontendMineAdminWebFiles(frontend.ApiFiles),
		views:    frontendMineAdminWebFiles(frontend.RouteFiles),
		locales:  frontendLocaleFiles(frontend),
	}
}

func frontendMineAdminWebFiles(files []string) []string {
	items := cleanFrontendManifestPaths(files)
	for index, file := range items {
		items[index] = strings.TrimPrefix(file, "MineAdmin-web/")
	}
	return items
}

func frontendLocaleFiles(frontend ManifestFrontend) []string {
	if len(frontend.LocaleFiles) > 0 {
		return frontendMineAdminWebFiles(frontend.LocaleFiles)
	}
	modulePath := strings.TrimPrefix(normalizeFrontendManifestPath(frontend.ModulePath), "MineAdmin-web/")
	if modulePath == "" {
		return nil
	}
	return []string{strings.TrimSuffix(modulePath, "/") + "/locales/zh_CN.yaml"}
}

func frontendFileDiffErrors(diff frontendFileDiff) []error {
	expectedSet := stringSet(diff.expected)
	actualSet := stringSet(diff.actual)
	var errs []error
	for _, file := range diff.expected {
		if _, ok := actualSet[file]; !ok {
			errs = append(errs, fmt.Errorf("frontend %s file missing for module %s: %s", diff.kind, diff.moduleID, file))
		}
	}
	for _, file := range diff.actual {
		if _, ok := expectedSet[file]; !ok && frontendFileBelongsToModule(diff.moduleID, file) {
			errs = append(errs, fmt.Errorf("frontend %s file extra for module %s: %s", diff.kind, diff.moduleID, file))
		}
	}
	return errs
}

func stringSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}

func frontendFileBelongsToModule(moduleID string, file string) bool {
	return strings.HasPrefix(file, "src/modules/"+moduleID+"/")
}

func cleanFrontendManifestPaths(files []string) []string {
	items := make([]string, 0, len(files))
	seen := map[string]struct{}{}
	for _, file := range files {
		file = normalizeFrontendManifestPath(file)
		if file == "" {
			continue
		}
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		items = append(items, file)
	}
	return items
}

func normalizeFrontendManifestPath(file string) string {
	file = strings.TrimSpace(strings.ReplaceAll(file, "\\", "/"))
	return strings.TrimPrefix(file, "./")
}
