package commands

import (
	"encoding/json"
	"os"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
)

type ModulePlanCommand struct{}

func (r *ModulePlanCommand) Signature() string {
	return "module:plan"
}

func (r *ModulePlanCommand) Description() string {
	return "Export a dry-run module lifecycle plan as JSON"
}

func (r *ModulePlanCommand) Extend() command.Extend {
	return command.Extend{
		Category: "module",
		Flags: []command.Flag{
			&command.StringFlag{Name: "action", Usage: "Lifecycle action: install, upgrade, rollback, uninstall", Value: "upgrade"},
		},
	}
}

func (r *ModulePlanCommand) Handle(ctx console.Context) error {
	action := ctx.Option("action")
	if action == "" {
		action = "upgrade"
	}
	plan, err := modulecatalog.NewService(moduleboot.Modules()).LifecyclePlan(action)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}

	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		ctx.Error(err.Error())
		return err
	}

	_, err = os.Stdout.Write(append(payload, '\n'))
	return err
}
