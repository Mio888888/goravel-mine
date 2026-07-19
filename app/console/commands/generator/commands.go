package generator

import "github.com/goravel/framework/contracts/console"

func Commands() []console.Command {
	return []console.Command{
		&MakeCrudCommand{},
		&MakeModuleCommand{},
	}
}
