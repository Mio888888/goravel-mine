package module

import "fmt"

type frontendManifestFixture struct {
	ExportName      string
	APIFile         string
	ViewFile        string
	LocaleFile      string
	MenuKey         string
	MenuTitle       string
	MenuPath        string
	Component       string
	Permission      string
	PermissionTitle string
}

func baseTenantFrontendManifest() frontendManifestFixture {
	return frontendManifestFixture{
		ExportName:      "baseModuleManifest",
		APIFile:         "src/modules/base/api/platformTenant.ts",
		ViewFile:        "src/modules/base/views/platform/tenant/index.vue",
		LocaleFile:      "src/modules/base/locales/zh_CN[简体中文].yaml",
		MenuKey:         "platform:tenant",
		MenuTitle:       "租户管理",
		MenuPath:        "/tenant-manage/tenant",
		Component:       "base/views/platform/tenant/index",
		Permission:      "platform:tenant:list",
		PermissionTitle: "租户列表",
	}
}

func auditLogFrontendManifest() frontendManifestFixture {
	return frontendManifestFixture{
		ExportName:      "auditLogModuleManifest",
		APIFile:         "src/modules/audit-log/api/index.ts",
		ViewFile:        "src/modules/audit-log/views/index.vue",
		LocaleFile:      "src/modules/audit-log/locales/zh_CN[简体中文].yaml",
		MenuKey:         "audit-log",
		MenuTitle:       "Audit Log",
		MenuPath:        "/audit-log",
		Component:       "audit-log/views/index",
		Permission:      "audit-log:list",
		PermissionTitle: "Audit Log 列表",
	}
}

func (fixture frontendManifestFixture) source() string {
	return fmt.Sprintf(`
export const %s = {
  apiFiles: [
    '%s',
  ],
  views: [
    '%s',
  ],
  locales: [
    '%s',
  ],
  menus: [
    { key: '%s', title: '%s', path: '%s', component: '%s', permission: '%s' },
  ],
  permissions: [
    { key: '%s', title: '%s' },
  ],
}
`, fixture.ExportName, fixture.APIFile, fixture.ViewFile, fixture.LocaleFile,
		fixture.MenuKey, fixture.MenuTitle, fixture.MenuPath, fixture.Component,
		fixture.Permission, fixture.Permission, fixture.PermissionTitle)
}
