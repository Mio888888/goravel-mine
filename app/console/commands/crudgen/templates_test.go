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

func TestTemplatesReuseSharedRequestAndIDHelpers(t *testing.T) {
	for _, expected := range []string{
		`sharedRequest.Page(ctx.Request())`,
		`sharedRequest.PageSize(ctx.Request())`,
		`collect.Map(ids, func(id uint64, _ int) any`,
	} {
		if !strings.Contains(controllerTemplate+repositoryTemplate, expected) {
			t.Fatalf("templates should reuse %q", expected)
		}
	}
	for _, duplicated := range []string{
		`func page(ctx contractshttp.Context) int`,
		`func pageSize(ctx contractshttp.Context) int`,
		`values := make([]any, 0, len(ids))`,
	} {
		if strings.Contains(controllerTemplate+repositoryTemplate, duplicated) {
			t.Fatalf("templates still contain duplicated helper %q", duplicated)
		}
	}
}
