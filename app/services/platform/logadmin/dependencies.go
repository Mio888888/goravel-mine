package logadmin

import (
	"context"
	"sync"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/support/contextutil"
)

type ORMFactory func(context.Context, string) contractsorm.Orm

var logAdminDependencies = struct {
	sync.RWMutex
	ormFactory ORMFactory
}{
	ormFactory: defaultORMFactory,
}

func ConfigureORMFactory(factory ORMFactory) {
	if factory == nil {
		return
	}
	logAdminDependencies.Lock()
	defer logAdminDependencies.Unlock()
	logAdminDependencies.ormFactory = factory
}

func NewServiceForConnection(connection string) *LogAdminService {
	logAdminDependencies.RLock()
	factory := logAdminDependencies.ormFactory
	logAdminDependencies.RUnlock()
	return NewService(connection, factory)
}

func defaultORMFactory(ctx context.Context, connection string) contractsorm.Orm {
	base := facades.Orm().WithContext(contextutil.OrBackground(ctx))
	if connection == "" {
		return base
	}
	return base.Connection(connection)
}
