package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectMenuDeleteTargetsIncludesAllDescendants(t *testing.T) {
	menus := []menuDeleteRow{
		{ID: 1, ParentID: 0, Name: "root"},
		{ID: 2, ParentID: 1, Name: "child"},
		{ID: 3, ParentID: 2, Name: "grandchild"},
		{ID: 4, ParentID: 3, Name: "button"},
		{ID: 5, ParentID: 0, Name: "other"},
	}

	ids, names := collectMenuDeleteTargets(menus, []uint64{1})

	require.ElementsMatch(t, []uint64{1, 2, 3, 4}, ids)
	require.ElementsMatch(t, []string{"root", "child", "grandchild", "button"}, names)
}

func TestCollectMenuDeleteTargetsDeduplicatesOverlappingRoots(t *testing.T) {
	menus := []menuDeleteRow{
		{ID: 1, ParentID: 0, Name: "root"},
		{ID: 2, ParentID: 1, Name: "child"},
		{ID: 3, ParentID: 2, Name: "grandchild"},
	}

	ids, names := collectMenuDeleteTargets(menus, []uint64{1, 2})

	require.ElementsMatch(t, []uint64{1, 2, 3}, ids)
	require.ElementsMatch(t, []string{"root", "child", "grandchild"}, names)
}
