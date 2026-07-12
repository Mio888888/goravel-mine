package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
	"goravel/database/seeders"
)

type ModuleManifestCheckCommand struct{}

func (r *ModuleManifestCheckCommand) Signature() string {
	return "module:manifest:check"
}

func (r *ModuleManifestCheckCommand) Description() string {
	return "Validate backend module route, menu, permission, OpenAPI, and test metadata"
}

func (r *ModuleManifestCheckCommand) Extend() command.Extend {
	return command.Extend{
		Category: "module",
		Flags: []command.Flag{
			&command.BoolFlag{Name: "artifacts", Usage: "Validate OpenAPI and test artifact files exist"},
			&command.BoolFlag{Name: "frontend", Usage: "Validate frontend module menu and permission parity"},
		},
	}
}

func (r *ModuleManifestCheckCommand) Handle(ctx console.Context) error {
	service := modulecatalog.NewService(moduleboot.Modules())
	if err := validateManifestService(service, ctx.OptionBool("artifacts")); err != nil {
		ctx.Error(err.Error())
		return err
	}
	frontend, err := frontendManifestForCheck(".", ctx.OptionBool("frontend"))
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	if err := service.ValidateManifestParity(seedManifest(service.Manifest()), frontend); err != nil {
		ctx.Error(err.Error())
		return err
	}

	ctx.Success("module manifest valid")
	return nil
}

func validateManifestService(service modulecatalog.Service, checkArtifacts bool) error {
	if checkArtifacts {
		if err := service.Validate(); err != nil {
			return err
		}
		return modulecatalog.LintManifestOpenAPI(service.Manifest())
	}
	return service.ValidateRuntime()
}

func frontendManifestForCheck(root string, enabled bool) (modulecatalog.ManifestFrontendParity, error) {
	if !enabled {
		return modulecatalog.ManifestFrontendParity{}, nil
	}
	return frontendManifestFromRoot(root)
}

func frontendManifestFromRoot(root string) (modulecatalog.ManifestFrontendParity, error) {
	pattern := filepath.Join(root, "MineAdmin-web/src/modules/*/manifest.ts")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return modulecatalog.ManifestFrontendParity{}, fmt.Errorf("frontend manifest glob failed: %w", err)
	}
	if len(paths) == 0 {
		return modulecatalog.ManifestFrontendParity{}, fmt.Errorf("frontend manifest read failed: no manifests matched %s", pattern)
	}
	parity := modulecatalog.ManifestFrontendParity{}
	for _, path := range paths {
		payload, err := os.ReadFile(path)
		if err != nil {
			return modulecatalog.ManifestFrontendParity{}, fmt.Errorf("frontend manifest read failed: %w", err)
		}
		item, err := parseFrontendManifestParity(payload)
		if err != nil {
			return modulecatalog.ManifestFrontendParity{}, fmt.Errorf("frontend manifest parse failed: %s: %w", path, err)
		}
		parity.Menus = append(parity.Menus, item.Menus...)
		parity.Permissions = append(parity.Permissions, item.Permissions...)
		parity.ApiFiles = append(parity.ApiFiles, item.ApiFiles...)
		parity.Views = append(parity.Views, item.Views...)
		parity.Locales = append(parity.Locales, item.Locales...)
	}
	if err := validateFrontendManifestFiles(root, parity); err != nil {
		return modulecatalog.ManifestFrontendParity{}, err
	}
	return parity, nil
}

func parseFrontendManifestParity(payload []byte) (modulecatalog.ManifestFrontendParity, error) {
	source := string(payload)
	parity := modulecatalog.ManifestFrontendParity{
		Menus:       parseFrontendManifestMenus(source),
		Permissions: parseFrontendManifestPermissions(source),
		ApiFiles:    parseFrontendManifestStringArray(source, "apiFiles"),
		Views:       parseFrontendManifestStringArray(source, "views"),
		Locales:     parseFrontendManifestStringArray(source, "locales"),
	}
	if len(parity.Menus) == 0 || len(parity.Permissions) == 0 {
		return modulecatalog.ManifestFrontendParity{}, fmt.Errorf("frontend manifest parse failed: menus=%d permissions=%d", len(parity.Menus), len(parity.Permissions))
	}
	return parity, nil
}

