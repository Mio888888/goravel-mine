package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"

	"goravel/app/modulecatalog"
	"goravel/app/modules"
)

func TestSeedManifestIncludesManifestManagedModuleAssets(t *testing.T) {
	manifest := modulecatalog.Manifest{Modules: []modulecatalog.ManifestItem{{
		ID: "audit-log",
		SeedStrategy: modulecatalog.ManifestSeedStrategy{
			Mode: "manifest",
		},
		Menus: []modulecatalog.ManifestMenu{{
			Key:        "audit-log",
			Path:       "/audit-log",
			Component:  "audit-log/views/index",
			Permission: "audit-log:list",
		}},
		Permissions: []modulecatalog.ManifestPermission{{
			Key: "audit-log:list",
		}},
	}}}

	seed := seedManifest(manifest)

	if !hasSeedMenu(seed.Menus, "audit-log") {
		t.Fatal("manifest-managed module menu missing from seed parity")
	}
	if !hasSeedPermission(seed.Permissions, "audit-log:list") {
		t.Fatal("manifest-managed module permission missing from seed parity")
	}
}

func TestSeedManifestExcludesUnseedableManifestPermissions(t *testing.T) {
	manifest := modulecatalog.Manifest{Modules: []modulecatalog.ManifestItem{{
		ID: "security-extra",
		SeedStrategy: modulecatalog.ManifestSeedStrategy{
			Mode: "manifest",
		},
		Menus: []modulecatalog.ManifestMenu{{
			Key: "security:ssoProvider",
		}},
		Permissions: []modulecatalog.ManifestPermission{{
			Key: "external:audit:list",
		}, {
			Key: "security:ssoProvider:list",
		}},
	}}}

	seed := seedManifest(manifest)

	if hasSeedPermission(seed.Permissions, "external:audit:list") {
		t.Fatal("unmatched manifest permission should not be treated as seeded")
	}
	if !hasSeedPermission(seed.Permissions, "security:ssoProvider:list") {
		t.Fatal("matched manifest permission missing from seed parity")
	}
}

func TestParseFrontendManifestParity(t *testing.T) {
	source := []byte(baseTenantFrontendManifest().source())

	parity, err := parseFrontendManifestParity(source)
	if err != nil {
		t.Fatalf("parseFrontendManifestParity() error = %v", err)
	}

	if !hasFrontendMenu(parity.Menus, "platform:tenant", "/tenant-manage/tenant", "base/views/platform/tenant/index", "platform:tenant:list") {
		t.Fatalf("frontend menu not parsed: %#v", parity.Menus)
	}
	if !hasSeedPermission(parity.Permissions, "platform:tenant:list") {
		t.Fatalf("frontend permission not parsed: %#v", parity.Permissions)
	}
	if !hasFrontendFile(parity.ApiFiles, "src/modules/base/api/platformTenant.ts") {
		t.Fatalf("frontend api file not parsed: %#v", parity.ApiFiles)
	}
	if !hasFrontendFile(parity.Views, "src/modules/base/views/platform/tenant/index.vue") {
		t.Fatalf("frontend view file not parsed: %#v", parity.Views)
	}
	if !hasFrontendFile(parity.Locales, "src/modules/base/locales/zh_CN[简体中文].yaml") {
		t.Fatalf("frontend locale file not parsed: %#v", parity.Locales)
	}
}

func TestParseFrontendManifestParityAllowsMultilineAndReorderedFields(t *testing.T) {
	source := []byte(`
export const baseModuleManifest = {
  apiFiles: [
    'src/modules/base/api/platformTenant.ts',
  ],
  views: [
    'src/modules/base/views/platform/tenant/index.vue',
  ],
  locales: [
    'src/modules/base/locales/zh_CN[简体中文].yaml',
  ],
  menus: [
    {
      permission: 'platform:tenant:list',
      component: 'base/views/platform/tenant/index',
      path: '/tenant-manage/tenant',
      title: '租户管理',
      key: 'platform:tenant',
    },
  ],
  permissions: [
    {
      title: '租户列表',
      key: 'platform:tenant:list',
    },
  ],
}
`)

	parity, err := parseFrontendManifestParity(source)
	if err != nil {
		t.Fatalf("parseFrontendManifestParity() error = %v", err)
	}

	if !hasFrontendMenu(parity.Menus, "platform:tenant", "/tenant-manage/tenant", "base/views/platform/tenant/index", "platform:tenant:list") {
		t.Fatalf("frontend multiline menu not parsed: %#v", parity.Menus)
	}
	if !hasSeedPermission(parity.Permissions, "platform:tenant:list") {
		t.Fatalf("frontend multiline permission not parsed: %#v", parity.Permissions)
	}
	if !hasFrontendFile(parity.ApiFiles, "src/modules/base/api/platformTenant.ts") {
		t.Fatalf("frontend multiline api file not parsed: %#v", parity.ApiFiles)
	}
}

