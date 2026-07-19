package modulecatalog

import (
	"strings"
	"testing"
)

func TestOpenAPILinterRejectsSemanticConflicts(t *testing.T) {
	tests := []struct {
		name      string
		documents []OpenAPIDocument
		expected  string
	}{
		{
			name:      "invalid document",
			documents: []OpenAPIDocument{{Name: "invalid.json", Content: []byte(`{"openapi":`)}},
			expected:  "invalid JSON",
		},
		{
			name: "unresolved reference",
			documents: []OpenAPIDocument{{Name: "unresolved.json", Content: fixtureOpenAPIDocument(`
"paths": {"/admin/example": {"get": {"operationId": "exampleGet", "responses": {"200": {"$ref": "#/components/responses/Missing"}}}}}
`)}},
			expected: "unresolved $ref",
		},
		{
			name: "duplicate operation ID",
			documents: []OpenAPIDocument{
				{Name: "first.json", Content: fixtureOpenAPIDocument(`"paths": {"/admin/first": {"get": {"operationId": "exampleGet", "responses": {"200": {"description": "ok"}}}}}`)},
				{Name: "second.json", Content: fixtureOpenAPIDocument(`"paths": {"/admin/second": {"post": {"operationId": "exampleGet", "responses": {"200": {"description": "ok"}}}}}`)},
			},
			expected: "duplicate operationId",
		},
		{
			name:      "missing operation ID",
			documents: []OpenAPIDocument{{Name: "missing-operation-id.json", Content: fixtureOpenAPIDocument(`"paths": {"/admin/example": {"get": {"responses": {"200": {"description": "ok"}}}}}`)}},
			expected:  "missing operationId",
		},
		{
			name: "duplicate method path",
			documents: []OpenAPIDocument{
				{Name: "first.json", Content: fixtureOpenAPIDocument(`"paths": {"/admin/example": {"get": {"operationId": "firstGet", "responses": {"200": {"description": "ok"}}}}}`)},
				{Name: "second.json", Content: fixtureOpenAPIDocument(`"paths": {"/admin/example": {"get": {"operationId": "secondGet", "responses": {"200": {"description": "ok"}}}}}`)},
			},
			expected: "duplicate operation route",
		},
		{
			name: "duplicate parameter",
			documents: []OpenAPIDocument{{Name: "parameters.json", Content: fixtureOpenAPIDocument(`
"paths": {"/admin/example": {"get": {"operationId": "exampleGet", "parameters": [
  {"name": "page", "in": "query"}, {"name": "page", "in": "query"}
], "responses": {"200": {"description": "ok"}}}}}
`)}},
			expected: "duplicate parameter",
		},
		{
			name: "conflicting schema",
			documents: []OpenAPIDocument{
				{Name: "first.json", Content: fixtureOpenAPIDocument(`"paths": {"/admin/first": {"get": {"operationId": "firstGet", "responses": {"200": {"description": "ok"}}}}}, "components": {"schemas": {"Shared": {"type": "string"}}}`)},
				{Name: "second.json", Content: fixtureOpenAPIDocument(`"paths": {"/admin/second": {"get": {"operationId": "secondGet", "responses": {"200": {"description": "ok"}}}}}, "components": {"schemas": {"Shared": {"type": "integer"}}}`)},
			},
			expected: "conflicting component schemas.Shared",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := LintOpenAPI(test.documents, nil)
			if err == nil || !strings.Contains(err.Error(), test.expected) {
				t.Fatalf("LintOpenAPI() error = %v, expected %q", err, test.expected)
			}
		})
	}
}

func TestOpenAPILinterChecksRouteCoverageAndPermission(t *testing.T) {
	document := OpenAPIDocument{Name: "module.json", Content: fixtureOpenAPIDocument(`
	"paths": {
	  "/admin/covered": {"get": {"operationId": "coveredGet", "x-permission": "covered:list", "responses": {"200": {"description": "ok"}}}},
	  "/admin/undeclared": {"post": {"operationId": "undeclaredPost", "x-permission": "covered:list", "responses": {"200": {"description": "ok"}}}}
}
	`)}
	routes := []OpenAPIRoute{
		{Method: "GET", Path: "/admin/covered", Permissions: []string{"covered:read"}},
		{Method: "DELETE", Path: "/admin/missing", Permissions: []string{"covered:delete"}},
	}

	err := LintOpenAPI([]OpenAPIDocument{document}, routes)
	if err == nil {
		t.Fatal("LintOpenAPI() should reject route coverage and permission mismatches")
	}
	for _, expected := range []string{
		"route permission mismatch: GET /admin/covered",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("LintOpenAPI() error = %q, missing %q", err.Error(), expected)
		}
	}
}

