package unit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/bootstrap"
)

const moduleGovernanceMarkdownContract = "../../docs/api-contract/module-governance.md"
const moduleGovernanceOpenAPIContract = "../../docs/api-contract/openapi/module-governance.openapi.json"
const moduleGovernanceFrontendAPI = "../../MineAdmin-web/src/modules/base/api/platformModuleLifecycle.ts"

type governanceAPISpec struct {
	OpenAPI    string                                       `json:"openapi"`
	Paths      map[string]map[string]governanceAPIOperation `json:"paths"`
	Components apiComponents                                `json:"components"`
}

type governanceAPIOperation struct {
	OperationID string                 `json:"operationId"`
	Tags        []string               `json:"tags"`
	Security    []map[string][]string  `json:"security"`
	Parameters  []apiParameter         `json:"parameters"`
	Responses   map[string]apiResponse `json:"responses"`
	Permission  string                 `json:"x-permission"`
}

func TestModuleGovernanceOpenAPI(t *testing.T) {
	spec := loadModuleGovernanceSpec(t)
	t.Run("machine readable", func(t *testing.T) { assertGovernanceMachineContract(t, spec) })
	t.Run("markdown and runtime parity", func(t *testing.T) { assertGovernanceRuntimeParity(t, spec) })
	t.Run("frontend and envelopes", func(t *testing.T) { assertGovernanceFrontendParity(t, spec) })
}

func assertGovernanceMachineContract(t *testing.T, spec governanceAPISpec) {
	t.Helper()
	require.Equal(t, "3.1.0", spec.OpenAPI)
	require.Len(t, spec.Paths, 7)
	require.Equal(t, []string{"RunKey", "ModuleID", "Action", "Status", "Owner", "Page", "PageSize"},
		parameterRefs(spec.Paths["/admin/platform/module-lifecycle/runs"]["get"].Parameters))
	require.Equal(t, []string{"RunKey", "ModuleID", "Action", "Status", "Page", "PageSize"},
		parameterRefs(spec.Paths["/admin/platform/module-lifecycle/steps"]["get"].Parameters))
	for _, name := range governanceSchemaNames() {
		require.Contains(t, spec.Components.Schemas, name)
	}
	require.ElementsMatch(t, []string{"install", "upgrade", "rollback", "uninstall"},
		schemaEnumStrings(t, spec.Components.Schemas["ModuleLifecycleExecuteRequest"].Properties["action"]))
	require.ElementsMatch(t, governanceStatuses(),
		schemaEnumStrings(t, spec.Components.Schemas["ModuleLifecycleResultItem"].Properties["status"]))
}

func parameterRefs(parameters []apiParameter) []string {
	items := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		items = append(items, strings.TrimPrefix(parameter.Ref, "#/components/parameters/"))
	}
	return items
}

func assertGovernanceRuntimeParity(t *testing.T, spec governanceAPISpec) {
	t.Helper()
	markdownEndpoints := extractModuleGovernanceMarkdownEndpoints(t)
	require.Len(t, markdownEndpoints, 7)

	routes := moduleGovernanceRoutes()
	require.ElementsMatch(t, endpointKeys(markdownEndpoints), endpointKeys(extractGovernanceSpecEndpoints(spec)))
	require.ElementsMatch(t, endpointKeys(markdownEndpoints), sortedRouteKeys(routes))

	for _, endpoint := range extractGovernanceSpecEndpoints(spec) {
		operation := spec.Paths[endpoint.Path][endpoint.Method]
		key := endpoint.Method + " " + endpoint.Path
		require.Equal(t, routes[key], operation.Permission)
		require.NotEmpty(t, operation.OperationID)
		require.Equal(t, []string{"Module Governance"}, operation.Tags)
		require.True(t, governanceSecurityUsesBearer(operation.Security), "%s missing bearerAuth", key)
	}
}

func assertGovernanceFrontendParity(t *testing.T, spec governanceAPISpec) {
	t.Helper()
	content, err := os.ReadFile(filepath.Clean(moduleGovernanceFrontendAPI))
	require.NoError(t, err)
	source := string(content)
	require.Contains(t, source, "locale_files?: string[]")

	for path, methods := range spec.Paths {
		require.Contains(t, source, fmt.Sprintf("'%s'", path))
		for method, operation := range methods {
			assertGovernanceResponse(t, governanceResponseCase{
				Spec: spec, Method: method, Path: path, Operation: operation,
			})
		}
	}
}