func TestFrontendManifestRejectsEmptyParseResult(t *testing.T) {
	_, err := parseFrontendManifestParity([]byte(`export const baseModuleManifest = { menus: [], permissions: [] }`))

	if err == nil {
		t.Fatal("empty frontend manifest parse should fail")
	}
}

func TestFrontendManifestReadsAllModuleManifests(t *testing.T) {
	root := t.TempDir()
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/api/platformTenant.ts"), "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/views/platform/tenant/index.vue"), "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/locales/zh_CN[简体中文].yaml"), "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/audit-log/api/index.ts"), "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/audit-log/views/index.vue"), "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/audit-log/locales/zh_CN[简体中文].yaml"), "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/manifest.ts"), baseTenantFrontendManifest().source())
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/audit-log/manifest.ts"), auditLogFrontendManifest().source())

	parity, err := frontendManifestFromRoot(root)

	if err != nil {
		t.Fatalf("frontendManifestFromRoot() error = %v", err)
	}
	if !hasFrontendMenu(parity.Menus, "platform:tenant", "/tenant-manage/tenant", "base/views/platform/tenant/index", "platform:tenant:list") {
		t.Fatalf("base frontend menu not parsed: %#v", parity.Menus)
	}
	if !hasFrontendMenu(parity.Menus, "audit-log", "/audit-log", "audit-log/views/index", "audit-log:list") {
		t.Fatalf("module frontend menu not parsed: %#v", parity.Menus)
	}
	if !hasSeedPermission(parity.Permissions, "audit-log:list") {
		t.Fatalf("module frontend permission not parsed: %#v", parity.Permissions)
	}
	if !hasFrontendFile(parity.ApiFiles, "src/modules/audit-log/api/index.ts") {
		t.Fatalf("module frontend api file not parsed: %#v", parity.ApiFiles)
	}
}

func TestFrontendManifestRequiresDeclaredFiles(t *testing.T) {
	root := t.TempDir()
	fixture := baseTenantFrontendManifest()
	fixture.APIFile = "src/modules/base/api/missing.ts"
	fixture.ViewFile = "src/modules/base/views/missing/index.vue"
	fixture.LocaleFile = "src/modules/base/locales/missing.yaml"
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/manifest.ts"), fixture.source())

	_, err := frontendManifestFromRoot(root)

	if err == nil {
		t.Fatal("frontend manifest should require declared files")
	}
	for _, expected := range []string{
		"frontend api file does not exist: src/modules/base/api/missing.ts",
		"frontend view file does not exist: src/modules/base/views/missing/index.vue",
		"frontend locale file does not exist: src/modules/base/locales/missing.yaml",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("frontend manifest error %q missing %q", err.Error(), expected)
		}
	}
}

func TestFrontendManifestRejectsEscapedDeclaredFiles(t *testing.T) {
	root := t.TempDir()
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/views/platform/tenant/index.vue"), "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/locales/zh_CN[简体中文].yaml"), "")
	fixture := baseTenantFrontendManifest()
	fixture.APIFile = "../outside.ts"
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/manifest.ts"), fixture.source())

	_, err := frontendManifestFromRoot(root)

	if err == nil {
		t.Fatal("frontend manifest should reject escaped declared files")
	}
	if !strings.Contains(err.Error(), "frontend api file escapes frontend root: ../outside.ts") {
		t.Fatalf("frontend manifest error = %q", err.Error())
	}
}

func TestFrontendManifestRejectsAbsoluteDeclaredFilesOutsideFrontendRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.ts")
	requireWriteFile(t, outside, "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/views/platform/tenant/index.vue"), "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/locales/zh_CN[简体中文].yaml"), "")
	fixture := baseTenantFrontendManifest()
	fixture.APIFile = filepath.ToSlash(outside)
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/manifest.ts"), fixture.source())

	_, err := frontendManifestFromRoot(root)

	if err == nil {
		t.Fatal("frontend manifest should reject absolute files outside frontend root")
	}
	if !strings.Contains(err.Error(), "frontend api file escapes frontend root: "+filepath.ToSlash(outside)) {
		t.Fatalf("frontend manifest error = %q", err.Error())
	}
}

