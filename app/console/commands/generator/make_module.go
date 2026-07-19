package generator

import (
	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/console/commands/modulegen"
	"goravel/app/facades"
)

type MakeModuleCommand struct{}

func (r *MakeModuleCommand) Signature() string {
	return "make:module"
}

func (r *MakeModuleCommand) Description() string {
	return "Generate backend and frontend scaffolding for a framework module"
}

func (r *MakeModuleCommand) Extend() command.Extend {
	return command.Extend{
		Category:  "make",
		ArgsUsage: "<name>",
		Arguments: []command.Argument{
			&command.ArgumentString{Name: "name", Required: true, Usage: "Module name, for example audit-log"},
		},
		Flags: []command.Flag{
			&command.StringFlag{Name: "path", Usage: "Output root path, defaults to application base path"},
			&command.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "Overwrite existing generated files"},
		},
	}
}

func (r *MakeModuleCommand) Handle(ctx console.Context) error {
	name := ctx.Argument(0)
	if name == "" {
		name = ctx.ArgumentString("name")
	}
	root := ctx.Option("path")
	if root == "" {
		root = facades.App().BasePath()
	}

	if err := modulegen.NewGenerator(root).Generate(modulegen.Options{
		Name:  name,
		Force: ctx.OptionBool("force"),
	}); err != nil {
		ctx.Error(err.Error())
		return err
	}

	ctx.Success("Module scaffolding generated successfully")
	return nil
}