type governanceResponseCase struct {
	Spec      governanceAPISpec
	Method    string
	Path      string
	Operation governanceAPIOperation
}

func assertGovernanceResponse(t *testing.T, item governanceResponseCase) {
	t.Helper()
	response := resolveResponse(item.Operation.Responses["200"], item.Spec.Components.Responses)
	schema := response.Content["application/json"].Schema
	require.Truef(t, schemaHasRef(schema, "#/components/schemas/ApiResponse"), "%s %s missing ApiResponse", item.Method, item.Path)
	require.Truef(t, schemaHasTypedData(schema), "%s %s success data must be typed", item.Method, item.Path)
	require.Truef(t, schemaHasRef(schema, "#/components/schemas/ErrorResponse"), "%s %s missing ErrorResponse", item.Method, item.Path)
}

func governanceSchemaNames() []string {
	return []string{"ApiResponse", "ErrorResponse", "PageData", "ModuleLifecycleDependency", "ModuleLifecycleMetadata",
		"ModuleLifecyclePersistedState", "ModuleLifecycleState", "ModuleLifecycleRun", "ModuleLifecycleStep",
		"ModuleLifecycleLock", "ModuleLifecycleDiff", "ModuleLifecycleExecuteRequest", "ModuleLifecycleResult",
		"ModuleLifecycleResultItem", "ModuleLifecycleLockReleaseRequest", "ModuleLifecycleLockReleaseResult"}
}

func governanceStatuses() []string {
	return []string{"planned", "running", "succeeded", "failed", "skipped", "lock_blocked", "manual_required", "reconciliation_required"}
}

func loadModuleGovernanceSpec(t *testing.T) governanceAPISpec {
	t.Helper()

	content, err := os.ReadFile(filepath.Clean(moduleGovernanceOpenAPIContract))
	require.NoError(t, err)

	var spec governanceAPISpec
	require.NoError(t, json.Unmarshal(content, &spec))
	return spec
}

func extractModuleGovernanceMarkdownEndpoints(t *testing.T) []apiEndpoint {
	t.Helper()

	content, err := os.ReadFile(filepath.Clean(moduleGovernanceMarkdownContract))
	require.NoError(t, err)

	pattern := regexp.MustCompile("`(GET|POST) ([^`]+)`")
	matches := pattern.FindAllStringSubmatch(string(content), -1)
	endpoints := make([]apiEndpoint, 0, len(matches))
	for _, match := range matches {
		endpoints = append(endpoints, apiEndpoint{Method: strings.ToLower(match[1]), Path: match[2]})
	}
	return endpoints
}

func extractGovernanceSpecEndpoints(spec governanceAPISpec) []apiEndpoint {
	endpoints := make([]apiEndpoint, 0, len(spec.Paths))
	for path, methods := range spec.Paths {
		for method := range methods {
			endpoints = append(endpoints, apiEndpoint{Method: method, Path: path})
		}
	}
	return endpoints
}

func moduleGovernanceRoutes() map[string]string {
	routes := make(map[string]string)
	for _, route := range bootstrap.Modules().Routes() {
		if !strings.HasPrefix(route.Name, "platform.module-lifecycle.") {
			continue
		}
		routes[strings.ToLower(route.Method)+" "+route.Path] = route.Permission
	}
	return routes
}

func sortedRouteKeys(routes map[string]string) []string {
	keys := make([]string, 0, len(routes))
	for key := range routes {
		keys = append(keys, key)
	}
	return endpointKeys(func() []apiEndpoint {
		items := make([]apiEndpoint, 0, len(keys))
		for _, key := range keys {
			items = append(items, splitEndpointKey(key))
		}
		return items
	}())
}

func governanceSecurityUsesBearer(security []map[string][]string) bool {
	for _, item := range security {
		if _, ok := item["bearerAuth"]; ok {
			return true
		}
	}
	return false
}
