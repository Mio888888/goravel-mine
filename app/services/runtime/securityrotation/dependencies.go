package securityrotation

import (
	"context"
	"sync"

	"github.com/goravel/framework/contracts/database/orm"

	tenantcontract "goravel/app/contracts/tenant"
	"goravel/app/facades"
	"goravel/app/support/contextutil"
)

type ORMFactory func(context.Context, string) orm.Orm
type PlatformConnectionResolver func() string
type TenantConnectionRegistrar func(Tenant) string

var securityRotationDependencies = struct {
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
	securityRotationDependencies.Lock()
	defer securityRotationDependencies.Unlock()
	if ormFactory != nil {
		securityRotationDependencies.ormFactory = ormFactory
	}
	if platformConnection != nil {
		securityRotationDependencies.platformConnection = platformConnection
	}
	if registerTenant != nil {
		securityRotationDependencies.registerTenant = registerTenant
	}
}

func OrmForConnectionWithContext(ctx context.Context, connection string) orm.Orm {
	securityRotationDependencies.RLock()
	factory := securityRotationDependencies.ormFactory
	securityRotationDependencies.RUnlock()
	return factory(contextutil.OrBackground(ctx), connection)
}

func PlatformConnection() string {
	securityRotationDependencies.RLock()
	resolve := securityRotationDependencies.platformConnection
	securityRotationDependencies.RUnlock()
	return resolve()
}

func RegisterTenantConnection(tenant Tenant) string {
	securityRotationDependencies.RLock()
	register := securityRotationDependencies.registerTenant
	securityRotationDependencies.RUnlock()
	return register(tenant)
}

func TenantConnectionName(tenant Tenant) string {
	return tenantcontract.ConnectionName(tenant)
}

func defaultORMFactory(ctx context.Context, connection string) orm.Orm {
	base := facades.Orm().WithContext(contextutil.OrBackground(ctx))
	if connection == "" {
		return base
	}
	return base.Connection(connection)
}
