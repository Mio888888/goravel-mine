package request

import (
	"errors"
	"strconv"
	"testing"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	contractshttp "github.com/goravel/framework/contracts/http"
)

type paginateQueryStub struct {
	contractsorm.Query
	paginate func(page, pageSize int, dest any, total *int64) error
}

type contextRequestStub struct {
	contractshttp.ContextRequest
	queries map[string]string
}

func (s contextRequestStub) Query(key string, defaultValue ...string) string {
	if value, ok := s.queries[key]; ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func (s contextRequestStub) QueryInt(key string, defaultValue ...int) int {
	value, ok := s.queries[key]
	if !ok {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func (s paginateQueryStub) Paginate(page, pageSize int, dest any, total *int64) error {
	return s.paginate(page, pageSize, dest, total)
}

func TestPaginateUsesDefaultsAndReturnsResult(t *testing.T) {
	query := paginateQueryStub{
		paginate: func(page, pageSize int, dest any, total *int64) error {
			if page != 1 || pageSize != 15 {
				t.Fatalf("Paginate() page=%d, pageSize=%d, want 1, 15", page, pageSize)
			}
			*dest.(*[]string) = []string{"first"}
			*total = 1
			return nil
		},
	}

	result, err := Paginate[string](query, 0, 0)

	if err != nil {
		t.Fatalf("Paginate() error = %v", err)
	}
	if len(result.List) != 1 || result.List[0] != "first" {
		t.Fatalf("Paginate() list = %#v, want [first]", result.List)
	}
	if result.Total != 1 {
		t.Fatalf("Paginate() total = %d, want 1", result.Total)
	}
}

func TestPaginatePropagatesErrorWithNonNilList(t *testing.T) {
	expected := errors.New("paginate failed")
	query := paginateQueryStub{
		paginate: func(page, pageSize int, _ any, _ *int64) error {
			if page != 2 || pageSize != 20 {
				t.Fatalf("Paginate() page=%d, pageSize=%d, want 2, 20", page, pageSize)
			}
			return expected
		},
	}

	result, err := Paginate[string](query, 2, 20)

	if !errors.Is(err, expected) {
		t.Fatalf("Paginate() error = %v, want %v", err, expected)
	}
	if result.List == nil {
		t.Fatal("Paginate() returned nil List")
	}
	if len(result.List) != 0 {
		t.Fatalf("Paginate() list = %#v, want empty", result.List)
	}
}

func TestPageAndPageSizeReadSupportedQueryNames(t *testing.T) {
	request := contextRequestStub{queries: map[string]string{
		"page":      "3",
		"page_size": "25",
	}}

	if value := Page(request); value != 3 {
		t.Fatalf("Page() = %d, want 3", value)
	}
	if value := PageSize(request); value != 25 {
		t.Fatalf("PageSize() = %d, want 25", value)
	}

	request.queries["per_page"] = "30"
	if value := PageSize(request); value != 30 {
		t.Fatalf("PageSize() with per_page = %d, want 30", value)
	}
}

func TestPageAndPageSizeUseDefaultsForInvalidValues(t *testing.T) {
	request := contextRequestStub{queries: map[string]string{
		"page":     "0",
		"per_page": "-1",
	}}

	if value := Page(request); value != DefaultPage {
		t.Fatalf("Page() = %d, want %d", value, DefaultPage)
	}
	if value := PageSize(request); value != DefaultPageSize {
		t.Fatalf("PageSize() = %d, want %d", value, DefaultPageSize)
	}
}
