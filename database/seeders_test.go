package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedersRegistersPlatformComponents(t *testing.T) {
	signatures := make(map[string]struct{})
	for _, item := range Seeders(nil) {
		signatures[item.Signature()] = struct{}{}
	}

	for _, signature := range []string{
		"platform_dictionary_seed",
		"platform_admin_seed",
		"platform_menu_seed",
		"platform_casbin_seed",
	} {
		_, exists := signatures[signature]
		require.True(t, exists, "seeder %s must be registered", signature)
	}
}
