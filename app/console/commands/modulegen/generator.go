package modulegen

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

var ErrFileExists = errors.New("generated file already exists")
var moduleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

type Options struct {
	Name  string
	Force bool
}

type Generator struct {
	root string
}

type moduleData struct {
	Slug        string
	PackageName string
	Title       string
	Snake       string
	Camel       string
	Pascal      string
}

type generatedFile struct {
	path    string
	content string
	goFile  bool
}

func NewGenerator(root string) *Generator {
	return &Generator{root: root}
}

func (g *Generator) Generate(opts Options) error {
	data, err := normalize(opts.Name)
	if err != nil {
		return err
	}
	files, err := renderFiles(data)
	if err != nil {
		return err
	}
	if err := g.ensureWritable(files, opts.Force); err != nil {
		return err
	}
	return g.writeFiles(files)
}

func normalize(name string) (moduleData, error) {
	slug := strings.TrimSpace(strings.ToLower(name))
	slug = strings.ReplaceAll(slug, "_", "-")
	if slug == "" {
		return moduleData{}, errors.New("module name is required")
	}
	if !moduleNamePattern.MatchString(slug) {
		return moduleData{}, fmt.Errorf("module name must match ^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$: %s", name)
	}
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	pascal := strings.Join(parts, "")

	return moduleData{
		Slug:        slug,
		PackageName: strings.ReplaceAll(slug, "-", ""),
		Title:       strings.Join(parts, " "),
		Snake:       strings.ReplaceAll(slug, "-", "_"),
		Camel:       strings.ToLower(pascal[:1]) + pascal[1:],
		Pascal:      pascal,
	}, nil
}

func renderFiles(data moduleData) ([]generatedFile, error) {
	files := []generatedFile{
		{path: "app/modules/" + data.PackageName + "/module.go", content: moduleTemplate, goFile: true},
		{path: "app/modules/" + data.PackageName + "/model.go", content: modelTemplate, goFile: true},
		{path: "app/modules/" + data.PackageName + "/migration.go", content: migrationTemplate, goFile: true},
		{path: "app/modules/" + data.PackageName + "/repository.go", content: repositoryTemplate, goFile: true},
		{path: "app/modules/" + data.PackageName + "/service.go", content: serviceTemplate, goFile: true},
		{path: "docs/api-contract/openapi/" + data.Slug + ".openapi.json", content: openAPITemplate},
		{path: "docs/framework/modules/" + data.Slug + "/README.md", content: moduleReadmeTemplate},
		{path: "tests/feature/admin/" + data.Snake + "_test.go", content: testTemplate, goFile: true},
		{path: "MineAdmin-web/src/modules/" + data.Slug + "/api/index.ts", content: frontendAPITemplate},
		{path: "MineAdmin-web/src/modules/" + data.Slug + "/manifest.ts", content: frontendManifestTemplate},
		{path: "MineAdmin-web/src/modules/" + data.Slug + "/locales/zh_CN.yaml", content: frontendLocaleTemplate},
		{path: "MineAdmin-web/src/modules/" + data.Slug + "/types/index.ts", content: frontendTypesTemplate},
		{path: "MineAdmin-web/src/modules/" + data.Slug + "/views/index.vue", content: frontendViewTemplate},
		{path: "MineAdmin-web/tests/e2e/" + data.Slug + ".spec.ts", content: frontendE2ETemplate},
	}
	for i := range files {
		content, err := executeTemplate(files[i].content, data)
		if err != nil {
			return nil, err
		}
		if files[i].goFile {
			content, err = formatGo(content)
			if err != nil {
				return nil, err
			}
		}
		files[i].content = content
	}

	return files, nil
}

