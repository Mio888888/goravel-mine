package module

import (
	"os"
	"path/filepath"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
)

type ModuleManifestExportCommand struct{}

func (r *ModuleManifestExportCommand) Signature() string {
	return "module:manifest:export"
}

func (r *ModuleManifestExportCommand) Description() string {
	return "Export backend module manifest as JSON"
}

func (r *ModuleManifestExportCommand) Extend() command.Extend {
	return command.Extend{
		Category: "module",
		Flags: []command.Flag{
			&command.StringFlag{Name: "target", Usage: "Write manifest JSON to this path instead of stdout"},
		},
	}
}

func (r *ModuleManifestExportCommand) Handle(ctx console.Context) error {
	payload, err := modulecatalog.NewService(moduleboot.Modules()).ManifestJSON()
	if err != nil {
		ctx.Error(err.Error())
		return err
	}

	target := ctx.Option("target")
	if target == "" {
		_, err = os.Stdout.Write(append(payload, '\n'))
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		ctx.Error(err.Error())
		return err
	}
	if err := os.WriteFile(target, append(payload, '\n'), 0644); err != nil {
		ctx.Error(err.Error())
		return err
	}
	ctx.Success("module manifest exported: " + target)
	return nil
}
