package crudgen

import (
	"strings"
	"testing"
)

func TestRepositoryTemplateUsesSharedPagination(t *testing.T) {
	if !strings.Contains(repositoryTemplate, `request.Paginate[models.{{ .StructName }}]`) {
		t.Fatal("repository template should use shared pagination")
	}
	for _, legacy := range []string{".Count()", ".Offset(", ".Limit("} {
		if strings.Contains(repositoryTemplate, legacy) {
			t.Fatalf("repository template still contains manual pagination %q", legacy)
		}
	}
}
