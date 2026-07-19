package messagebus

import (
	"context"
	"sync"

	"github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/support/apperror"
	"goravel/app/support/contextutil"
)

type ORMFactory func(context.Context, string) orm.Orm
type PlatformConnectionResolver func() string

type BusinessError = apperror.BusinessError

var messageBusDependencies = struct {
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

func ConfigureDependencies(ormFactory ORMFactory, platformConnection PlatformConnectionResolver) {
	messageBusDependencies.Lock()
	defer messageBusDependencies.Unlock()
	if ormFactory != nil {
		messageBusDependencies.ormFactory = ormFactory
	}
	if platformConnection != nil {
		messageBusDependencies.platformConnection = platformConnection
	}
}

func OrmForConnectionWithContext(ctx context.Context, connection string) orm.Orm {
	messageBusDependencies.RLock()
	factory := messageBusDependencies.ormFactory
	messageBusDependencies.RUnlock()
	return factory(contextOrBackground(ctx), connection)
}

func PlatformConnection() string {
	messageBusDependencies.RLock()
	resolve := messageBusDependencies.platformConnection
	messageBusDependencies.RUnlock()
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
