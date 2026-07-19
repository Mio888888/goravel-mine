package sso

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvalConditionSupportsMappingSyntax(t *testing.T) {
	claims := map[string]any{
		"role":       "admin",
		"level":      float64(7),
		"department": "IT",
		"email":      "admin@company.com",
		"groups":     []any{"admin", "dev"},
	}

	cases := map[string]bool{
		"level >= 5 && (department == 'IT' || department == 'HR')":      true,
		"{{level}} >= 9 || {{department}} == 'IT'":                      true,
		"role in ['admin', 'manager'] && email contains '@company.com'": true,
		"email starts_with 'admin' && email ends_with '.com'":           true,
		"email not_matches '.*@example\\.com'":                          true,
		"role not_in ['guest', 'staff']":                                true,
		"!(department == 'HR')":                                         true,
		"groups contains 'ops' || role == 'guest'":                      false,
	}

	for condition, expected := range cases {
		require.Equal(t, expected, EvalCondition(condition, claims), condition)
	}
}
