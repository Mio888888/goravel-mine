package commands

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllKeepsCommandRegistrationOrder(t *testing.T) {
	commands := All()
	signatures := make([]string, 0, len(commands))
	for _, item := range commands {
		signatures = append(signatures, item.Signature())
	}

	require.Equal(t, []string{
		"make:crud",
		"make:module",
		"tenant:migrate",
		"migration:safe",
		"tenant:permissions:snapshot-legacy",
		"security:audit-prune",
		"security:rotate-check",
		"module:manifest:check",
		"module:admission:check",
		"module:openapi:lint",
		"module:manifest:export",
		"module:compatibility:export",
		"module:state",
		"module:plan",
		"module:lifecycle",
		"reference-case:upgrade",
		"reference-case:rollback",
	}, signatures)
}
