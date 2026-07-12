package moduleadmission

import (
	"strings"
	"testing"
)

func TestResolverProducesDeterministicExactDependencyOrder(t *testing.T) {
	index := mustIndex(t, `{"schema_version":"v1","modules":[
		{"id":"reporting","version":"1.0.0","source_kind":"internal","source_uri":"reporting.bundle","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","go_import_path":"example.com/modules/reporting","dependencies":[{"id":"core","version_constraint":"1.0.0","required":true}]},
		{"id":"core","version":"1.0.0","source_kind":"internal","source_uri":"core.bundle","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","go_import_path":"example.com/modules/core"}
	]}`)
	resolution, err := Resolve(index, []ModuleReference{{ID: "reporting", Version: "1.0.0"}}, []SourceModuleMetadata{
		{ID: "core", Version: "1.0.0", GoImportPath: "example.com/modules/core"},
		{ID: "reporting", Version: "1.0.0", GoImportPath: "example.com/modules/reporting", Dependencies: []IndexDependency{{ID: "core", VersionConstraint: "1.0.0", Required: true}}},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got := resolution.ModuleIDs(); strings.Join(got, ",") != "core,reporting" {
		t.Fatalf("resolved IDs = %v, want core,reporting", got)
	}
}

func TestResolverRejectsUnsafeDependencyGraphs(t *testing.T) {
	tests := []struct {
		name     string
		index    string
		sources  []SourceModuleMetadata
		contains string
	}{
		{
			name: "missing dependency", index: `[{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","dependencies":[{"id":"missing","version_constraint":"1.0.0","required":true}]}]`, contains: "missing dependency",
		},
		{
			name: "version conflict", index: `[{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","dependencies":[{"id":"core","version_constraint":">=1.0.0","required":true}]},{"id":"core","version":"1.0.0","source_kind":"internal","source_uri":"core","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]`, contains: "exact version",
		},
		{
			name: "dependency cycle", index: `[{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","dependencies":[{"id":"beta","version_constraint":"1.0.0","required":true}]},{"id":"beta","version":"1.0.0","source_kind":"internal","source_uri":"beta","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","dependencies":[{"id":"alpha","version_constraint":"1.0.0","required":true}]}]`, contains: "dependency cycle",
		},
		{
			name: "source index mismatch", index: `[{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]`, sources: []SourceModuleMetadata{{ID: "alpha", Version: "1.0.1"}}, contains: "source metadata mismatch",
		},
		{
			name: "Go module graph mismatch", index: `[{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","go_import_path":"example.com/modules/alpha"}]`, sources: []SourceModuleMetadata{{ID: "alpha", Version: "1.0.0", GoImportPath: "example.com/modules/alpha", GoModulePath: "example.com/other"}}, contains: "source metadata mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index := mustIndex(t, `{"schema_version":"v1","modules":`+test.index+`}`)
			_, err := Resolve(index, []ModuleReference{{ID: "alpha", Version: "1.0.0"}}, test.sources)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Resolve() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestResolverRejectsTwoVersionsOfSameModule(t *testing.T) {
	index := mustIndex(t, `{"schema_version":"v1","modules":[
		{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","dependencies":[{"id":"core","version_constraint":"1.0.0","required":true}]},
		{"id":"beta","version":"1.0.0","source_kind":"internal","source_uri":"beta","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","dependencies":[{"id":"core","version_constraint":"2.0.0","required":true}]},
		{"id":"core","version":"1.0.0","source_kind":"internal","source_uri":"core-1","digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
		{"id":"core","version":"2.0.0","source_kind":"internal","source_uri":"core-2","digest":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}
	]}`)
	_, err := Resolve(index, []ModuleReference{{ID: "alpha", Version: "1.0.0"}, {ID: "beta", Version: "1.0.0"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "module version conflict") {
		t.Fatalf("Resolve() error = %v, want module version conflict", err)
	}
}

func mustIndex(t *testing.T, payload string) RepositoryIndex {
	t.Helper()
	index, err := LoadRepositoryIndex([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	return index
}