func parseFrontendManifestStringArray(source string, section string) []string {
	body := parseFrontendManifestArrayBody(source, section)
	if body == "" {
		return nil
	}
	valueRe := regexp.MustCompile(`'([^']*)'`)
	matches := valueRe.FindAllStringSubmatch(body, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		value := strings.TrimSpace(match[1])
		if value != "" {
			values = append(values, value)
		}
	}

	return values
}

func validateFrontendManifestFiles(root string, parity modulecatalog.ManifestFrontendParity) error {
	var errs []error
	errs = append(errs, missingFrontendFiles(root, "api", parity.ApiFiles)...)
	errs = append(errs, missingFrontendFiles(root, "view", parity.Views)...)
	errs = append(errs, missingFrontendFiles(root, "locale", parity.Locales)...)

	return errors.Join(errs...)
}

func missingFrontendFiles(root string, kind string, files []string) []error {
	var errs []error
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		path, err := frontendManifestFilePath(root, file)
		if err != nil {
			errs = append(errs, fmt.Errorf("frontend %s file %w: %s", kind, err, file))
			continue
		}
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("frontend %s file does not exist: %s", kind, file))
				continue
			}
			errs = append(errs, fmt.Errorf("frontend %s file cannot be read: %s: %w", kind, file, err))
		}
	}

	return errs
}

func frontendManifestFilePath(root string, file string) (string, error) {
	frontRoot, err := filepath.Abs(filepath.Join(root, "MineAdmin-web"))
	if err != nil {
		return "", fmt.Errorf("cannot resolve frontend root")
	}
	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(frontRoot, file)
		if strings.HasPrefix(filepath.ToSlash(file), "MineAdmin-web/") {
			path = filepath.Join(root, file)
		}
	}
	path, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("cannot be resolved")
	}
	rel, err := filepath.Rel(frontRoot, path)
	if err != nil {
		return "", fmt.Errorf("cannot be resolved")
	}
	if rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("escapes frontend root")
	}

	return path, nil
}

func parseFrontendManifestMenus(source string) []modulecatalog.ManifestMenu {
	blocks := parseFrontendManifestObjectBlocks(source, "menus")
	menus := make([]modulecatalog.ManifestMenu, 0, len(blocks))
	for _, block := range blocks {
		fields := parseFrontendManifestFields(block)
		if fields["key"] == "" {
			continue
		}
		menus = append(menus, modulecatalog.ManifestMenu{
			Key:        fields["key"],
			Title:      fields["title"],
			Path:       fields["path"],
			Component:  fields["component"],
			Permission: fields["permission"],
		})
	}
	return menus
}

func parseFrontendManifestPermissions(source string) []modulecatalog.ManifestPermission {
	blocks := parseFrontendManifestObjectBlocks(source, "permissions")
	permissions := make([]modulecatalog.ManifestPermission, 0, len(blocks))
	for _, block := range blocks {
		fields := parseFrontendManifestFields(block)
		if fields["key"] == "" {
			continue
		}
		permissions = append(permissions, modulecatalog.ManifestPermission{
			Key:         fields["key"],
			Description: fields["title"],
		})
	}
	return permissions
}

func parseFrontendManifestObjectBlocks(source string, section string) []string {
	body := parseFrontendManifestArrayBody(source, section)
	if body == "" {
		return nil
	}
	blockRe := regexp.MustCompile(`(?s)\{([^{}]*)\}`)
	matches := blockRe.FindAllStringSubmatch(body, -1)
	blocks := make([]string, 0, len(matches))
	for _, item := range matches {
		blocks = append(blocks, item[1])
	}
	return blocks
}

