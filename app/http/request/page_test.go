package request

import (
	"errors"
	"testing"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

type paginateQueryStub struct {
	contractsorm.Query
	paginate func(page, pageSize int, dest any, total *int64) error
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
