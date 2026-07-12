package modulecatalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var openAPIMethods = map[string]struct{}{
	"delete": {}, "get": {}, "head": {}, "options": {}, "patch": {}, "post": {}, "put": {}, "trace": {},
}

type OpenAPIDocument struct {
	Name    string
	Content []byte
}

type OpenAPIRoute struct {
	Method      string
	Path        string
	Permissions []string
}

type openAPIDocument struct {
	OpenAPI    string                     `json:"openapi"`
	Info       json.RawMessage            `json:"info"`
	Paths      map[string]json.RawMessage `json:"paths"`
	Components map[string]json.RawMessage `json:"components"`
}

type openAPIOperation struct {
	OperationID string            `json:"operationId"`
	Permission  string            `json:"x-permission"`
	Parameters  []json.RawMessage `json:"parameters"`
	raw         json.RawMessage
}

type openAPIParameter struct {
	Name string `json:"name"`
	In   string `json:"in"`
}

type openAPILintState struct {
	components  map[string]map[string][]byte
	operationID map[string]string
	routes      map[string]openAPIOperation
}

func OpenAPIDocumentsFromFiles(paths []string) ([]OpenAPIDocument, error) {
	seen := make(map[string]struct{}, len(paths))
	documents := make([]OpenAPIDocument, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		resolved := resolveOpenAPIRepositoryPath(path)
		key := filepath.Clean(resolved)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		content, err := os.ReadFile(key)
		if err != nil {
			return nil, fmt.Errorf("read OpenAPI document %s: %w", path, err)
		}
		documents = append(documents, OpenAPIDocument{Name: path, Content: content})
	}
	return documents, nil
}

func LintOpenAPI(documents []OpenAPIDocument, routes []OpenAPIRoute) error {
	state, err := parseOpenAPIDocuments(documents)
	if err != nil {
		return err
	}
	return validateOpenAPIRoutes(state.routes, routes)
}

func LintManifestOpenAPI(manifest Manifest) error {
	documents, err := OpenAPIDocumentsFromFiles(manifestOpenAPIFiles(manifest))
	if err != nil {
		return err
	}
	return LintOpenAPI(documents, manifestOpenAPIRoutes(manifest))
}

func BuildManifestOpenAPIBundle(manifest Manifest) (OpenAPIBundle, error) {
	documents, err := OpenAPIDocumentsFromFiles(manifestOpenAPIFiles(manifest))
	if err != nil {
		return OpenAPIBundle{}, err
	}
	if err := LintOpenAPI(documents, manifestOpenAPIRoutes(manifest)); err != nil {
		return OpenAPIBundle{}, err
	}
	return BuildOpenAPIBundle(documents)
}

func manifestOpenAPIFiles(manifest Manifest) []string {
	paths := make([]string, 0)
	for _, module := range manifest.Modules {
		paths = append(paths, module.OpenAPIFiles...)
	}
	return paths
}

func manifestOpenAPIRoutes(manifest Manifest) []OpenAPIRoute {
	routes := make([]OpenAPIRoute, 0)
	seen := make(map[string]struct{})
	for _, module := range manifest.Modules {
		for _, route := range module.Routes {
			permissions := append([]string(nil), route.Permissions...)
			if route.Permission != "" {
				permissions = append([]string{route.Permission}, permissions...)
			}
			item := OpenAPIRoute{Method: route.Method, Path: route.Path, Permissions: permissions}
			key := openAPIRouteKey(item.Method, item.Path)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			routes = append(routes, item)
		}
	}
	return routes
}

func parseOpenAPIDocuments(documents []OpenAPIDocument) (openAPILintState, error) {
	state := openAPILintState{
		components:  make(map[string]map[string][]byte),
		operationID: make(map[string]string),
		routes:      make(map[string]openAPIOperation),
	}
	var errs []error
	for _, document := range documents {
		parsed, err := parseOpenAPIDocument(document)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		errs = append(errs, collectOpenAPIComponents(&state, document.Name, parsed.Components)...)
		errs = append(errs, collectOpenAPIOperations(&state, document.Name, parsed.OpenAPI, parsed.Paths)...)
	}
	return state, errors.Join(errs...)
}

func parseOpenAPIDocument(document OpenAPIDocument) (openAPIDocument, error) {
	var parsed openAPIDocument
	if err := json.Unmarshal(document.Content, &parsed); err != nil {
		return openAPIDocument{}, fmt.Errorf("OpenAPI document %s has invalid JSON: %w", document.Name, err)
	}
	if !strings.HasPrefix(parsed.OpenAPI, "3.") {
		return openAPIDocument{}, fmt.Errorf("OpenAPI document %s must declare OpenAPI 3.x", document.Name)
	}
	if len(parsed.Info) == 0 || len(parsed.Paths) == 0 {
		return openAPIDocument{}, fmt.Errorf("OpenAPI document %s must declare info and paths", document.Name)
	}
	if err := validateOpenAPIReferences(document.Name, document.Content, parsed); err != nil {
		return openAPIDocument{}, err
	}
	return parsed, nil
}

