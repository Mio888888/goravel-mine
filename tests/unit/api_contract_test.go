package unit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/bootstrap"
)

const adminAPIContractMarkdown = "../../docs/api-contract/admin-base-apis.md"
const adminAPIOpenAPIContract = "../../docs/api-contract/openapi/admin-base-apis.openapi.json"
const webRoutesFile = "../../routes/web.go"

type apiSpec struct {
	OpenAPI    string                                       `json:"openapi"`
	Info       struct{ Title, Version, Description string } `json:"info"`
	Paths      map[string]map[string]apiOperation           `json:"paths"`
	Components apiComponents                                `json:"components"`
}

type apiComponents struct {
	Schemas    map[string]apiSchema    `json:"schemas"`
	Parameters map[string]apiParameter `json:"parameters"`
	Responses  map[string]apiResponse  `json:"responses"`
}

type apiOperation struct {
	OperationID string                 `json:"operationId"`
	Tags        []string               `json:"tags"`
	Security    []map[string][]string  `json:"security"`
	Parameters  []apiParameter         `json:"parameters"`
	Responses   map[string]apiResponse `json:"responses"`
}

type apiParameter struct {
	Ref  string `json:"$ref"`
	Name string `json:"name"`
	In   string `json:"in"`
}

type apiResponse struct {
	Ref     string              `json:"$ref"`
	Content map[string]apiMedia `json:"content"`
}

type apiMedia struct {
	Schema apiSchema `json:"schema"`
}

type apiSchema struct {
	Ref        string               `json:"$ref"`
	Type       string               `json:"type"`
	Format     string               `json:"format"`
	Enum       json.RawMessage      `json:"enum"`
	Required   []string             `json:"required"`
	Properties map[string]apiSchema `json:"properties"`
	AllOf      []apiSchema          `json:"allOf"`
	OneOf      []apiSchema          `json:"oneOf"`
	Items      *apiSchema           `json:"items"`
}

type apiEndpoint struct{ Method, Path string }

func TestAdminOpenAPIIsMachineReadable(t *testing.T) {
	spec := loadAdminAPISpec(t)
	require.True(t, strings.HasPrefix(spec.OpenAPI, "3.1."))
	require.NotEmpty(t, spec.Info.Title)
	require.NotEmpty(t, spec.Info.Version)
	require.Contains(t, strings.ToLower(spec.Info.Description), "scoped subset")
	require.NotEmpty(t, spec.Paths)

	apiResponse := spec.Components.Schemas["ApiResponse"]
	require.Equal(t, "object", apiResponse.Type)
	require.ElementsMatch(t, []string{"code", "message", "data"}, apiResponse.Required)
	require.Equal(t, "integer", apiResponse.Properties["code"].Type)
	require.JSONEq(t, `[200]`, string(apiResponse.Properties["code"].Enum))
	require.Equal(t, "string", apiResponse.Properties["message"].Type)
	require.JSONEq(t, `[401,403,404,405,406,422,423,429,500]`,
		string(spec.Components.Schemas["ErrorResponse"].Properties["code"].Enum))
	require.Equal(t, "#/components/schemas/EmptyArray", spec.Components.Schemas["ErrorResponse"].Properties["data"].Ref)
}

func TestAdminOpenAPIEndpointCoverage(t *testing.T) {
	spec := loadAdminAPISpec(t)

	require.NotContains(t, spec.Paths, "/storage/{path}")
	require.NotContains(t, spec.Paths, "/admin/user-login-log")
	require.NotContains(t, spec.Paths, "/admin/user-operation-log")
	require.Contains(t, spec.Paths, "/admin/platform/attachment/upload")
	require.Contains(t, spec.Paths["/admin/platform/attachment/upload"], "post")
	require.ElementsMatch(t, endpointKeys(extractMarkdownEndpoints(t)), endpointKeys(extractSpecEndpoints(spec)))
}

func TestAdminOpenAPIEndpointsAreImplemented(t *testing.T) {
	specEndpoints := endpointKeys(extractSpecEndpoints(loadAdminAPISpec(t)))
	implemented := endpointKeySet(extractImplementedRouteEndpoints(t))

	for _, endpoint := range specEndpoints {
		require.Truef(t, implemented[endpoint], "OpenAPI endpoint %q is not implemented by routes/web.go or registered modules", endpoint)
	}
}

func TestAdminOpenAPIOperationsSupportSdkGeneration(t *testing.T) {
	spec := loadAdminAPISpec(t)
	seen := map[string]bool{}

	for path, methods := range spec.Paths {
		for method, operation := range methods {
			require.NotEmptyf(t, operation.OperationID, "%s %s missing operationId", method, path)
			require.NotEmptyf(t, operation.Tags, "%s %s missing tags", method, path)
			require.Falsef(t, seen[operation.OperationID], "duplicate operationId %q", operation.OperationID)
			seen[operation.OperationID] = true
		}
	}
}

