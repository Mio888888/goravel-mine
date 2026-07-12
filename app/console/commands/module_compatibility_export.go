package commands

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
	"goravel/app/modules"
)

type ModuleCompatibilityExportCommand struct{}

func (r *ModuleCompatibilityExportCommand) Signature() string {
	return "module:compatibility:export"
}

func (r *ModuleCompatibilityExportCommand) Description() string {
	return "Export module compatibility matrix as JSON"
}

func (r *ModuleCompatibilityExportCommand) Extend() command.Extend {
	return command.Extend{
		Category: "module",
		Flags: []command.Flag{
			&command.StringFlag{Name: "framework-version", Usage: "Goravel framework version used for compatibility evaluation"},
			&command.StringFlag{Name: "target", Usage: "Write compatibility matrix JSON to this path instead of stdout"},
		},
	}
}

func (r *ModuleCompatibilityExportCommand) Handle(ctx console.Context) error {
	matrix, err := exportCompatibilityMatrix(
		moduleboot.Modules(),
		ctx.Option("framework-version"),
		ctx.Option("target"),
		os.Stdout,
	)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	if matrix.Status != "passed" {
		return errors.New("module compatibility matrix failed")
	}
	if target := strings.TrimSpace(ctx.Option("target")); target != "" {
		ctx.Success("module compatibility matrix exported: " + target)
	}
	return nil
}

func exportCompatibilityMatrix(registry modules.Registry, frameworkVersion string, target string, output io.Writer) (modulecatalog.CompatibilityMatrix, error) {
	frameworkVersion = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(frameworkVersion), "v"))
	if frameworkVersion == "" {
		return modulecatalog.CompatibilityMatrix{}, errors.New("framework version is required")
	}
	service := modulecatalog.NewService(registry)
	matrix := service.CompatibilityMatrix(frameworkVersion)
	payload, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		return modulecatalog.CompatibilityMatrix{}, err
	}
	target = strings.TrimSpace(target)
	if target == "" {
		_, err = output.Write(append(payload, '\n'))
		return matrix, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return modulecatalog.CompatibilityMatrix{}, err
	}
	err = os.WriteFile(target, append(payload, '\n'), 0644)
	return matrix, err
}