func validateOpenAPIReferences(name string, content []byte, document openAPIDocument) error {
	var value any
	if err := json.Unmarshal(content, &value); err != nil {
		return fmt.Errorf("OpenAPI document %s has invalid JSON: %w", name, err)
	}
	var errs []error
	visitOpenAPIValue(value, func(reference string) {
		if !strings.HasPrefix(reference, "#/") {
			errs = append(errs, fmt.Errorf("OpenAPI document %s has unsupported external $ref: %s", name, reference))
			return
		}
		if !openAPIReferenceExists(document, reference) {
			errs = append(errs, fmt.Errorf("OpenAPI document %s has unresolved $ref: %s", name, reference))
		}
	})
	return errors.Join(errs...)
}

func visitOpenAPIValue(value any, visit func(string)) {
	switch item := value.(type) {
	case map[string]any:
		if reference, ok := item["$ref"].(string); ok {
			visit(reference)
		}
		for _, child := range item {
			visitOpenAPIValue(child, visit)
		}
	case []any:
		for _, child := range item {
			visitOpenAPIValue(child, visit)
		}
	}
}

func openAPIReferenceExists(document openAPIDocument, reference string) bool {
	parts := strings.Split(strings.TrimPrefix(reference, "#/"), "/")
	if len(parts) != 3 || parts[0] != "components" {
		return false
	}
	section, ok := document.Components[parts[1]]
	if !ok {
		return false
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(section, &entries); err != nil {
		return false
	}
	_, ok = entries[unescapeJSONPointer(parts[2])]
	return ok
}

func unescapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~1", "/"), "~0", "~")
}

func collectOpenAPIComponents(state *openAPILintState, name string, components map[string]json.RawMessage) []error {
	var errs []error
	for _, kind := range sortedRawMessageKeys(components) {
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(components[kind], &entries); err != nil {
			errs = append(errs, fmt.Errorf("OpenAPI document %s has invalid components.%s: %w", name, kind, err))
			continue
		}
		if state.components[kind] == nil {
			state.components[kind] = make(map[string][]byte)
		}
		for _, componentName := range sortedRawMessageKeys(entries) {
			canonical, err := canonicalJSON(entries[componentName])
			if err != nil {
				errs = append(errs, fmt.Errorf("OpenAPI document %s has invalid component %s.%s: %w", name, kind, componentName, err))
				continue
			}
			if existing, ok := state.components[kind][componentName]; ok && !bytes.Equal(existing, canonical) {
				if !openAPIComponentsAreCompatible(existing, canonical) {
					errs = append(errs, fmt.Errorf("conflicting component %s.%s between documents", kind, componentName))
				}
				continue
			}
			state.components[kind][componentName] = canonical
		}
	}
	return errs
}

func openAPIComponentsAreCompatible(first, second []byte) bool {
	if bytes.Equal(first, second) {
		return true
	}
	firstReferences := openAPIComponentReferences(first)
	secondReferences := openAPIComponentReferences(second)
	if len(firstReferences) == 1 && len(secondReferences) == 1 && firstReferences[0] == secondReferences[0] {
		return true
	}
	return openAPIKnownSharedComponent(first, second)
}

func openAPIKnownSharedComponent(first, second []byte) bool {
	var firstSchema openAPISharedSchema
	var secondSchema openAPISharedSchema
	if json.Unmarshal(first, &firstSchema) != nil || json.Unmarshal(second, &secondSchema) != nil {
		return false
	}
	if firstSchema.Type != "object" || secondSchema.Type != "object" || len(firstSchema.Properties) == 0 || len(secondSchema.Properties) == 0 {
		return false
	}
	for _, key := range []string{"code", "message", "data", "list", "total"} {
		firstProperty, firstOK := firstSchema.Properties[key]
		secondProperty, secondOK := secondSchema.Properties[key]
		if !firstOK && !secondOK {
			continue
		}
		if !firstOK || !secondOK || !openAPISharedPropertyCompatible(firstProperty, secondProperty) {
			return false
		}
	}
	return true
}

type openAPISharedSchema struct {
	Type       string                           `json:"type"`
	Properties map[string]openAPISharedProperty `json:"properties"`
}

type openAPISharedProperty struct {
	Ref  string `json:"$ref"`
	Type string `json:"type"`
}