func parseFrontendManifestArrayBody(source string, section string) string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(section) + `\s*:\s*\[`)
	match := re.FindStringIndex(source)
	if len(match) != 2 {
		return ""
	}
	start := match[1] - 1
	depth := 0
	inString := false
	for index := start; index < len(source); index++ {
		ch := source[index]
		if ch == '\'' && (index == 0 || source[index-1] != '\\') {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return source[start+1 : index]
			}
		}
	}

	return ""
}

func parseFrontendManifestFields(block string) map[string]string {
	re := regexp.MustCompile(`(?s)\b(key|title|path|component|permission)\s*:\s*'([^']*)'`)
	matches := re.FindAllStringSubmatch(block, -1)
	fields := map[string]string{}
	for _, match := range matches {
		fields[match[1]] = match[2]
	}
	return fields
}

func seedManifest(manifest modulecatalog.Manifest) modulecatalog.ManifestSeedParity {
	parity := modulecatalog.ManifestSeedParity{}
	platformSeeds := seeders.PlatformMenuCatalogSeeds()
	tenantSeeds := seeders.TenantMenuCatalogSeeds()
	parity.Menus = seedMenuCatalog(nil, platformSeeds)
	parity.Menus = seedMenuCatalog(parity.Menus, tenantSeeds)
	parity.Permissions = seedPermissionCatalog(nil, platformSeeds)
	parity.Permissions = seedPermissionCatalog(parity.Permissions, tenantSeeds)
	for _, module := range manifest.Modules {
		if module.SeedStrategy.Mode != "manifest" {
			continue
		}
		for _, menu := range module.Menus {
			parity.Menus = append(parity.Menus, menu)
		}
		parity.Permissions = append(parity.Permissions, seedableManifestPermissions(module)...)
	}
	return parity
}

func seedableManifestPermissions(module modulecatalog.ManifestItem) []modulecatalog.ManifestPermission {
	menuKeys := map[string]bool{}
	for _, menu := range module.Menus {
		menuKeys[menu.Key] = true
	}

	items := make([]modulecatalog.ManifestPermission, 0, len(module.Permissions))
	for _, permission := range module.Permissions {
		if menuKeys[permission.Key] || hasManifestPermissionParent(permission.Key, module.Menus) {
			items = append(items, permission)
		}
	}

	return items
}

func hasManifestPermissionParent(permissionKey string, menus []modulecatalog.ManifestMenu) bool {
	for _, menu := range menus {
		if manifestSeedTable(permissionKey) == manifestSeedTable(menu.Key) && strings.HasPrefix(permissionKey, menu.Key+":") {
			return true
		}
	}

	return false
}

func manifestSeedTable(key string) string {
	if strings.HasPrefix(key, "platform:") {
		return "platform_menu"
	}

	return "menu"
}

func seedMenuCatalog(items []modulecatalog.ManifestMenu, seeds []seeders.MenuCatalogSeed) []modulecatalog.ManifestMenu {
	for _, seed := range seeds {
		items = append(items, modulecatalog.ManifestMenu{
			Key:       seed.Name,
			ParentKey: "",
			Title:     seedMetaString(seed.Meta, "title"),
			Path:      seed.Path,
			Component: seed.Component,
			Type:      seedMetaString(seed.Meta, "type"),
			I18n:      seedMetaString(seed.Meta, "i18n"),
			Sort:      seed.Sort,
		})
	}
	return items
}

func seedPermissionCatalog(items []modulecatalog.ManifestPermission, seeds []seeders.MenuCatalogSeed) []modulecatalog.ManifestPermission {
	for _, seed := range seeds {
		if seedMetaString(seed.Meta, "type") != "B" {
			continue
		}
		items = append(items, modulecatalog.ManifestPermission{
			Key:         seed.Name,
			Description: seedMetaString(seed.Meta, "title"),
		})
	}
	return items
}

func seedMetaString(meta map[string]any, key string) string {
	value, _ := meta[key].(string)
	return value
}