func TestAdminOpenAPIResponseContracts(t *testing.T) {
	spec := loadAdminAPISpec(t)

	for path, methods := range spec.Paths {
		for method, operation := range methods {
			response := resolveResponse(operation.Responses["200"], spec.Components.Responses)
			if path == "/admin/platform/tenant/{id}/exports/{run_id}/download" {
				require.Equal(t, "string", response.Content["application/x-ndjson"].Schema.Type)
				require.Equal(t, "binary", response.Content["application/x-ndjson"].Schema.Format)
				continue
			}
			schema := response.Content["application/json"].Schema
			schema = resolveSchema(schema, spec.Components.Schemas)
			require.Truef(t, schemaHasRef(schema, "#/components/schemas/ApiResponse"),
				"%s %s must use ApiResponse envelope", method, path)
			require.Truef(t, schemaHasTypedData(schema),
				"%s %s success response data must be typed for SDK generation", method, path)
			require.Truef(t, schemaHasRef(schema, "#/components/schemas/ErrorResponse"),
				"%s %s must allow HTTP 200 business error envelopes", method, path)
		}
	}
}

func resolveSchema(schema apiSchema, shared map[string]apiSchema) apiSchema {
	if !strings.HasPrefix(schema.Ref, "#/components/schemas/") {
		return schema
	}
	return shared[refName(schema.Ref)]
}

func TestAdminOpenAPIModelsAuthChallengeShape(t *testing.T) {
	spec := loadAdminAPISpec(t)
	response := resolveResponse(spec.Paths["/admin/passport/login"]["post"].Responses["200"], spec.Components.Responses)

	require.ElementsMatch(t, []string{
		"#/components/schemas/AuthTokenData",
		"#/components/schemas/MFAChallengeData",
		"#/components/schemas/PasswordChangeChallengeData",
	}, schemaDataRefs(response.Content["application/json"].Schema))
	require.ElementsMatch(t, []string{"access_token", "refresh_token", "expire_at"}, spec.Components.Schemas["AuthTokenData"].Required)
	require.ElementsMatch(t, []string{"mfa_required", "mfa_token"}, spec.Components.Schemas["MFAChallengeData"].Required)
	require.ElementsMatch(t, []string{"password_change_required", "password_change_token"}, spec.Components.Schemas["PasswordChangeChallengeData"].Required)
}

func TestAdminOpenAPIModelsMenuPayloadShape(t *testing.T) {
	spec := loadAdminAPISpec(t)
	menuPayload := spec.Components.Schemas["MenuPayload"]

	require.Contains(t, menuPayload.Properties, "redirect")
	require.Contains(t, menuPayload.Properties, "remark")
}

func TestAdminOpenAPIModelsPositionDataPermissionShape(t *testing.T) {
	spec := loadAdminAPISpec(t)
	payload := spec.Components.Schemas["PositionDataPermissionPayload"]

	require.ElementsMatch(t, []string{"policy_type"}, payload.Required)
	require.ElementsMatch(t, []string{"ALL", "DEPT_SELF", "DEPT_TREE", "SELF", "CUSTOM_DEPT", "CUSTOM_FUNC"},
		schemaEnumStrings(t, payload.Properties["policy_type"]))
}

func TestAdminOpenAPIAllowsAnonymousPassportChallenges(t *testing.T) {
	spec := loadAdminAPISpec(t)
	anonymousEndpoints := []apiEndpoint{
		{Method: "post", Path: "/admin/passport/login"},
		{Method: "post", Path: "/admin/passport/mfa/login"},
		{Method: "post", Path: "/admin/passport/password/change"},
		{Method: "post", Path: "/admin/platform/passport/login"},
		{Method: "post", Path: "/admin/platform/passport/mfa/login"},
		{Method: "post", Path: "/admin/platform/passport/password/change"},
		{Method: "get", Path: "/admin/platform/passport/csrf-token"},
	}

	for _, endpoint := range anonymousEndpoints {
		operation := spec.Paths[endpoint.Path][endpoint.Method]
		require.NotNilf(t, operation.Security, "%s %s must override global bearerAuth", endpoint.Method, endpoint.Path)
		require.Emptyf(t, operation.Security, "%s %s must allow anonymous access", endpoint.Method, endpoint.Path)
	}
}

func TestAdminOpenAPIIncludesDocumentedQueryParameters(t *testing.T) {
	spec := loadAdminAPISpec(t)
	expected := map[string][]string{
		"get /admin/user/list":                    {"email", "nickname", "page", "per_page", "phone", "status", "username"},
		"get /admin/platform/storage-config/list": {"driver", "name", "page", "page_size", "per_page", "provider", "status"},
		"get /admin/attachment/list":              {"mime_type", "origin_name", "page", "page_size", "per_page", "suffix"},
		"get /admin/user-login-log/list":          {"browser", "ip", "os", "page", "per_page", "status", "username"},
		"get /admin/user-operation-log/list":      {"method", "page", "per_page", "router", "service_name", "username"},
	}

	for key, names := range expected {
		endpoint := splitEndpointKey(key)
		operation := spec.Paths[endpoint.Path][endpoint.Method]
		require.ElementsMatchf(t, names, queryParameterNames(operation.Parameters, spec.Components.Parameters),
			"%s %s query parameters must match markdown contract", endpoint.Method, endpoint.Path)
	}
}