func openAPISharedPropertyCompatible(first, second openAPISharedProperty) bool {
	if first.Ref == second.Ref && first.Ref != "" {
		return true
	}
	if first.Type == second.Type && first.Type != "" {
		return true
	}
	return (first.Ref != "" && second.Type == "array") || (second.Ref != "" && first.Type == "array")
}

func openAPIComponentReferences(content []byte) []string {
	var value any
	if json.Unmarshal(content, &value) != nil {
		return nil
	}
	references := make([]string, 0)
	visitOpenAPIValue(value, func(reference string) { references = append(references, reference) })
	return references
}

func collectOpenAPIOperations(state *openAPILintState, name string, version string, paths map[string]json.RawMessage) []error {
	var errs []error
	for _, path := range sortedRawMessageKeys(paths) {
		var pathItem map[string]json.RawMessage
		if err := json.Unmarshal(paths[path], &pathItem); err != nil {
			errs = append(errs, fmt.Errorf("OpenAPI document %s has invalid path %s: %w", name, path, err))
			continue
		}
		for _, method := range sortedRawMessageKeys(pathItem) {
			method = strings.ToLower(method)
			if _, ok := openAPIMethods[method]; !ok {
				continue
			}
			var operation openAPIOperation
			if err := json.Unmarshal(pathItem[method], &operation); err != nil {
				errs = append(errs, fmt.Errorf("OpenAPI document %s has invalid %s %s operation: %w", name, strings.ToUpper(method), path, err))
				continue
			}
			raw, err := canonicalJSON(pathItem[method])
			if err != nil {
				errs = append(errs, fmt.Errorf("OpenAPI document %s has invalid %s %s operation: %w", name, strings.ToUpper(method), path, err))
				continue
			}
			operation.raw = raw
			key := openAPIRouteKey(method, path)
			if existing, ok := state.routes[key]; ok {
				errs = append(errs, fmt.Errorf("duplicate operation route: %s (operationId %q conflicts with %q)", key, existing.OperationID, operation.OperationID))
				continue
			}
			if operation.OperationID != "" {
				if existing, ok := state.operationID[operation.OperationID]; ok {
					errs = append(errs, fmt.Errorf("duplicate operationId %q: %s and %s", operation.OperationID, existing, key))
				} else {
					state.operationID[operation.OperationID] = key
				}
			} else if strings.HasPrefix(version, "3.1.") {
				errs = append(errs, fmt.Errorf("OpenAPI operation missing operationId: %s", key))
			}
			errs = append(errs, duplicateOpenAPIParameterErrors(key, operation.Parameters)...)
			state.routes[key] = operation
		}
	}
	return errs
}

func duplicateOpenAPIParameterErrors(route string, parameters []json.RawMessage) []error {
	seen := make(map[string]struct{}, len(parameters))
	var errs []error
	for _, raw := range parameters {
		var parameter openAPIParameter
		if err := json.Unmarshal(raw, &parameter); err != nil {
			continue
		}
		if parameter.Name == "" || parameter.In == "" {
			continue
		}
		key := parameter.In + " " + parameter.Name
		if _, ok := seen[key]; ok {
			errs = append(errs, fmt.Errorf("duplicate parameter %s on %s", key, route))
			continue
		}
		seen[key] = struct{}{}
	}
	return errs
}

func validateOpenAPIRoutes(operations map[string]openAPIOperation, routes []OpenAPIRoute) error {
	if routes == nil {
		return nil
	}
	routeByKey := make(map[string]OpenAPIRoute, len(routes))
	for _, route := range routes {
		routeByKey[openAPIRouteKey(route.Method, route.Path)] = route
	}
	var errs []error
	for _, key := range sortedOperationKeys(operations) {
		operation := operations[key]
		route, ok := routeByKey[key]
		if !ok {
			continue
		}
		if !openAPIPermissionMatches(route.Permissions, operation.Permission) {
			errs = append(errs, fmt.Errorf("route permission mismatch: %s (OpenAPI %q, route %s)", key, operation.Permission, strings.Join(route.Permissions, ",")))
		}
	}
	return errors.Join(errs...)
}

func openAPIPermissionMatches(routePermissions []string, documented string) bool {
	if documented == "" {
		return true
	}
	if len(routePermissions) == 0 {
		return documented == ""
	}
	for _, permission := range routePermissions {
		if permission == documented {
			return true
		}
	}
	return false
}

func openAPIRouteKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}

func sortedRawMessageKeys(items map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedOperationKeys(items map[string]openAPIOperation) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func canonicalJSON(value []byte) ([]byte, error) {
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, err
	}
	return json.Marshal(decoded)
}

func resolveOpenAPIRepositoryPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	for {
		candidate := filepath.Join(cwd, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return candidate
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return path
		}
		cwd = parent
	}
}
