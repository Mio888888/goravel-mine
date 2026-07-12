package services

import (
	"context"
	"sync"

	"github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
)

var tenantORMConnectionMu sync.Mutex

func Orm() orm.Orm {
	return facades.Orm()
}

func OrmForConnection(connection string) orm.Orm {
	return OrmForConnectionWithContext(context.Background(), connection)
}

func OrmWithContext(ctx context.Context) orm.Orm {
	return OrmForConnectionWithContext(ctx, "")
}

func OrmForConnectionWithContext(ctx context.Context, connection string) orm.Orm {
	ctx = contextOrBackground(ctx)
	base := facades.Orm().WithContext(ctx)
	if connection == "" {
		return base
	}
	if TenantConnectionRegistered(connection) {
		tenantORMConnectionMu.Lock()
		defer tenantORMConnectionMu.Unlock()
	}
	instance := base.Connection(connection)
	configureTenantConnectionPool(connection, instance)
	return instance
}

func configureTenantConnectionPool(connection string, instance orm.Orm) {
	if !TenantConnectionRegistered(connection) || instance.Query() == nil {
		return
	}
	database, err := instance.DB()
	if err == nil {
		database.SetMaxIdleConns(0)
	}
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func TenantConnectionFromContext(ctx context.Context) string {
	tenant, ok := CurrentTenant(ctx)
	if !ok {
		return ""
	}
	return TenantConnectionName(tenant)
}
