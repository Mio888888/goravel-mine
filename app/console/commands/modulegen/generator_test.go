package modulegen

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"

	"goravel/app/modules"
	"goravel/database/seeders"
)

func TestGeneratorCreatesModuleScaffold(t *testing.T) {
	root := t.TempDir()

	err := NewGenerator(root).Generate(Options{Name: "audit-log"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	expected := []string{
		"app/modules/auditlog/module.go",
		"app/modules/auditlog/model.go",
		"app/modules/auditlog/migration.go",
		"app/modules/auditlog/repository.go",
		"app/modules/auditlog/service.go",
		"docs/api-contract/openapi/audit-log.openapi.json",
		"tests/feature/admin/audit_log_test.go",
		"docs/framework/modules/audit-log/README.md",
		"MineAdmin-web/src/modules/audit-log/api/index.ts",
		"MineAdmin-web/src/modules/audit-log/manifest.ts",
		"MineAdmin-web/src/modules/audit-log/locales/zh_CN.yaml",
		"MineAdmin-web/src/modules/audit-log/types/index.ts",
		"MineAdmin-web/src/modules/audit-log/views/index.vue",
		"MineAdmin-web/tests/e2e/audit-log.spec.ts",
	}
	for _, path := range expected {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("generated file missing %s: %v", path, err)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(root, "MineAdmin-web/src/modules/audit-log/manifest.ts"))
	if err != nil {
		t.Fatalf("read frontend manifest: %v", err)
	}
	for _, expected := range []string{
		`id: 'audit-log'`,
		`key: 'audit-log'`,
		`path: '/audit-log'`,
		`component: 'audit-log/views/index'`,
		`permission: 'audit-log:list'`,
		`key: 'audit-log:delete'`,
	} {
		if !strings.Contains(string(manifest), expected) {
			t.Fatalf("frontend manifest missing %q:\n%s", expected, string(manifest))
		}
	}

	moduleGo, err := os.ReadFile(filepath.Join(root, "app/modules/auditlog/module.go"))
	if err != nil {
		t.Fatalf("read module.go: %v", err)
	}
	content := string(moduleGo)
	for _, expected := range []string{
		`func (m Module) ID() string`,
		`return "audit-log"`,
		`modules.BuiltinMetadata("Audit Log")`,
		`metadata.Lifecycle`,
		`Install:              "go run . artisan tenant:migrate && go run . artisan db:seed"`,
		`Upgrade:              "go run . artisan tenant:migrate"`,
		`Rollback:             "manual tenant migration rollback required"`,
		`metadata.SeedStrategy`,
		`seeders.NewManifestMenuSeeder`,
		`audit-log:list`,
		`audit-log:save`,
		`modules.InstallTenantRoute`,
		`modules.InstallTenantAuditRoute`,
		`[]string{"tenant-rbac"}`,
		`[]string{"tenant-rbac-audit"}`,
		`NewService().WithContext(ctx.Context()).List`,
		`NewService().WithContext(ctx.Context()).Create`,
		`NewService().WithContext(ctx.Context()).Update`,
		`NewService().WithContext(ctx.Context()).Delete`,
		`func list(ctx contractshttp.Context) contractshttp.Response`,
		`audit-log.index`,
		`func (m Module) TenantMigrations() []schema.Migration`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("module.go missing %q:\n%s", expected, content)
		}
	}
	if strings.Contains(content, `Routes() []modules.Route {
	return nil
}`) {
		t.Fatalf("module.go still generates nil routes:\n%s", content)
	}

	readme, err := os.ReadFile(filepath.Join(root, "docs/framework/modules/audit-log/README.md"))
	if err != nil {
		t.Fatalf("read module README: %v", err)
	}
	if !strings.Contains(string(readme), "module:plan --action=install") {
		t.Fatalf("README missing lifecycle command:\n%s", string(readme))
	}

	frontendAPI, err := os.ReadFile(filepath.Join(root, "MineAdmin-web/src/modules/audit-log/api/index.ts"))
	if err != nil {
		t.Fatalf("read frontend api: %v", err)
	}
	for _, expected := range []string{
		`import type { AuditLogItem } from '../types'`,
		`listAuditLog(params?: Record<string, any>)`,
		`return useHttp().get('/admin/audit-log/list', { params })`,
		`createAuditLog(data: Partial<AuditLogItem>)`,
		`return useHttp().post('/admin/audit-log', data)`,
		`updateAuditLog(id: number, data: Partial<AuditLogItem>)`,
		`return useHttp().put('/admin/audit-log/' + id, data)`,
		`deleteAuditLog(ids: number[])`,
		`return useHttp().delete('/admin/audit-log', { data: ids })`,
	} {
		if !strings.Contains(string(frontendAPI), expected) {
			t.Fatalf("frontend api missing %q:\n%s", expected, string(frontendAPI))
		}
	}

	openAPI, err := os.ReadFile(filepath.Join(root, "docs/api-contract/openapi/audit-log.openapi.json"))
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	var openAPIDocument map[string]any
	if err := json.Unmarshal(openAPI, &openAPIDocument); err != nil {
		t.Fatalf("openapi should be valid JSON: %v\n%s", err, string(openAPI))
	}
	for _, expected := range []string{
		`"/admin/audit-log/list"`,
		`"get"`,
		`"/admin/audit-log"`,
		`"post"`,
		`"delete"`,
		`"/admin/audit-log/{id}"`,
		`"put"`,
		`"operationId": "adminAuditLogCreate"`,
		`"operationId": "adminAuditLogUpdate"`,
		`"operationId": "adminAuditLogDelete"`,
	} {
		if !strings.Contains(string(openAPI), expected) {
			t.Fatalf("openapi missing %q:\n%s", expected, string(openAPI))
		}
	}

	for path, expected := range map[string][]string{
		"app/modules/auditlog/model.go": {
			`type AuditLog struct`,
			`func (AuditLog) TableName() string`,
			`return "audit_log_item"`,
		},
		"app/modules/auditlog/migration.go": {
			`type Migration struct{}`,
			`func (r Migration) Signature() string`,
			`Create("audit_log_item"`,
			`table.String("name", 120)`,
		},
		"app/modules/auditlog/repository.go": {
			`type Repository struct`,
			`var ErrTenantContextRequired = errors.New("tenant context required")`,
			`func (r *Repository) query() (contractsorm.Query, error)`,
			`connection := services.TenantConnectionFromContext(ctx)`,
			`return nil, ErrTenantContextRequired`,
			`services.OrmForConnectionWithContext(ctx, connection)`,
			`func (r *Repository) List`,
			`query, err := r.query()`,
			`query = query.Where("name LIKE ?", "%"+name+"%")`,
			`func (r *Repository) Create`,
			`func (r *Repository) Update`,
			`func (r *Repository) Delete`,
			`func uint64Any(values []uint64) []any`,
			`WhereIn("id", uint64Any(ids))`,
			`if result.RowsAffected == 0`,
			`services.BusinessError{Message: "记录不存在"}`,
		},
		"app/modules/auditlog/service.go": {
			`type Service struct`,
			`type Payload struct`,
			`func (s *Service) List`,
			`func (s *Service) Create`,
			`func (s *Service) Update`,
			`func (s *Service) Delete`,
		},
		"tests/feature/admin/audit_log_test.go": {
			`"context"`,
			`"goravel/tests"`,
			`"goravel/app/facades"`,
			`"goravel/app/services"`,
			`type AuditLogModuleTestSuite struct`,
			`tests.TestCase`,
			`func (s *AuditLogModuleTestSuite) SetupTest()`,
			`s.RefreshDatabase()`,
			`auditlog.Migration{}.Up()`,
			`TestAuditLogModuleCRUD`,
			`ctx := s.tenantContext()`,
			`auditlog.NewService().WithContext(ctx)`,
			`service.Create`,
			`service.List`,
			`service.Update`,
			`service.Delete`,
			`func (s *AuditLogModuleTestSuite) tenantContext() context.Context`,
			`services.RegisterTenantConnection(tenant)`,
			`services.WithTenant(context.Background(), tenant)`,
		},
	} {
		payload, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, item := range expected {
			if !strings.Contains(string(payload), item) {
				t.Fatalf("%s missing %q:\n%s", path, item, string(payload))
			}
		}
	}
}

func TestGeneratedModuleExampleIsRuntimeValidBeforeControllerWiring(t *testing.T) {
	module := generatedModuleFixture{}
	registry := modules.NewRegistry([]modules.Module{module})

	if err := registry.ValidateRuntime(); err != nil {
		t.Fatalf("generated module example should be runtime valid before controller wiring: %v", err)
	}
}

func TestGeneratedModuleExampleHasManifestSeeder(t *testing.T) {
	module := generatedModuleFixture{}
	registry := modules.NewRegistry([]modules.Module{module})

	seeders := registry.Seeders()

	if len(seeders) != 1 {
		t.Fatalf("len(Seeders()) = %d, want 1", len(seeders))
	}
	if got := seeders[0].Signature(); got != "audit_log_manifest_menu_seed" {
		t.Fatalf("Seeder signature = %q", got)
	}
}

func TestGeneratedModuleExampleRollbackRequiresManualTenantReview(t *testing.T) {
	module := generatedModuleFixture{}
	command := module.Metadata().Lifecycle.Rollback

	if command != "manual tenant migration rollback required" {
		t.Fatalf("rollback command = %q, want manual tenant rollback", command)
	}
}

type generatedModuleFixture struct{}

func (m generatedModuleFixture) ID() string {
	return "audit-log"
}

func (m generatedModuleFixture) Metadata() modules.Metadata {
	metadata := modules.BuiltinMetadata("Audit Log")
	metadata.Lifecycle = modules.Lifecycle{
		Install:              "go run . artisan migrate && go run . artisan db:seed",
		Uninstall:            "manual data review required",
		Upgrade:              "go run . artisan migrate",
		Rollback:             "manual tenant migration rollback required",
		DestructiveCheck:     "go run . artisan module:manifest:check",
		BreakingChangePolicy: "document in release notes and block without review",
	}
	metadata.SeedStrategy = modules.SeedStrategy{
		Mode:       "manifest",
		Idempotent: true,
		Command:    "go run . artisan db:seed",
		Notes:      "menus and permissions are seeded from module manifest",
	}
	metadata.Frontend = modules.FrontendArtifact{
		ModulePath: "MineAdmin-web/src/modules/audit-log",
		ApiFiles: []string{
			"MineAdmin-web/src/modules/audit-log/api/index.ts",
		},
		RouteFiles: []string{
			"MineAdmin-web/src/modules/audit-log/views/index.vue",
		},
		TypeFiles: []string{
			"MineAdmin-web/src/modules/audit-log/types/index.ts",
		},
		TestFiles: []string{
			"tests/feature/admin/audit_log_test.go",
			"MineAdmin-web/tests/e2e/audit-log.spec.ts",
		},
	}
	return metadata
}

func (m generatedModuleFixture) Routes() []modules.Route {
	return []modules.Route{{
		Name:       "audit-log.list",
		Method:     "GET",
		Path:       "/admin/audit-log/list",
		Permission: "audit-log:list",
		Install:    func() {},
	}}
}

func (m generatedModuleFixture) Menus() []modules.Menu {
	return []modules.Menu{{
		Key:        "audit-log",
		Title:      "Audit Log",
		Path:       "/audit-log",
		Component:  "audit-log/views/index",
		Permission: "audit-log:list",
		Type:       "M",
		I18n:       "audit-log.index",
		Sort:       100,
	}}
}

func (m generatedModuleFixture) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "audit-log:list", Description: "Audit Log 列表"},
		{Key: "audit-log:save", Description: "Audit Log 创建"},
		{Key: "audit-log:update", Description: "Audit Log 更新"},
		{Key: "audit-log:delete", Description: "Audit Log 删除"},
	}
}

