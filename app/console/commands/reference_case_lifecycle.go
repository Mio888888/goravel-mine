package commands

import (
	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/services"
)

type ReferenceCaseUpgradeCommand struct{}

func (r *ReferenceCaseUpgradeCommand) Signature() string {
	return "reference-case:upgrade"
}

func (r *ReferenceCaseUpgradeCommand) Description() string {
	return "Apply the golden reference module upgrade example"
}

func (r *ReferenceCaseUpgradeCommand) Extend() command.Extend {
	return command.Extend{Category: "reference-case"}
}

func (r *ReferenceCaseUpgradeCommand) Handle(ctx console.Context) error {
	if err := services.ApplyReferenceCaseUpgrade(nil); err != nil {
		ctx.Error(err.Error())
		return err
	}
	ctx.Success("reference-case upgraded to 1.1.0")
	return nil
}

type ReferenceCaseRollbackCommand struct{}

func (r *ReferenceCaseRollbackCommand) Signature() string {
	return "reference-case:rollback"
}

func (r *ReferenceCaseRollbackCommand) Description() string {
	return "Rollback the golden reference module upgrade example"
}

func (r *ReferenceCaseRollbackCommand) Extend() command.Extend {
	return command.Extend{Category: "reference-case"}
}

func (r *ReferenceCaseRollbackCommand) Handle(ctx console.Context) error {
	if err := services.RollbackReferenceCaseUpgrade(nil); err != nil {
		ctx.Error(err.Error())
		return err
	}
	ctx.Success("reference-case rolled back to 1.0.0")
	return nil
}
