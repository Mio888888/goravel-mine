package moduleadmission

import (
	"strings"
	"testing"
)

func TestLoadRepositoryIndexNormalizesEntriesDeterministically(t *testing.T) {
	index, err := LoadRepositoryIndex([]byte(`{
  "schema_version": "v1",
  "modules": [
    {"id":"reporting","version":"1.2.0","source_kind":"internal","source_uri":"bundle-b","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","dependencies":[{"id":"core","version_constraint":"1.0.0","required":true}]},
    {"id":"core","version":"1.0.0","source_kind":"internal","source_uri":"bundle-a","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
  ]
}`))
	if err != nil {
		t.Fatalf("LoadRepositoryIndex() error = %v", err)
	}
	if got := []string{index.Modules[0].ID, index.Modules[1].ID}; strings.Join(got, ",") != "core,reporting" {
		t.Fatalf("normalized IDs = %v, want core,reporting", got)
	}
	if index.Digest == "" {
		t.Fatal("normalized index digest is empty")
	}
}

func TestLoadRepositoryIndexRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(string) string
		contain string
	}{
		{
			name: "duplicate module version",
			mutate: func(source string) string {
				return strings.Replace(source, `]}`, `,{"id":"core","version":"1.0.0","source_kind":"internal","source_uri":"bundle-b","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}`, 1)
			},
			contain: "duplicate module version",
		},
		{
			name: "floating version",
			mutate: func(source string) string {
				return strings.Replace(source, `"1.0.0"`, `"latest"`, 1)
			},
			contain: "exact version",
		},
		{
			name: "unsupported URI",
			mutate: func(source string) string {
				return strings.Replace(source, `"bundle-a"`, `"ftp://registry.example/module"`, 1)
			},
			contain: "unsupported source URI",
		},
		{
			name: "invalid digest",
			mutate: func(source string) string {
				return strings.Replace(source, `sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`, `sha256:bad`, 1)
			},
			contain: "digest",
		},
		{
			name: "unknown field",
			mutate: func(source string) string {
				return strings.Replace(source, `"schema_version": "v1",`, `"schema_version": "v1", "unexpected": true,`, 1)
			},
			contain: "unknown field",
		},
		{
			name: "incomplete external evidence",
			mutate: func(source string) string {
				return strings.Replace(source, `"source_kind":"internal"`, `"source_kind":"external","cosign_issuer":"https://issuer.example"`, 1)
			},
			contain: "cosign identity",
		},
	}
	valid := `{"schema_version": "v1", "modules":[{"id":"core","version":"1.0.0","source_kind":"internal","source_uri":"bundle-a","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadRepositoryIndex([]byte(test.mutate(valid)))
			if err == nil || !strings.Contains(err.Error(), test.contain) {
				t.Fatalf("LoadRepositoryIndex() error = %v, want %q", err, test.contain)
			}
		})
	}
}
