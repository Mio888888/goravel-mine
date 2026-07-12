package commands

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

func TestValidateManifestServiceChecksOpenAPISemanticsWithArtifacts(t *testing.T) {
	root := t.TempDir()
	path := writeOpenAPIArtifact(t, root, `{"openapi":"3.1.0","info":{"title":"Example","version":"1"},"paths":{"/admin/manifest-check-stub/list":{"get":{"operationId":"exampleGet","x-permission":"wrong:permission","responses":{"200":{"description":"ok"}}}}}}`)
	service := modulecatalog.NewService(modules.NewRegistry([]modules.Module{manifestCheckOpenAPIStubModule{openAPI: path}}))

	err := validateManifestService(service, true)
	if err == nil || !strings.Contains(err.Error(), "route permission mismatch: GET /admin/manifest-check-stub/list") {
		t.Fatalf("validateManifestService() error = %v", err)
	}
}

func writeOpenAPIArtifact(t *testing.T, root string, content string) string {
	t.Helper()
	path := filepath.Join(root, "module.openapi.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write OpenAPI artifact: %v", err)
	}
	return path
}

type manifestCheckOpenAPIStubModule struct {
	openAPI string
}

func (m manifestCheckOpenAPIStubModule) ID() string { return "manifest-check-openapi-stub" }
func (m manifestCheckOpenAPIStubModule) Routes() []modules.Route {
	return []modules.Route{{
		Name:       "manifest-check-openapi-stub.list",
		Method:     "GET",
		Path:       "/admin/manifest-check-stub/list",
		Permission: "manifest-check-stub:list",
		Install:    func() {},
	}}
}
func (m manifestCheckOpenAPIStubModule) Menus() []modules.Menu { return nil }
func (m manifestCheckOpenAPIStubModule) Permissions() []modules.Permission {
	return []modules.Permission{{Key: "manifest-check-stub:list"}}
}
func (m manifestCheckOpenAPIStubModule) Migrations() []schema.Migration { return nil }
func (m manifestCheckOpenAPIStubModule) Seeders() []seeder.Seeder       { return nil }
func (m manifestCheckOpenAPIStubModule) OpenAPIFiles() []string         { return []string{m.openAPI} }
func (m manifestCheckOpenAPIStubModule) TestTemplates() []string        { return nil }