func executeTemplate(source string, data moduleData) (string, error) {
	tpl, err := template.New("module").Parse(source)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func formatGo(content string) (string, error) {
	out, err := format.Source([]byte(content))
	if err != nil {
		return "", fmt.Errorf("format generated go: %w", err)
	}
	return string(out), nil
}

func (g *Generator) ensureWritable(files []generatedFile, force bool) error {
	if force {
		return nil
	}
	for _, file := range files {
		if _, err := os.Stat(filepath.Join(g.root, file.path)); err == nil {
			return fmt.Errorf("%w: %s", ErrFileExists, file.path)
		}
	}
	return nil
}

func (g *Generator) writeFiles(files []generatedFile) error {
	for _, file := range files {
		path := filepath.Join(g.root, file.path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(file.content), 0644); err != nil {
			return err
		}
	}
	return nil
}

const moduleTemplate = `package {{.PackageName}}

import (
	"net/http"

	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/request"
	"goravel/app/http/response"
	"goravel/app/modules"
	"goravel/database/seeders"
)

type Module struct{}

func New() Module {
	return Module{}
}

func (m Module) ID() string {
	return "{{.Slug}}"
}

func (m Module) Metadata() modules.Metadata {
	metadata := modules.BuiltinMetadata("{{.Title}}")
	metadata.Lifecycle = modules.Lifecycle{
		Install:              "go run . artisan tenant:migrate && go run . artisan db:seed",
		Uninstall:            "manual data review required",
		Upgrade:              "go run . artisan tenant:migrate",
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
		ModulePath: "MineAdmin-web/src/modules/{{.Slug}}",
		ApiFiles: []string{
			"MineAdmin-web/src/modules/{{.Slug}}/api/index.ts",
		},
		RouteFiles: []string{
			"MineAdmin-web/src/modules/{{.Slug}}/views/index.vue",
		},
		LocaleFiles: []string{
			"MineAdmin-web/src/modules/{{.Slug}}/locales/zh_CN.yaml",
		},
		TypeFiles: []string{
			"MineAdmin-web/src/modules/{{.Slug}}/types/index.ts",
		},
		TestFiles: []string{
			"tests/feature/admin/{{.Snake}}_test.go",
			"MineAdmin-web/tests/e2e/{{.Slug}}.spec.ts",
		},
	}
	return metadata
}

func (m Module) Package() modules.Package {
	return modules.BuiltinPackage(m.ID(), "module-owner")
}

func (m Module) Routes() []modules.Route {
	return []modules.Route{
		{
			Name:       "{{.Slug}}.list",
			Method:     "GET",
			Path:       "/admin/{{.Slug}}/list",
			Permission: "{{.Slug}}:list",
			Middlewares: []string{"tenant-rbac"},
			Install:    modules.InstallTenantRoute("GET", "/admin/{{.Slug}}/list", list),
		},
		{
			Name:       "{{.Slug}}.create",
			Method:     "POST",
			Path:       "/admin/{{.Slug}}",
			Permission: "{{.Slug}}:save",
			Middlewares: []string{"tenant-rbac-audit"},
			Install:    modules.InstallTenantAuditRoute("POST", "/admin/{{.Slug}}", create),
		},
		{
			Name:       "{{.Slug}}.update",
			Method:     "PUT",
			Path:       "/admin/{{.Slug}}/{id}",
			Permission: "{{.Slug}}:update",
			Middlewares: []string{"tenant-rbac-audit"},
			Install:    modules.InstallTenantAuditRoute("PUT", "/admin/{{.Slug}}/{id}", update),
		},
		{
			Name:       "{{.Slug}}.delete",
			Method:     "DELETE",
			Path:       "/admin/{{.Slug}}",
			Permission: "{{.Slug}}:delete",
			Middlewares: []string{"tenant-rbac-audit"},
			Install:    modules.InstallTenantAuditRoute("DELETE", "/admin/{{.Slug}}", deleteItems),
		},
	}
}

func (m Module) Menus() []modules.Menu {
	return []modules.Menu{
		{
			Key:        "{{.Slug}}",
			Title:      "{{.Title}}",
			Path:       "/{{.Slug}}",
			Component:  "{{.Slug}}/views/index",
			Permission: "{{.Slug}}:list",
			Type:       "M",
			I18n:       "{{.Slug}}.index",
			Sort:       100,
		},
	}
}

func (m Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "{{.Slug}}:list", Description: "{{.Title}} 列表"},
		{Key: "{{.Slug}}:save", Description: "{{.Title}} 创建"},
		{Key: "{{.Slug}}:update", Description: "{{.Title}} 更新"},
		{Key: "{{.Slug}}:delete", Description: "{{.Title}} 删除"},
	}
}

func (m Module) Migrations() []schema.Migration {
	return nil
}

func (m Module) TenantMigrations() []schema.Migration {
	return []schema.Migration{Migration{}}
}

func (m Module) Seeders() []seeder.Seeder {
	return []seeder.Seeder{
		seeders.NewManifestMenuSeeder(m.ID(), m.seedMenus(), m.seedPermissions()),
	}
}

func (m Module) seedMenus() []seeders.ManifestMenu {
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

func (m Module) seedPermissions() []seeders.ManifestPermission {
	items := make([]seeders.ManifestPermission, 0, len(m.Permissions()))
	for _, permission := range m.Permissions() {
		items = append(items, seeders.ManifestPermission{
			Key:         permission.Key,
			Description: permission.Description,
		})
	}

	return items
}

func (m Module) OpenAPIFiles() []string {
	return []string{"docs/api-contract/openapi/{{.Slug}}.openapi.json"}
}

func (m Module) TestTemplates() []string {
	return []string{"tests/feature/admin/{{.Snake}}_test.go"}
}

func list(ctx contractshttp.Context) contractshttp.Response {
	result, err := NewService().WithContext(ctx.Context()).List(ctx.Request().Query("name"), ctx.Request().QueryInt("page", 1), ctx.Request().QueryInt("page_size", 15))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}
	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func create(ctx contractshttp.Context) contractshttp.Response {
	var payload Payload
	if err := ctx.Request().Bind(&payload); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	item, err := NewService().WithContext(ctx.Context()).Create(payload)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}
	return ctx.Response().Json(http.StatusOK, response.Success(item))
}

func update(ctx contractshttp.Context) contractshttp.Response {
	id := ctx.Request().RouteInt("id")
	var payload Payload
	if err := ctx.Request().Bind(&payload); err != nil || id <= 0 {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	item, err := NewService().WithContext(ctx.Context()).Update(uint64(id), payload)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}
	return ctx.Response().Json(http.StatusOK, response.Success(item))
}

func deleteItems(ctx contractshttp.Context) contractshttp.Response {
	var ids []uint64
	if err := ctx.Request().Bind(&ids); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	if err := NewService().WithContext(ctx.Context()).Delete(ids); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}
	return ctx.Response().Json(http.StatusOK, response.SuccessEmpty())
}
`

const modelTemplate = `package {{.PackageName}}

import "time"

type {{.Pascal}} struct {
	ID        uint64     ` + "`gorm:\"column:id;primaryKey\" json:\"id\"`" + `
	Name      string     ` + "`gorm:\"column:name\" json:\"name\"`" + `
	Status    int8       ` + "`gorm:\"column:status\" json:\"status\"`" + `
	Remark    string     ` + "`gorm:\"column:remark\" json:\"remark\"`" + `
	CreatedAt *time.Time ` + "`gorm:\"column:created_at\" json:\"created_at\"`" + `
	UpdatedAt *time.Time ` + "`gorm:\"column:updated_at\" json:\"updated_at\"`" + `
}

func ({{.Pascal}}) TableName() string {
	return "{{.Snake}}_item"
}
`

const migrationTemplate = `package {{.PackageName}}

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type Migration struct{}

func (r Migration) Signature() string {
	return "create_{{.Snake}}_item_table"
}

func (r Migration) Up() error {
	if facades.Schema().HasTable("{{.Snake}}_item") {
		return nil
	}
	return facades.Schema().Create("{{.Snake}}_item", func(table schema.Blueprint) {
		table.ID()
		table.String("name", 120)
		table.TinyInteger("status").Default(1)
		table.String("remark", 255).Default("")
		table.Timestamp("created_at").Nullable()
		table.Timestamp("updated_at").Nullable()
		table.Index("status")
	})
}

func (r Migration) Down() error {
	return facades.Schema().DropIfExists("{{.Snake}}_item")
}
`

const repositoryTemplate = `package {{.PackageName}}

import (
	"context"
	"errors"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/http/request"
	"goravel/app/services"
)

type Repository struct {
	ctx context.Context
}

var ErrTenantContextRequired = errors.New("tenant context required")

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) WithContext(ctx context.Context) *Repository {
	clone := *r
	if ctx != nil {
		clone.ctx = ctx
	}
	return &clone
}

func (r *Repository) query() (contractsorm.Query, error) {
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	connection := services.TenantConnectionFromContext(ctx)
	if connection == "" {
		return nil, ErrTenantContextRequired
	}
	return services.OrmForConnectionWithContext(ctx, connection).Query().Table({{.Pascal}}{}.TableName()), nil
}

func (r *Repository) List(name string, page int, pageSize int) (request.PageResult[{{.Pascal}}], error) {
	query, err := r.query()
	if err != nil {
		return request.PageResult[{{.Pascal}}]{}, err
	}
	if name = strings.TrimSpace(name); name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}
	return request.Paginate[{{.Pascal}}](query.OrderByDesc("id"), page, pageSize)
}

func (r *Repository) Create(item {{.Pascal}}) ({{.Pascal}}, error) {
	query, err := r.query()
	if err != nil {
		return item, err
	}
	item.CreatedAt = ptrTime(time.Now())
	item.UpdatedAt = item.CreatedAt
	err = query.Create(&item)
	return item, err
}

func (r *Repository) Update(id uint64, item {{.Pascal}}) ({{.Pascal}}, error) {
	query, err := r.query()
	if err != nil {
		return {{.Pascal}}{}, err
	}
	values := map[string]any{
		"name":       item.Name,
		"status":     item.Status,
		"remark":     item.Remark,
		"updated_at": time.Now(),
	}
	result, err := query.Where("id", id).Update(values)
	if err != nil {
		return {{.Pascal}}{}, err
	}
	if result.RowsAffected == 0 {
		return {{.Pascal}}{}, services.BusinessError{Message: "记录不存在"}
	}
	item.ID = id
	return item, nil
}

func (r *Repository) Delete(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	query, err := r.query()
	if err != nil {
		return err
	}
	_, err = query.WhereIn("id", uint64Any(ids)).Delete()
	return err
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func uint64Any(values []uint64) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return items
}
`

const serviceTemplate = `package {{.PackageName}}

import (
	"context"
	"strings"

	"goravel/app/http/request"
)

type Service struct {
	ctx        context.Context
	repository *Repository
}

type Payload struct {
	Name   string ` + "`json:\"name\"`" + `
	Status int8   ` + "`json:\"status\"`" + `
	Remark string ` + "`json:\"remark\"`" + `
}

func NewService() *Service {
	return &Service{repository: NewRepository()}
}

func (s *Service) WithContext(ctx context.Context) *Service {
	clone := *s
	clone.ctx = ctx
	clone.repository = s.repository.WithContext(ctx)
	return &clone
}

func (s *Service) List(name string, page int, pageSize int) (request.PageResult[{{.Pascal}}], error) {
	return s.repository.List(name, page, pageSize)
}

func (s *Service) Create(payload Payload) ({{.Pascal}}, error) {
	return s.repository.Create(payload.Model())
}

func (s *Service) Update(id uint64, payload Payload) ({{.Pascal}}, error) {
	return s.repository.Update(id, payload.Model())
}

func (s *Service) Delete(ids []uint64) error {
	return s.repository.Delete(ids)
}

func (p Payload) Model() {{.Pascal}} {
	status := p.Status
	if status == 0 {
		status = 1
	}
	return {{.Pascal}}{
		Name:   strings.TrimSpace(p.Name),
		Status: status,
		Remark: strings.TrimSpace(p.Remark),
	}
}
`

const openAPITemplate = `{
  "openapi": "3.1.0",
  "info": {
    "title": "{{.Title}} API",
    "version": "0.1.0"
  },
  "paths": {
    "/admin/{{.Slug}}/list": {
      "get": {
        "tags": ["{{.Title}}"],
        "operationId": "admin{{.Pascal}}List",
        "responses": {
          "200": {
            "description": "OK"
          }
        }
      }
    },
    "/admin/{{.Slug}}": {
      "post": {
        "tags": ["{{.Title}}"],
        "operationId": "admin{{.Pascal}}Create",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/{{.Pascal}}Payload"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "OK"
          }
        }
      },
      "delete": {
        "tags": ["{{.Title}}"],
        "operationId": "admin{{.Pascal}}Delete",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "array",
                "items": {
                  "type": "integer"
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "OK"
          }
        }
      }
    },
    "/admin/{{.Slug}}/{id}": {
      "put": {
        "tags": ["{{.Title}}"],
        "operationId": "admin{{.Pascal}}Update",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "schema": {
              "type": "integer"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/{{.Pascal}}Payload"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "OK"
          }
        }
      }
    }
  },
  "components": {
    "schemas": {
      "{{.Pascal}}Payload": {
        "type": "object",
        "additionalProperties": true
      }
    }
  }
}
`

const testTemplate = `package admin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/modules"
	"goravel/app/modules/{{.PackageName}}"
	"goravel/app/services"
	"goravel/tests"
)

type {{.Pascal}}ModuleTestSuite struct {
	suite.Suite
	tests.TestCase
}

func Test{{.Pascal}}ModuleTestSuite(t *testing.T) {
	suite.Run(t, new({{.Pascal}}ModuleTestSuite))
}

func (s *{{.Pascal}}ModuleTestSuite) SetupTest() {
	s.RefreshDatabase()
	require.NoError(s.T(), {{.PackageName}}.Migration{}.Up())
}

func (s *{{.Pascal}}ModuleTestSuite) Test{{.Pascal}}ModuleContract() {
	module := {{.PackageName}}.New()
	registry := modules.NewRegistry([]modules.Module{module})

	if err := registry.ValidateRuntime(); err != nil {
		s.T().Fatalf("generated module should be runtime valid: %v", err)
	}

	if got := len(module.Routes()); got != 4 {
		s.T().Fatalf("len(Routes()) = %d, want 4", got)
	}
	for _, permission := range []string{"{{.Slug}}:list", "{{.Slug}}:save", "{{.Slug}}:update", "{{.Slug}}:delete"} {
		if !has{{.Pascal}}Permission(module.Permissions(), permission) {
			s.T().Fatalf("permission %s missing", permission)
		}
	}
}

func (s *{{.Pascal}}ModuleTestSuite) Test{{.Pascal}}ModuleCRUD() {
	ctx := s.tenantContext()
	service := {{.PackageName}}.NewService().WithContext(ctx)
	created, err := service.Create({{.PackageName}}.Payload{Name: "first", Remark: "created"})
	if err != nil {
		s.T().Fatalf("Create() error = %v", err)
	}
	if created.Name != "first" || created.Status != 1 {
		s.T().Fatalf("created item = %#v", created)
	}

	page, err := service.List("first", 1, 15)
	if err != nil {
		s.T().Fatalf("List() error = %v", err)
	}
	if page.Total < 1 {
		s.T().Fatalf("List().Total = %d, want at least 1", page.Total)
	}

	updated, err := service.Update(created.ID, {{.PackageName}}.Payload{Name: "second", Status: 2})
	if err != nil {
		s.T().Fatalf("Update() error = %v", err)
	}
	if updated.Name != "second" || updated.Status != 2 {
		s.T().Fatalf("updated item = %#v", updated)
	}

	if err := service.Delete([]uint64{created.ID}); err != nil {
		s.T().Fatalf("Delete() error = %v", err)
	}
}

func (s *{{.Pascal}}ModuleTestSuite) tenantContext() context.Context {
	connection := facades.Config().GetString("database.default")
	tenant := services.Tenant{
		ID:         1,
		Code:       "{{.Snake}}",
		Name:       "{{.Title}} Test",
		Status:     services.TenantStatusActive,
		DBHost:     facades.Config().GetString("database.connections." + connection + ".host"),
		DBPort:     facades.Config().GetInt("database.connections."+connection+".port", 5432),
		DBDatabase: facades.Config().GetString("database.connections." + connection + ".database"),
		DBUsername: facades.Config().GetString("database.connections." + connection + ".username"),
		DBPassword: facades.Config().GetString("database.connections." + connection + ".password"),
		DBSchema:   facades.Config().GetString("database.connections."+connection+".schema", "public"),
	}
	require.NotEmptyf(s.T(), tenant.DBDatabase, "database for %s is required", connection)
	services.RegisterTenantConnection(tenant)
	return services.WithTenant(context.Background(), tenant)
}

func has{{.Pascal}}Permission(items []modules.Permission, key string) bool {
	for _, item := range items {
		if item.Key == key {
			return true
		}
	}
	return false
}
`

const frontendAPITemplate = `import type { {{.Pascal}}Item } from '../types'

export function list{{.Pascal}}(params?: Record<string, any>) {
  return useHttp().get('/admin/{{.Slug}}/list', { params })
}

export function create{{.Pascal}}(data: Partial<{{.Pascal}}Item>) {
  return useHttp().post('/admin/{{.Slug}}', data)
}

export function update{{.Pascal}}(id: number, data: Partial<{{.Pascal}}Item>) {
  return useHttp().put('/admin/{{.Slug}}/' + id, data)
}

export function delete{{.Pascal}}(ids: number[]) {
  return useHttp().delete('/admin/{{.Slug}}', { data: ids })
}
`

const frontendManifestTemplate = `import type { MineModuleManifest } from '../base/manifest'

export const {{.Camel}}ModuleManifest = {
  id: '{{.Slug}}',
  apiFiles: [
    'src/modules/{{.Slug}}/api/index.ts',
  ],
  views: [
    'src/modules/{{.Slug}}/views/index.vue',
  ],
  locales: [
    'src/modules/{{.Slug}}/locales/zh_CN.yaml',
  ],
  menus: [
    { key: '{{.Slug}}', title: '{{.Title}}', path: '/{{.Slug}}', component: '{{.Slug}}/views/index', permission: '{{.Slug}}:list' },
  ],
  permissions: [
    { key: '{{.Slug}}:list', title: '{{.Title}} 列表' },
    { key: '{{.Slug}}:save', title: '{{.Title}} 创建' },
    { key: '{{.Slug}}:update', title: '{{.Title}} 更新' },
    { key: '{{.Slug}}:delete', title: '{{.Title}} 删除' },
  ],
} as const satisfies MineModuleManifest

export default {{.Camel}}ModuleManifest
`

const frontendLocaleTemplate = `{{.Slug}}:
  index: {{.Title}}
`

const frontendTypesTemplate = `export interface {{.Pascal}}Item {
  id: number
}
`

const frontendViewTemplate = `<template>
  <div class="{{.Slug}}-page">
    {{.Title}}
  </div>
</template>

<script setup lang="ts">
defineOptions({ name: '{{.Pascal}}Index' })
</script>
`

const frontendE2ETemplate = `import { test, expect } from '@playwright/test'

test.describe('{{.Title}} module', () => {
  test('{{.Slug}} page route is protected or reachable', async ({ page }) => {
    await page.goto('/#/login')
    await expect(page).toHaveURL(/login/)
  })
})
`

const moduleReadmeTemplate = `# {{.Title}} Module

## Register

Add this module to ` + "`app/moduleboot/modules.go`" + `:

` + "```go" + `
{{.PackageName}}.New(),
` + "```" + `

## Lifecycle

Preview lifecycle actions before release:

` + "```bash" + `
go run . artisan module:plan --action=install
go run . artisan module:plan --action=upgrade
go run . artisan module:lifecycle --action=upgrade --module={{.Slug}}
go run . artisan module:plan --action=rollback
go run . artisan module:manifest:check --artifacts --frontend
` + "```" + `

## Generated Assets

- Backend module: ` + "`app/modules/{{.PackageName}}/module.go`" + `
- OpenAPI fragment: ` + "`docs/api-contract/openapi/{{.Slug}}.openapi.json`" + `
- Feature test: ` + "`tests/feature/admin/{{.Snake}}_test.go`" + `
- Frontend module: ` + "`MineAdmin-web/src/modules/{{.Slug}}`" + `
- E2E skeleton: ` + "`MineAdmin-web/tests/e2e/{{.Slug}}.spec.ts`" + `
`
