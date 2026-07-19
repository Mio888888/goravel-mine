package auth

import (
	"context"
	"sync"

	"github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/support/contextutil"
)

type ORMFactory func(context.Context, string) orm.Orm
type PlatformConnectionResolver func() string

var authDependencies = struct {
	sync.RWMutex
	ormFactory         ORMFactory
	platformConnection PlatformConnectionResolver
}{
	ormFactory: defaultORMFactory,
	platformConnection: func() string {
		connection := facades.Config().GetString("tenant.platform_connection")
		if connection == "" {
			return facades.Config().GetString("database.default")
		}
		return connection
	},
}

func ConfigureDependencies(factory ORMFactory, platformConnection PlatformConnectionResolver) {
	authDependencies.Lock()
	defer authDependencies.Unlock()
	if factory != nil {
		authDependencies.ormFactory = factory
	}
	if platformConnection != nil {
		authDependencies.platformConnection = platformConnection
	}
}

func OrmForConnectionWithContext(ctx context.Context, connection string) orm.Orm {
	authDependencies.RLock()
	factory := authDependencies.ormFactory
	authDependencies.RUnlock()
	return factory(contextOrBackground(ctx), connection)
}

func PlatformConnection() string {
	authDependencies.RLock()
	resolve := authDependencies.platformConnection
	authDependencies.RUnlock()
	return resolve()
}

func defaultORMFactory(ctx context.Context, connection string) orm.Orm {
	base := facades.Orm().WithContext(contextOrBackground(ctx))
	if connection == "" {
		return base
	}
	return base.Connection(connection)
}

func contextOrBackground(ctx context.Context) context.Context {
	return contextutil.OrBackground(ctx)
}
