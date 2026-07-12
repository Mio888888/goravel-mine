package seeders

import (
	"encoding/json"
	"strings"

	"github.com/goravel/framework/contracts/database/seeder"
)

type ManifestMenu struct {
	Key        string
	ParentKey  string
	Title      string
	Path       string
	Component  string
	Permission string
	Type       string
	I18n       string
	Sort       int
}

type ManifestPermission struct {
	Key         string
	Description string
}

type ManifestMenuSeeder struct {
	moduleID    string
	menus       []ManifestMenu
	permissions []ManifestPermission
}

func NewManifestMenuSeeder(moduleID string, menus []ManifestMenu, permissions []ManifestPermission) seeder.Seeder {
	return &ManifestMenuSeeder{
		moduleID:    moduleID,
		menus:       append([]ManifestMenu(nil), menus...),
		permissions: append([]ManifestPermission(nil), permissions...),
	}
}

func (s *ManifestMenuSeeder) Signature() string {
	id := strings.ReplaceAll(s.moduleID, "-", "_")
	return id + "_manifest_menu_seed"
}

func (s *ManifestMenuSeeder) Run() error {
	for _, table := range []string{"platform_menu", "menu"} {
		if err := s.seedTable(table); err != nil {
			return err
		}
	}

	return nil
}

func (s *ManifestMenuSeeder) seedTable(table string) error {
	menus := s.menusForTable(table)
	permissions := s.permissionsForTable(table)
	if len(menus) == 0 && len(permissions) == 0 {
		return nil
	}

	menuKeys := map[string]bool{}
	for _, menu := range menus {
		menuKeys[menu.Key] = true
		if err := insertManifestMenu(table, s.moduleID, menu); err != nil {
			return err
		}
	}
	for index, permission := range permissions {
		if menuKeys[permission.Key] {
			continue
		}
		parentKey, ok := parentMenuForPermission(permission, menus)
		if !ok {
			continue
		}
		if err := insertManifestPermission(table, s.moduleID, parentKey, permission, index); err != nil {
			return err
		}
	}
	if err := grantManifestMenuAccess(table, s.moduleID); err != nil {
		return err
	}

	return syncSequence(table, "id")
}

func (s *ManifestMenuSeeder) menusForTable(table string) []ManifestMenu {
	items := make([]ManifestMenu, 0)
	for _, menu := range s.menus {
		if manifestMenuTable(menu) == table {
			items = append(items, menu)
		}
	}

	return items
}

func (s *ManifestMenuSeeder) permissionsForTable(table string) []ManifestPermission {
	items := make([]ManifestPermission, 0)
	for _, permission := range s.permissions {
		if manifestPermissionTable(permission) == table {
			items = append(items, permission)
		}
	}

	return items
}

func manifestMenuTable(menu ManifestMenu) string {
	if strings.HasPrefix(menu.Key, "platform:") {
		return "platform_menu"
	}

	return "menu"
}

func manifestPermissionTable(permission ManifestPermission) string {
	if strings.HasPrefix(permission.Key, "platform:") {
		return "platform_menu"
	}

	return "menu"
}

func insertManifestMenu(table string, moduleID string, menu ManifestMenu) error {
	menuType := strings.TrimSpace(menu.Type)
	if menuType == "" {
		menuType = "M"
	}
	title := strings.TrimSpace(menu.Title)
	if title == "" {
		title = menu.Key
	}
	i18n := strings.TrimSpace(menu.I18n)
	if i18n == "" {
		i18n = menu.Key
	}

	return insertManifestMenuRow(table, menu.ParentKey, menu.Key, map[string]any{
		"title":            title,
		"type":             menuType,
		"i18n":             i18n,
		"hidden":           0,
		"componentPath":    "modules/",
		"componentSuffix":  ".vue",
		"breadcrumbEnable": 1,
		"copyright":        1,
		"cache":            1,
		"affix":            0,
	}, menu.Path, menu.Component, menu.Sort, "manifest:"+moduleID)
}

func insertManifestPermission(table string, moduleID string, parentKey string, permission ManifestPermission, index int) error {
	title := strings.TrimSpace(permission.Description)
	if title == "" {
		title = permission.Key
	}

	return insertManifestMenuRow(table, parentKey, permission.Key, map[string]any{
		"title": title,
		"type":  "B",
		"i18n":  permission.Key,
	}, "", "", 100+index, "manifest:"+moduleID)
}

func insertManifestMenuRow(table, parentName, name string, meta map[string]any, path, component string, sort int, remark string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	return exec(`
		INSERT INTO `+table+` (
			parent_id, name, meta, path, component, redirect, status, sort,
			created_by, updated_by, created_at, updated_at, remark
		)
		VALUES (
			COALESCE((SELECT id FROM `+table+` WHERE name = ?), 0),
			?, ?::jsonb, ?, ?, '', 1, ?, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?
		)
		ON CONFLICT (name) DO UPDATE SET
			parent_id = EXCLUDED.parent_id,
			meta = EXCLUDED.meta,
			path = EXCLUDED.path,
			component = EXCLUDED.component,
			redirect = EXCLUDED.redirect,
			status = EXCLUDED.status,
			sort = EXCLUDED.sort,
			updated_at = CURRENT_TIMESTAMP,
			remark = EXCLUDED.remark
	`, parentName, name, string(encoded), path, component, sort, remark)
}

func grantManifestMenuAccess(table string, moduleID string) error {
	remark := "manifest:" + moduleID
	switch table {
	case "platform_menu":
		return grantPlatformManifestMenuAccess(remark)
	case "menu":
		return grantTenantManifestMenuAccess(remark)
	default:
		return nil
	}
}

func grantPlatformManifestMenuAccess(remark string) error {
	if err := exec(`
		INSERT INTO platform_role_belongs_menu (role_id, menu_id, created_at, updated_at)
		SELECT 1, id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM platform_menu
		WHERE remark = ?
		ON CONFLICT (role_id, menu_id) DO NOTHING
	`, remark); err != nil {
		return err
	}

	return exec(`
		INSERT INTO platform_casbin_rule (ptype, v0, v1, v2, created_at, updated_at)
		SELECT 'p', 'role:PlatformSuperAdmin', name, '*', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM platform_menu
		WHERE remark = ?
		AND NOT EXISTS (
			SELECT 1 FROM platform_casbin_rule
			WHERE ptype = 'p' AND v0 = 'role:PlatformSuperAdmin' AND v1 = platform_menu.name AND v2 = '*'
		)
	`, remark)
}

func grantTenantManifestMenuAccess(remark string) error {
	if err := exec(`
		INSERT INTO role_belongs_menu (role_id, menu_id, created_at, updated_at)
		SELECT 1, id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM menu
		WHERE remark = ?
		ON CONFLICT (role_id, menu_id) DO NOTHING
	`, remark); err != nil {
		return err
	}

	return exec(`
		INSERT INTO casbin_rule (ptype, v0, v1, v2, created_at, updated_at)
		SELECT 'p', 'role:SuperAdmin', name, '*', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM menu
		WHERE remark = ?
		AND NOT EXISTS (
			SELECT 1 FROM casbin_rule
			WHERE ptype = 'p' AND v0 = 'role:SuperAdmin' AND v1 = menu.name AND v2 = '*'
		)
	`, remark)
}

func parentMenuForPermission(permission ManifestPermission, menus []ManifestMenu) (string, bool) {
	best := ""
	for _, menu := range menus {
		if strings.HasPrefix(permission.Key, menu.Key+":") && len(menu.Key) > len(best) {
			best = menu.Key
		}
	}
	if best != "" {
		return best, true
	}

	return "", false
}
