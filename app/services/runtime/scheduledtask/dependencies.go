package scheduledtask

import (
	"context"
	"sync"

	"github.com/goravel/framework/contracts/database/orm"

	tenantcontract "goravel/app/contracts/tenant"
	"goravel/app/facades"
	"goravel/app/support/apperror"
	"goravel/app/support/contextutil"
)

type ORMFactory func(context.Context, string) orm.Orm
type PlatformConnectionResolver func() string
type GovernanceEventRecorder func(context.Context, map[string]any)

type BusinessError = apperror.BusinessError
type Tenant = tenantcontract.Tenant

const TenantStatusActive = tenantcontract.StatusActive

var scheduledTaskDependencies = struct {
	sync.RWMutex
	ormFactory         ORMFactory
	platformConnection PlatformConnectionResolver
	recordGovernance   GovernanceEventRecorder
}{
	ormFactory: defaultORMFactory,
	platformConnection: func() string {
		connection := facades.Config().GetString("tenant.platform_connection")
		if connection == "" {
			return facades.Config().GetString("database.default")
		}
		return connection
	},
	recordGovernance: func(context.Context, map[string]any) {},
}

func ConfigureDependencies(
	ormFactory ORMFactory,
	platformConnection PlatformConnectionResolver,
	recordGovernance GovernanceEventRecorder,
) {
	scheduledTaskDependencies.Lock()
	defer scheduledTaskDependencies.Unlock()
	if ormFactory != nil {
		scheduledTaskDependencies.ormFactory = ormFactory
	}
	if platformConnection != nil {
		scheduledTaskDependencies.platformConnection = platformConnection
	}
	if recordGovernance != nil {
		scheduledTaskDependencies.recordGovernance = recordGovernance
	}
}

func OrmForConnectionWithContext(ctx context.Context, connection string) orm.Orm {
	scheduledTaskDependencies.RLock()
	factory := scheduledTaskDependencies.ormFactory
	scheduledTaskDependencies.RUnlock()
	return factory(contextOrBackground(ctx), connection)
}

func PlatformConnection() string {
	scheduledTaskDependencies.RLock()
	resolve := scheduledTaskDependencies.platformConnection
	scheduledTaskDependencies.RUnlock()
	return resolve()
}

func RecordTenantGovernanceEvent(ctx context.Context, fields map[string]any) {
	scheduledTaskDependencies.RLock()
	record := scheduledTaskDependencies.recordGovernance
	scheduledTaskDependencies.RUnlock()
	record(contextOrBackground(ctx), fields)
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
