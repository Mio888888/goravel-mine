package commands

import (
	"fmt"
	"strings"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
)

type ModuleOpenAPILintCommand struct{}

func (r *ModuleOpenAPILintCommand) Signature() string { return "module:openapi:lint" }

func (r *ModuleOpenAPILintCommand) Description() string {
	return "Lint all module OpenAPI fragments and write a deterministic bundle"
}

func (r *ModuleOpenAPILintCommand) Extend() command.Extend {
	return command.Extend{Category: "module", Flags: []command.Flag{
		&command.BoolFlag{Name: "all", Usage: "Lint every registered module"},
		&command.StringFlag{Name: "bundle", Usage: "Atomic OpenAPI bundle output"},
	}}
}

func (r *ModuleOpenAPILintCommand) Handle(ctx console.Context) error {
	if !ctx.OptionBool("all") {
		return fmt.Errorf("module OpenAPI lint requires --all")
	}
	bundle, err := modulecatalog.BuildManifestOpenAPIBundle(modulecatalog.NewService(moduleboot.Modules()).Manifest())
	if err != nil {
		return err
	}
	target := strings.TrimSpace(ctx.Option("bundle"))
	if target == "" {
		return fmt.Errorf("module OpenAPI lint requires --bundle")
	}
	if err := modulecatalog.WriteOpenAPIBundle(target, bundle); err != nil {
		return err
	}
	ctx.Success("module OpenAPI bundle written: " + bundle.SHA256)
	return nil
}