func (m generatedModuleFixture) Migrations() []schema.Migration {
	return nil
}

func (m generatedModuleFixture) Seeders() []seeder.Seeder {
	return []seeder.Seeder{
		seeders.NewManifestMenuSeeder(m.ID(), m.seedMenus(), m.seedPermissions()),
	}
}

func (m generatedModuleFixture) seedMenus() []seeders.ManifestMenu {
	items := make([]seeders.ManifestMenu, 0, len(m.Menus()))
	for _, menu := range m.Menus() {
		items = append(items, seeders.ManifestMenu{
			Key:        menu.Key,
			ParentKey:  menu.ParentKey,
			Title:      menu.Title,
			Path:       menu.Path,
			Component:  menu.Component,
			Permission: menu.Permission,
			Type:       menu.Type,
			I18n:       menu.I18n,
			Sort:       menu.Sort,
		})
	}

	return items
}

func (m generatedModuleFixture) seedPermissions() []seeders.ManifestPermission {
	items := make([]seeders.ManifestPermission, 0, len(m.Permissions()))
	for _, permission := range m.Permissions() {
		items = append(items, seeders.ManifestPermission{
			Key:         permission.Key,
			Description: permission.Description,
		})
	}

	return items
}

func (m generatedModuleFixture) OpenAPIFiles() []string {
	return []string{"docs/api-contract/openapi/audit-log.openapi.json"}
}

func (m generatedModuleFixture) TestTemplates() []string {
	return []string{"tests/feature/admin/audit_log_test.go"}
}

func TestGeneratorRejectsExistingFilesUnlessForced(t *testing.T) {
	root := t.TempDir()
	generator := NewGenerator(root)

	if err := generator.Generate(Options{Name: "audit-log"}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	err := generator.Generate(Options{Name: "audit-log"})
	if !errors.Is(err, ErrFileExists) {
		t.Fatalf("Generate() error = %v, want ErrFileExists", err)
	}
	if err := generator.Generate(Options{Name: "audit-log", Force: true}); err != nil {
		t.Fatalf("Generate(force) error = %v", err)
	}
}

func TestGeneratorValidatesModuleName(t *testing.T) {
	for _, name := range []string{"../bad", "audit-", "audit--log"} {
		err := NewGenerator(t.TempDir()).Generate(Options{Name: name})
		if err == nil {
			t.Fatalf("Generate(%q) expected invalid module name error", name)
		}
	}
}
