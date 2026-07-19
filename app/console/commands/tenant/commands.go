package tenant

import "github.com/goravel/framework/contracts/console"

func Commands() []console.Command {
	return []console.Command{
		&TenantPermissionsSnapshotLegacyCommand{},
	}
}
