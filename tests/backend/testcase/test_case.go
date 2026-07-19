package tests

import (
	contractsseeder "github.com/goravel/framework/contracts/database/seeder"
	"github.com/goravel/framework/testing"

	"goravel/app/services"
	"goravel/bootstrap"
)

func init() {
	bootstrap.Boot()
}

type TestCase struct {
	testing.TestCase
}

func (r *TestCase) RefreshDatabase(seeders ...contractsseeder.Seeder) {
	r.TestCase.RefreshDatabase(seeders...)
	services.ResetCasbinEnforcerCacheForTest()
}
