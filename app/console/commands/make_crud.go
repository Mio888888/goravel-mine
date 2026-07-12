package commands

import (
	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/console/commands/crudgen"
	"goravel/app/facades"
)

type MakeCrudCommand struct{}

func (r *MakeCrudCommand) Signature() string {
	return "make:crud"
}

func (r *MakeCrudCommand) Description() string {
	return "Generate MineAdmin compatible CRUD scaffolding from a PostgreSQL table"
}

func (r *MakeCrudCommand) Extend() command.Extend {
	return command.Extend{
		Category:  "make",
		ArgsUsage: "<table>",
		Arguments: []command.Argument{
			&command.ArgumentString{Name: "table", Required: true, Usage: "PostgreSQL table name"},
		},
		Flags: []command.Flag{
			&command.StringFlag{Name: "module", Value: "system", Usage: "Generated module name"},
			&command.StringFlag{Name: "path", Usage: "Output root path, defaults to application base path"},
			&command.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "Overwrite existing generated files"},
		},
	}
}

func (r *MakeCrudCommand) Handle(ctx console.Context) error {
	table := ctx.Argument(0)
	if table == "" {
		table = ctx.ArgumentString("table")
	}
	root := ctx.Option("path")
	if root == "" {
		root = facades.App().BasePath()
	}
	err := crudgen.NewGenerator(facades.Orm(), root).Generate(crudgen.Options{
		Table:  table,
		Module: ctx.Option("module"),
		Force:  ctx.OptionBool("force"),
	})
	if err != nil {
		ctx.Error(err.Error())
		return nil
	}
	ctx.Success("CRUD scaffolding generated successfully")
	return nil
}
