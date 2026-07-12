package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/models"
)

func TestCasbinPolicyLineTrimsUnusedValues(t *testing.T) {
	line, err := casbinPolicyLine(models.CasbinRule{Ptype: "p", V0: "role:Auditor", V1: "permission:user:index", V2: "*"})

	require.NoError(t, err)
	require.Equal(t, []string{"p", "role:Auditor", "permission:user:index", "*"}, line)
}

func TestCasbinPolicyLineRejectsEmptyPolicyType(t *testing.T) {
	_, err := casbinPolicyLine(models.CasbinRule{})

	require.EqualError(t, err, "ptype is empty")
}
