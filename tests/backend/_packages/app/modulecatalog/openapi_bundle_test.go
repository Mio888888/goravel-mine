package modulecatalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildOpenAPIBundleSortsAndDeduplicatesMatchingComponents(t *testing.T) {
	documents := []OpenAPIDocument{
		{Name: "z.json", Content: fixtureOpenAPIDocument(`
"paths": {"/admin/z": {"post": {"operationId": "zPost", "responses": {"200": {"description": "ok"}}}}},
"components": {"schemas": {"Shared": {"type": "string"}, "Zed": {"type": "integer"}}}
`)},
		{Name: "a.json", Content: fixtureOpenAPIDocument(`
"paths": {"/admin/a": {"get": {"operationId": "aGet", "responses": {"200": {"description": "ok"}}}}},
"components": {"schemas": {"Shared": {"type": "string"}, "Alpha": {"type": "boolean"}}}
`)},
	}

	bundle, err := BuildOpenAPIBundle(documents)
	if err != nil {
		t.Fatalf("BuildOpenAPIBundle() error = %v", err)
	}
	if !strings.Contains(string(bundle.Content), `"/admin/a"`) || !strings.Contains(string(bundle.Content), `"/admin/z"`) {
		t.Fatalf("bundle missing paths: %s", bundle.Content)
	}
	if strings.Index(string(bundle.Content), `"/admin/a"`) > strings.Index(string(bundle.Content), `"/admin/z"`) {
		t.Fatalf("bundle paths not sorted: %s", bundle.Content)
	}
	if strings.Count(string(bundle.Content), `"Shared"`) != 1 {
		t.Fatalf("bundle should deduplicate matching components: %s", bundle.Content)
	}
	if len(bundle.SHA256) != 64 {
		t.Fatalf("bundle SHA256 = %q", bundle.SHA256)
	}

	var parsed map[string]any
	if err := json.Unmarshal(bundle.Content, &parsed); err != nil {
		t.Fatalf("bundle JSON error = %v", err)
	}
}

func TestBuildOpenAPIBundleRejectsConflictingComponents(t *testing.T) {
	documents := []OpenAPIDocument{
		{Name: "first.json", Content: fixtureOpenAPIDocument(`"paths": {"/admin/first": {"get": {"operationId": "firstGet", "responses": {"200": {"description": "ok"}}}}}, "components": {"schemas": {"Shared": {"type": "string"}}}`)},
		{Name: "second.json", Content: fixtureOpenAPIDocument(`"paths": {"/admin/second": {"get": {"operationId": "secondGet", "responses": {"200": {"description": "ok"}}}}}, "components": {"schemas": {"Shared": {"type": "integer"}}}`)},
	}

	_, err := BuildOpenAPIBundle(documents)
	if err == nil || !strings.Contains(err.Error(), "conflicting component schemas.Shared") {
		t.Fatalf("BuildOpenAPIBundle() error = %v", err)
	}
}

func TestWriteOpenAPIBundleAtomicallyLeavesTargetOnFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "modules.openapi.json")
	if err := os.WriteFile(target, []byte("existing"), 0644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	err := WriteOpenAPIBundle(target, OpenAPIBundle{})
	if err == nil {
		t.Fatal("WriteOpenAPIBundle() should reject an empty bundle")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "existing" {
		t.Fatalf("target changed after failure: %q", content)
	}
}
