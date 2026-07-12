package commands

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
)

type ModuleLifecycleCommand struct{}

func (r *ModuleLifecycleCommand) Signature() string {
	return "module:lifecycle"
}

func (r *ModuleLifecycleCommand) Description() string {
	return "Dry-run module lifecycle orchestration"
}

func (r *ModuleLifecycleCommand) Extend() command.Extend {
	return command.Extend{
		Category: "module",
		Flags: []command.Flag{
			&command.StringFlag{Name: "action", Usage: "Lifecycle action: install, upgrade, rollback, uninstall", Value: "upgrade"},
			&command.StringFlag{Name: "module", Usage: "Limit execution to one module id"},
			&command.BoolFlag{Name: "execute", Usage: "Deprecated: direct execution is rejected; use the platform management API"},
			&command.StringFlag{Name: "owner", Usage: "Lifecycle run owner"},
			&command.StringFlag{Name: "reason", Usage: "Lifecycle run reason for audit"},
		},
	}
}

func (r *ModuleLifecycleCommand) Handle(ctx console.Context) error {
	if err := validateModuleLifecycleCLIExecution(ctx.OptionBool("execute")); err != nil {
		ctx.Error(err.Error())
		return err
	}
	action := ctx.Option("action")
	if action == "" {
		action = modulecatalog.LifecycleActionUpgrade
	}
	result, err := modulecatalog.NewLifecycleService(moduleboot.Modules()).Execute(context.Background(), action, modulecatalog.LifecycleOptions{
		ModuleID: ctx.Option("module"),
		Execute:  ctx.OptionBool("execute"),
		Owner:    ctx.Option("owner"),
		Reason:   ctx.Option("reason"),
	})
	if err != nil {
		if len(result.Items) > 0 {
			if writeErr := writeLifecycleResult(result); writeErr != nil {
				ctx.Error(writeErr.Error())
				return writeErr
			}
		}
		ctx.Error(err.Error())
		return err
	}

	return writeLifecycleResult(result)
}

func validateModuleLifecycleCLIExecution(execute bool) error {
	if execute {
		return errors.New("module lifecycle execution is restricted to the platform management API")
	}
	return nil
}

func writeLifecycleResult(result modulecatalog.LifecycleResult) error {
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(payload, '\n'))
	return err
}