func TestOpenAPILinterAllowsFragmentsToCoverSharedRouteSubsets(t *testing.T) {
	document := OpenAPIDocument{Name: "subset.json", Content: fixtureOpenAPIDocument(`
"paths": {"/admin/covered": {"get": {"operationId": "coveredGet", "x-permission": "covered:list", "responses": {"200": {"description": "ok"}}}}}
`)}
	routes := []OpenAPIRoute{
		{Method: "GET", Path: "/admin/covered", Permissions: []string{"covered:list"}},
		{Method: "DELETE", Path: "/admin/other", Permissions: []string{"covered:delete"}},
	}

	if err := LintOpenAPI([]OpenAPIDocument{document}, routes); err != nil {
		t.Fatalf("LintOpenAPI() error = %v", err)
	}
}

func TestOpenAPILinterAllowsExistingFragmentsWithoutPermissionExtension(t *testing.T) {
	document := OpenAPIDocument{Name: "legacy.json", Content: fixtureOpenAPIDocument(`
"paths": {"/admin/example": {"get": {"operationId": "exampleGet", "responses": {"200": {"description": "ok"}}}}}
`)}

	if err := LintOpenAPI([]OpenAPIDocument{document}, []OpenAPIRoute{{
		Method: "GET", Path: "/admin/example", Permissions: []string{"example:list"},
	}}); err != nil {
		t.Fatalf("LintOpenAPI() error = %v", err)
	}
}

func TestOpenAPILinterRejectsUnsupportedOpenAPIVersion(t *testing.T) {
	document := OpenAPIDocument{Name: "openapi-2.json", Content: []byte(`
{"openapi":"2.0","info":{"title":"Example","version":"1"},"paths":{"/admin/example":{"get":{"responses":{"200":{"description":"ok"}}}}}}
`)}

	err := LintOpenAPI([]OpenAPIDocument{document}, nil)
	if err == nil || !strings.Contains(err.Error(), "must declare OpenAPI 3.x") {
		t.Fatalf("LintOpenAPI() error = %v", err)
	}
}

func TestOpenAPILinterAllowsKnownSharedEnvelopeComponents(t *testing.T) {
	documents := []OpenAPIDocument{
		{Name: "reference.json", Content: fixtureOpenAPIDocument(`
"paths": {"/admin/first": {"get": {"operationId": "firstGet", "responses": {"200": {"description": "ok"}}}}},
"components": {"schemas": {"EmptyArray": {"type": "array"}, "ErrorResponse": {"type": "object", "properties": {"code": {"type": "integer"}, "message": {"type": "string"}, "data": {"$ref": "#/components/schemas/EmptyArray"}}}}}
`)},
		{Name: "fragment.json", Content: fixtureOpenAPIDocument(`
"paths": {"/admin/second": {"get": {"operationId": "secondGet", "responses": {"200": {"description": "ok"}}}}},
"components": {"schemas": {"ErrorResponse": {"type": "object", "properties": {"code": {"type": "integer"}, "message": {"type": "string"}, "data": {"type": "array"}}}}}
`)},
	}

	if err := LintOpenAPI(documents, nil); err != nil {
		t.Fatalf("LintOpenAPI() error = %v", err)
	}
}

func TestOpenAPILinterAcceptsSharedFilesAndMatchingComponents(t *testing.T) {
	document := OpenAPIDocument{Name: "module.json", Content: fixtureOpenAPIDocument(`
"paths": {"/admin/example": {"get": {"operationId": "exampleGet", "x-permission": "example:list", "responses": {"200": {"description": "ok"}}}}},
"components": {"schemas": {"Shared": {"type": "string"}}}
`)}

	err := LintOpenAPI([]OpenAPIDocument{document}, []OpenAPIRoute{{
		Method: "GET", Path: "/admin/example", Permissions: []string{"example:list"},
	}})
	if err != nil {
		t.Fatalf("LintOpenAPI() error = %v", err)
	}
}

func fixtureOpenAPIDocument(content string) []byte {
	return []byte(`{"openapi":"3.1.0","info":{"title":"Example","version":"1"},` + content + `}`)
}
