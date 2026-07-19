package dictionary

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
type TenantConnectionRegistrar func(Tenant) string

type BusinessError = apperror.BusinessError
type Tenant = tenantcontract.Tenant

const TenantStatusActive = tenantcontract.StatusActive

var ErrTenantNotFound = tenantcontract.ErrNotFound

var dictionaryDependencies = struct {
	sync.RWMutex
	ormFactory         ORMFactory
	platformConnection PlatformConnectionResolver
	registerTenant     TenantConnectionRegistrar
}{
	ormFactory: defaultORMFactory,
	platformConnection: func() string {
		connection := facades.Config().GetString("tenant.platform_connection")
		if connection == "" {
			return facades.Config().GetString("database.default")
		}
		return connection
	},
	registerTenant: func(tenant Tenant) string {
		return tenantcontract.ConnectionName(tenant)
	},
}

func ConfigureDependencies(
	ormFactory ORMFactory,
	platformConnection PlatformConnectionResolver,
	registerTenant TenantConnectionRegistrar,
) {
	dictionaryDependencies.Lock()
	defer dictionaryDependencies.Unlock()
	if ormFactory != nil {
		dictionaryDependencies.ormFactory = ormFactory
	}
	if platformConnection != nil {
		dictionaryDependencies.platformConnection = platformConnection
	}
	if registerTenant != nil {
		dictionaryDependencies.registerTenant = registerTenant
	}
}

func OrmForConnectionWithContext(ctx context.Context, connection string) orm.Orm {
	dictionaryDependencies.RLock()
	factory := dictionaryDependencies.ormFactory
	dictionaryDependencies.RUnlock()
	return factory(contextOrBackground(ctx), connection)
}

func PlatformConnection() string {
	dictionaryDependencies.RLock()
	resolve := dictionaryDependencies.platformConnection
	dictionaryDependencies.RUnlock()
	return resolve()
}

func RegisterTenantConnection(tenant Tenant) string {
	dictionaryDependencies.RLock()
	register := dictionaryDependencies.registerTenant
	dictionaryDependencies.RUnlock()
	return register(tenant)
}

func TenantConnectionName(tenant Tenant) string {
	return tenantcontract.ConnectionName(tenant)
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

func uint64Any(values []uint64) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}
