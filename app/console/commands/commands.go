package commands

import (
	"github.com/goravel/framework/contracts/console"

	"goravel/app/console/commands/generator"
	"goravel/app/console/commands/migration"
	modulecommands "goravel/app/console/commands/module"
	securitycommands "goravel/app/console/commands/security"
	tenantcommands "goravel/app/console/commands/tenant"
	"goravel/app/console/commands/testsupport"
)

func All() []console.Command {
	groups := [][]console.Command{
		generator.Commands(),
		migration.Commands(),
		testsupport.Commands(),
		tenantcommands.Commands(),
		securitycommands.Commands(),
		modulecommands.Commands(),
	}
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	commands := make([]console.Command, 0, total)
	for _, group := range groups {
		commands = append(commands, group...)
	}
	return commands
}
