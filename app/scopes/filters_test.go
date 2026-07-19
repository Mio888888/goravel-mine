package scopes

import (
	"testing"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"github.com/stretchr/testify/require"
)

func TestEqualSkipsBlankValue(t *testing.T) {
	query := &querySpy{}

	result := Equal("status", "  ")(query)

	require.Same(t, query, result)
	require.Empty(t, query.calls)
}

func TestEqualAddsWhereClause(t *testing.T) {
	query := &querySpy{}

	result := Equal("status", "enabled")(query)

	require.Same(t, query, result)
	require.Equal(t, []whereCall{{query: "status", args: []any{"enabled"}}}, query.calls)
}

func TestEqualIfPresentPreservesWhitespaceValue(t *testing.T) {
	query := &querySpy{}

	result := EqualIfPresent("status", "  ")(query)

	require.Same(t, query, result)
	require.Equal(t, []whereCall{{query: "status", args: []any{"  "}}}, query.calls)
}

func TestContainsAddsLikeClause(t *testing.T) {
	query := &querySpy{}

	result := Contains("name", "admin")(query)

	require.Same(t, query, result)
	require.Equal(t, []whereCall{{query: "name LIKE ?", args: []any{"%admin%"}}}, query.calls)
}

func TestContainsFoldAddsCaseInsensitiveClause(t *testing.T) {
	query := &querySpy{}

	result := ContainsFold("name", "Admin")(query)

	require.Same(t, query, result)
	require.Equal(t, []whereCall{{query: "name ILIKE ?", args: []any{"%Admin%"}}}, query.calls)
}

func TestContainsFoldIfPresentPreservesWhitespaceValue(t *testing.T) {
	query := &querySpy{}

	result := ContainsFoldIfPresent("name", "  ")(query)

	require.Same(t, query, result)
	require.Equal(t, []whereCall{{query: "name ILIKE ?", args: []any{"%  %"}}}, query.calls)
}

func TestRangeScopesAddBoundaryClauses(t *testing.T) {
	query := &querySpy{}

	require.Same(t, query, GreaterThanOrEqual("created_at", "2026-07-01")(query))
	require.Same(t, query, LessThanOrEqual("created_at", "2026-07-31")(query))
	require.Equal(t, []whereCall{
		{query: "created_at >= ?", args: []any{"2026-07-01"}},
		{query: "created_at <= ?", args: []any{"2026-07-31"}},
	}, query.calls)
}

type whereCall struct {
	query any
	args  []any
}

type querySpy struct {
	contractsorm.Query
	calls []whereCall
}

func (q *querySpy) Where(query any, args ...any) contractsorm.Query {
	q.calls = append(q.calls, whereCall{query: query, args: args})
	return q
}