func TestFrontendManifestAcceptsMineAdminWebPrefixedFiles(t *testing.T) {
	root := t.TempDir()
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/api/platformTenant.ts"), "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/views/platform/tenant/index.vue"), "")
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/locales/zh_CN[简体中文].yaml"), "")
	fixture := baseTenantFrontendManifest()
	fixture.APIFile = "MineAdmin-web/" + fixture.APIFile
	fixture.ViewFile = "MineAdmin-web/" + fixture.ViewFile
	fixture.LocaleFile = "MineAdmin-web/" + fixture.LocaleFile
	requireWriteFile(t, filepath.Join(root, "MineAdmin-web/src/modules/base/manifest.ts"), fixture.source())

	if _, err := frontendManifestFromRoot(root); err != nil {
		t.Fatalf("frontendManifestFromRoot() error = %v", err)
	}
}

func TestFrontendManifestForCheckSkipsWhenParityDisabled(t *testing.T) {
	parity, err := frontendManifestForCheck(t.TempDir(), false)

	if err != nil {
		t.Fatalf("frontendManifestForCheck() error = %v", err)
	}
	if len(parity.Menus) != 0 || len(parity.Permissions) != 0 {
		t.Fatalf("frontend parity should be empty when disabled: %#v", parity)
	}
}

func TestFrontendManifestForCheckRequiresSourceWhenParityEnabled(t *testing.T) {
	_, err := frontendManifestForCheck(t.TempDir(), true)

	if err == nil {
		t.Fatal("frontend manifest parity should require source when enabled")
	}
}

func TestValidateManifestServiceRuntimeSkipsArtifactFileChecks(t *testing.T) {
	service := modulecatalog.NewService(modules.NewRegistry([]modules.Module{
		manifestCheckStubModule{},
	}))

	if err := validateManifestService(service, false); err != nil {
		t.Fatalf("runtime manifest check should skip missing artifact files: %v", err)
	}

	err := validateManifestService(service, true)
	if err == nil {
		t.Fatal("artifact manifest check should require declared files")
	}
}

func requireWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func hasSeedMenu(menus []modulecatalog.ManifestMenu, key string) bool {
	for _, menu := range menus {
		if menu.Key == key {
			return true
		}
	}

	return false
}

type manifestCheckStubModule struct{}

func (m manifestCheckStubModule) ID() string { return "manifest-check-stub" }
func (m manifestCheckStubModule) Routes() []modules.Route {
	return []modules.Route{{
		Name:       "manifest-check-stub.list",
		Method:     "GET",
		Path:       "/admin/manifest-check-stub/list",
		Permission: "manifest-check-stub:list",
		Install:    func() {},
	}}
}
func (m manifestCheckStubModule) Menus() []modules.Menu {
	return []modules.Menu{{
		Key:        "manifest-check-stub",
		Path:       "/manifest-check-stub",
		Component:  "manifest-check-stub/views/index",
		Permission: "manifest-check-stub:list",
	}}
}
func (m manifestCheckStubModule) Permissions() []modules.Permission {
	return []modules.Permission{{Key: "manifest-check-stub:list"}}
}
func (m manifestCheckStubModule) Migrations() []schema.Migration { return nil }
func (m manifestCheckStubModule) Seeders() []seeder.Seeder       { return nil }
func (m manifestCheckStubModule) OpenAPIFiles() []string {
	return []string{"docs/api-contract/openapi/missing-stub.openapi.json"}
}
func (m manifestCheckStubModule) TestTemplates() []string {
	return []string{"tests/feature/admin/missing_stub_test.go"}
}

func hasFrontendMenu(menus []modulecatalog.ManifestMenu, key, path, component, permission string) bool {
	for _, menu := range menus {
		if menu.Key == key && menu.Path == path && menu.Component == component && menu.Permission == permission {
			return true
		}
	}

	return false
}

func hasSeedPermission(permissions []modulecatalog.ManifestPermission, key string) bool {
	for _, permission := range permissions {
		if permission.Key == key {
			return true
		}
	}

	return false
}

func hasFrontendFile(files []string, path string) bool {
	for _, file := range files {
		if file == path {
			return true
		}
	}

	return false
}