func loadAdminAPISpec(t *testing.T) apiSpec {
	t.Helper()
	content, err := os.ReadFile(filepath.Clean(adminAPIOpenAPIContract))
	require.NoError(t, err)

	var spec apiSpec
	require.NoError(t, json.Unmarshal(content, &spec))
	return spec
}

func extractMarkdownEndpoints(t *testing.T) []apiEndpoint {
	t.Helper()
	content, err := os.ReadFile(filepath.Clean(adminAPIContractMarkdown))
	require.NoError(t, err)

	pattern := regexp.MustCompile("`(GET|POST|PUT|DELETE|PATCH) ([^`]+)`")
	matches := pattern.FindAllStringSubmatch(string(content), -1)
	endpoints := make([]apiEndpoint, 0, len(matches))
	for _, match := range matches {
		path := match[2]
		if path == "/storage/..." {
			continue
		}
		if path != "" {
			endpoints = append(endpoints, apiEndpoint{Method: strings.ToLower(match[1]), Path: path})
		}
	}
	return endpoints
}

func extractSpecEndpoints(spec apiSpec) []apiEndpoint {
	endpoints := make([]apiEndpoint, 0, len(spec.Paths))
	for path, methods := range spec.Paths {
		for method := range methods {
			endpoints = append(endpoints, apiEndpoint{Method: method, Path: path})
		}
	}
	return endpoints
}

func extractImplementedRouteEndpoints(t *testing.T) []apiEndpoint {
	t.Helper()

	file, err := os.Open(filepath.Clean(webRoutesFile))
	require.NoError(t, err)
	defer file.Close()

	pattern := regexp.MustCompile(`\.(Get|Post|Put|Delete|Patch)\("([^"]+)"`)
	endpoints := make([]apiEndpoint, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "facades.Route().") {
			continue
		}
		match := pattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		path := match[2]
		if strings.HasPrefix(path, "/admin/") || path == "/api/system/captcha" {
			endpoints = append(endpoints, apiEndpoint{Method: strings.ToLower(match[1]), Path: path})
		}
	}
	require.NoError(t, scanner.Err())

	for _, route := range bootstrap.Modules().Routes() {
		if route.Install == nil {
			continue
		}
		if strings.HasPrefix(route.Path, "/admin/") || route.Path == "/api/system/captcha" {
			endpoints = append(endpoints, apiEndpoint{Method: strings.ToLower(route.Method), Path: route.Path})
		}
	}

	return endpoints
}

func endpointKeys(endpoints []apiEndpoint) []string {
	keys := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		keys = append(keys, endpoint.Method+" "+endpoint.Path)
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

func endpointKeySet(endpoints []apiEndpoint) map[string]bool {
	keys := endpointKeys(endpoints)
	set := make(map[string]bool, len(keys))
	for _, key := range keys {
		set[key] = true
	}
	return set
}

func splitEndpointKey(key string) apiEndpoint {
	parts := strings.SplitN(key, " ", 2)
	return apiEndpoint{Method: parts[0], Path: parts[1]}
}

func queryParameterNames(parameters []apiParameter, shared map[string]apiParameter) []string {
	names := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		if parameter.Ref != "" {
			parameter = shared[refName(parameter.Ref)]
		}
		if parameter.In == "query" {
			names = append(names, parameter.Name)
		}
	}
	slices.Sort(names)
	return names
}

func refName(ref string) string {
	index := strings.LastIndex(ref, "/")
	if index == -1 {
		return ref
	}
	return ref[index+1:]
}

func schemaEnumStrings(t *testing.T, schema apiSchema) []string {
	t.Helper()
	var values []string
	require.NoError(t, json.Unmarshal(schema.Enum, &values))
	return values
}

func schemaHasRef(schema apiSchema, ref string) bool {
	if schema.Ref == ref {
		return true
	}
	for _, child := range append(schema.AllOf, schema.OneOf...) {
		if schemaHasRef(child, ref) {
			return true
		}
	}
	return false
}

func schemaHasTypedData(schema apiSchema) bool {
	if len(schema.OneOf) > 0 {
		for _, child := range schema.OneOf {
			if schemaHasRef(child, "#/components/schemas/ApiResponse") && schemaHasTypedData(child) {
				return true
			}
		}
		return false
	}
	if data, ok := schema.Properties["data"]; ok {
		return data.Ref != "" || data.Type != "" || len(data.AllOf) > 0 || len(data.OneOf) > 0 || data.Items != nil || len(data.Properties) > 0
	}
	for _, child := range schema.AllOf {
		if schemaHasTypedData(child) {
			return true
		}
	}
	return false
}

func schemaDataRefs(schema apiSchema) []string {
	refs := make([]string, 0)
	if data, ok := schema.Properties["data"]; ok {
		refs = append(refs, data.Ref)
	}
	for _, child := range append(schema.AllOf, schema.OneOf...) {
		refs = append(refs, schemaDataRefs(child)...)
	}
	return refs
}

func resolveResponse(response apiResponse, shared map[string]apiResponse) apiResponse {
	if response.Ref == "" {
		return response
	}
	return shared[refName(response.Ref)]
}
