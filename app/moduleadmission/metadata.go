package moduleadmission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const sourceManifestFile = "module-admission.json"

func ReadSourceModuleMetadata(sourceDir string) (SourceModuleMetadata, error) {
	payload, err := os.ReadFile(filepath.Join(sourceDir, sourceManifestFile))
	if err != nil {
		return SourceModuleMetadata{}, fmt.Errorf("read source admission manifest: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var metadata SourceModuleMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return SourceModuleMetadata{}, fmt.Errorf("decode source admission manifest: %w", err)
	}
	if metadata.ID == "" || metadata.Version == "" || metadata.GoImportPath == "" || metadata.GoModulePath == "" {
		return SourceModuleMetadata{}, fmt.Errorf("source admission manifest is incomplete")
	}
	return metadata, nil
}
