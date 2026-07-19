package protection

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

var dependencies = struct {
	sync.RWMutex
	ormFactory         ORMFactory
	platformConnection PlatformConnectionResolver
}{
	ormFactory: func(ctx context.Context, connection string) orm.Orm {
		base := facades.Orm().WithContext(contextutil.OrBackground(ctx))
		if connection == "" {
			return base
		}
		return base.Connection(connection)
	},
	platformConnection: func() string {
		connection := facades.Config().GetString("tenant.platform_connection")
		if connection == "" {
			return facades.Config().GetString("database.default")
		}
		return connection
	},
}

func ConfigureDependencies(ormFactory ORMFactory, platformConnection PlatformConnectionResolver) {
	dependencies.Lock()
	defer dependencies.Unlock()
	if ormFactory != nil {
		dependencies.ormFactory = ormFactory
	}
	if platformConnection != nil {
		dependencies.platformConnection = platformConnection
	}
}

func ormForContext(ctx context.Context) orm.Orm {
	dependencies.RLock()
	factory := dependencies.ormFactory
	connection := dependencies.platformConnection
	dependencies.RUnlock()
	return factory(contextutil.OrBackground(ctx), connection())
}

func platformConnection() string {
	dependencies.RLock()
	resolve := dependencies.platformConnection
	dependencies.RUnlock()
	return resolve()
}
