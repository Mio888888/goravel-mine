package unit

import (
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
)

func TestDataPermission(t *testing.T) {
	t.Run("policy precedence prefers user policy", func(t *testing.T) {
		policy := services.ResolveDataPolicy(
			services.DataPolicy{Type: services.PolicyDeptSelf, DeptIDs: []uint64{1}},
			[]services.DataPolicy{{Type: services.PolicyAll}},
		)
		require.Equal(t, services.PolicyDeptSelf, policy.Type)
	})

	t.Run("falls back to first position policy", func(t *testing.T) {
		policy := services.ResolveDataPolicy(
			services.DataPolicy{},
			[]services.DataPolicy{
				{Type: services.PolicyDeptTree, DeptIDs: []uint64{2, 3}},
				{Type: services.PolicyAll},
			},
		)
		require.Equal(t, services.PolicyDeptTree, policy.Type)
		require.Equal(t, []uint64{2, 3}, policy.DeptIDs)
	})

	t.Run("self filters to owner id", func(t *testing.T) {
		scope, err := services.BuildDataScope(services.DataPolicy{Type: services.PolicySelf}, services.DataScopeContext{
			UserID: 9, OwnerColumn: "created_by",
		})
		require.NoError(t, err)
		require.Equal(t, "created_by = ?", scope.Condition)
		require.Equal(t, []any{uint64(9)}, scope.Args)
	})

	t.Run("custom func is rejected", func(t *testing.T) {
		_, err := services.BuildDataScope(services.DataPolicy{Type: services.PolicyCustomFunc}, services.DataScopeContext{UserID: 1})
		require.ErrorIs(t, err, services.ErrBusinessRule)
	})

	t.Run("department policies with no resolved department deny by empty set", func(t *testing.T) {
		scope, err := services.BuildDataScope(services.DataPolicy{Type: services.PolicyDeptSelf}, services.DataScopeContext{
			UserID: 9, DeptColumn: "dept_id",
		})
		require.NoError(t, err)
		require.Equal(t, "1 = 0", scope.Condition)
		require.Empty(t, scope.Args)
	})
}
