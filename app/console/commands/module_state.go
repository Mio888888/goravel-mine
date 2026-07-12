package commands

import (
	"encoding/json"
	"os"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
)

type ModuleStateCommand struct{}

func (r *ModuleStateCommand) Signature() string {
	return "module:state"
}

func (r *ModuleStateCommand) Description() string {
	return "Export module enabled states as JSON"
}

func (r *ModuleStateCommand) Extend() command.Extend {
	return command.Extend{Category: "module"}
}

func (r *ModuleStateCommand) Handle(ctx console.Context) error {
	state, err := modulecatalog.NewService(moduleboot.Modules()).ModuleStateManifest()
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		ctx.Error(err.Error())
		return err
	}

	_, err = os.Stdout.Write(append(payload, '\n'))
	return err
}
