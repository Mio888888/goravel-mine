package commands

import (
	"fmt"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/services"
)

type TenantPermissionsSnapshotLegacyCommand struct{}

func (r *TenantPermissionsSnapshotLegacyCommand) Signature() string {
	return "tenant:permissions:snapshot-legacy"
}

func (r *TenantPermissionsSnapshotLegacyCommand) Description() string {
	return "Generate explicit full permission snapshots for legacy tenants without permission snapshots"
}

func (r *TenantPermissionsSnapshotLegacyCommand) Extend() command.Extend {
	return command.Extend{
		Category: "tenant",
		Flags: []command.Flag{
			&command.BoolFlag{Name: "dry-run", Usage: "Count tenants that would be snapshotted without writing changes"},
		},
	}
}

func (r *TenantPermissionsSnapshotLegacyCommand) Handle(ctx console.Context) error {
	dryRun := ctx.OptionBool("dry-run")
	count, err := services.NewTenantService().SnapshotLegacyPermissions(dryRun)
	if err != nil {
		ctx.Error(err.Error())
		return nil
	}
	if dryRun {
		ctx.Success(fmt.Sprintf("%d legacy tenant permission snapshots would be generated", count))
		return nil
	}
	ctx.Success(fmt.Sprintf("%d legacy tenant permission snapshots generated", count))
	return nil
}
