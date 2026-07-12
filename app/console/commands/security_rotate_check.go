package commands

import (
	"fmt"
	"time"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/services"
)

type SecurityRotateCheckCommand struct{}

func (r *SecurityRotateCheckCommand) Signature() string {
	return "security:rotate-check"
}

func (r *SecurityRotateCheckCommand) Description() string {
	return "Check secret rotation metadata without changing environment files"
}

func (r *SecurityRotateCheckCommand) Extend() command.Extend {
	return command.Extend{Category: "security"}
}

func (r *SecurityRotateCheckCommand) Handle(ctx console.Context) error {
	findings := services.CheckKeyRotation(time.Now())
	dbFindings, err := services.CheckDatabaseKeyRotation(nil, time.Now())
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	findings = append(findings, dbFindings...)
	if len(findings) == 0 {
		ctx.Success("no configured secrets require rotation metadata check")
		return nil
	}
	for _, finding := range findings {
		ctx.Info(fmt.Sprintf("%s [%s] %s", finding.Name, finding.Status, finding.Message))
	}
	return nil
}
