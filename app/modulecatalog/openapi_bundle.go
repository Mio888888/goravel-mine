package modulecatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type OpenAPIBundle struct {
	Content []byte
	SHA256  string
}

func BuildOpenAPIBundle(documents []OpenAPIDocument) (OpenAPIBundle, error) {
	state, err := parseOpenAPIDocuments(documents)
	if err != nil {
		return OpenAPIBundle{}, err
	}
	paths := make(map[string]map[string]json.RawMessage)
	for key, operation := range state.routes {
		method, path := splitOpenAPIRouteKey(key)
		if paths[path] == nil {
			paths[path] = make(map[string]json.RawMessage)
		}
		paths[path][method] = operation.raw
	}
	document := orderedOpenAPIBundle{OpenAPI: "3.1.0", Info: map[string]string{
		"title": "Module OpenAPI Bundle", "version": "1.0.0",
	}, Paths: newOrderedPaths(paths), Components: newOrderedComponents(state.components)}
	content, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return OpenAPIBundle{}, fmt.Errorf("marshal OpenAPI bundle: %w", err)
	}
	content = append(content, '\n')
	digest := sha256.Sum256(content)
	return OpenAPIBundle{Content: content, SHA256: hex.EncodeToString(digest[:])}, nil
}

func WriteOpenAPIBundle(target string, bundle OpenAPIBundle) error {
	if len(bundle.Content) == 0 || bundle.SHA256 == "" {
		return fmt.Errorf("OpenAPI bundle content and SHA-256 are required")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("create OpenAPI bundle directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".openapi-bundle-*")
	if err != nil {
		return fmt.Errorf("create OpenAPI bundle temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(bundle.Content); err != nil {
		temporary.Close()
		return fmt.Errorf("write OpenAPI bundle temporary file: %w", err)
	}
	if err := temporary.Chmod(0644); err != nil {
		temporary.Close()
		return fmt.Errorf("set OpenAPI bundle permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close OpenAPI bundle temporary file: %w", err)
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return fmt.Errorf("replace OpenAPI bundle: %w", err)
	}
	return nil
}

type orderedOpenAPIBundle struct {
	OpenAPI    string            `json:"openapi"`
	Info       map[string]string `json:"info"`
	Paths      orderedPaths      `json:"paths"`
	Components orderedComponents `json:"components,omitempty"`
}

type orderedPaths map[string]map[string]json.RawMessage

func (p orderedPaths) MarshalJSON() ([]byte, error) {
	return marshalOrderedRawMessageMap(map[string]map[string]json.RawMessage(p))
}

type orderedComponents map[string]map[string]json.RawMessage

func (c orderedComponents) MarshalJSON() ([]byte, error) {
	return marshalOrderedRawMessageMap(map[string]map[string]json.RawMessage(c))
}

func marshalOrderedRawMessageMap(items map[string]map[string]json.RawMessage) ([]byte, error) {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]byte, 0)
	result = append(result, '{')
	for index, key := range keys {
		if index > 0 {
			result = append(result, ',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		result = append(result, encodedKey...)
		result = append(result, ':')
		value, err := marshalOrderedRawValues(items[key])
		if err != nil {
			return nil, err
		}
		result = append(result, value...)
	}
	result = append(result, '}')
	return result, nil
}

func marshalOrderedRawValues(items map[string]json.RawMessage) ([]byte, error) {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]byte, 0)
	result = append(result, '{')
	for index, key := range keys {
		if index > 0 {
			result = append(result, ',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		result = append(result, encodedKey...)
		result = append(result, ':')
		result = append(result, items[key]...)
	}
	result = append(result, '}')
	return result, nil
}

func newOrderedPaths(paths map[string]map[string]json.RawMessage) orderedPaths {
	return orderedPaths(paths)
}

func newOrderedComponents(components map[string]map[string][]byte) orderedComponents {
	result := make(orderedComponents, len(components))
	for kind, entries := range components {
		result[kind] = make(map[string]json.RawMessage, len(entries))
		for name, value := range entries {
			result[kind][name] = value
		}
	}
	return result
}

func splitOpenAPIRouteKey(key string) (string, string) {
	for index := 0; index < len(key); index++ {
		if key[index] == ' ' {
			return key[:index], key[index+1:]
		}
	}
	return key, ""
}
