package migrationlock

import (
	"context"
	"sync"

	"github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	observabilityservice "goravel/app/services/runtime/observability"
	"goravel/app/support/contextutil"
)

type ORMFactory func(context.Context, string) orm.Orm
type PlatformConnectionResolver func() string
type AuditRecorder func(context.Context, AuditEvent)

var migrationLockDependencies = struct {
	sync.RWMutex
	ormFactory         ORMFactory
	platformConnection PlatformConnectionResolver
	audit              AuditRecorder
}{
	ormFactory: defaultORMFactory,
	platformConnection: func() string {
		connection := facades.Config().GetString("tenant.platform_connection")
		if connection == "" {
			return facades.Config().GetString("database.default")
		}
		return connection
	},
	audit: observabilityservice.RecordAuditEvent,
}

func ConfigureDependencies(
	ormFactory ORMFactory,
	platformConnection PlatformConnectionResolver,
	audit AuditRecorder,
) {
	migrationLockDependencies.Lock()
	defer migrationLockDependencies.Unlock()
	if ormFactory != nil {
		migrationLockDependencies.ormFactory = ormFactory
	}
	if platformConnection != nil {
		migrationLockDependencies.platformConnection = platformConnection
	}
	if audit != nil {
		migrationLockDependencies.audit = audit
	}
}

func OrmForConnectionWithContext(ctx context.Context, connection string) orm.Orm {
	migrationLockDependencies.RLock()
	factory := migrationLockDependencies.ormFactory
	migrationLockDependencies.RUnlock()
	return factory(contextOrBackground(ctx), connection)
}

func PlatformConnection() string {
	migrationLockDependencies.RLock()
	resolve := migrationLockDependencies.platformConnection
	migrationLockDependencies.RUnlock()
	return resolve()
}

func RecordAuditEvent(ctx context.Context, event AuditEvent) {
	migrationLockDependencies.RLock()
	record := migrationLockDependencies.audit
	migrationLockDependencies.RUnlock()
	record(contextOrBackground(ctx), event)
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
