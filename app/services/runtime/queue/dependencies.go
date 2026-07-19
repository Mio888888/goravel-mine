package queue

import (
	"context"
	"sync"

	"github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/support/apperror"
	"goravel/app/support/contextutil"
	"goravel/app/support/token"
)

type ORMFactory func(context.Context, string) orm.Orm

type BusinessError = apperror.BusinessError

var (
	ormFactoryMu sync.RWMutex
	ormFactory   ORMFactory = defaultORMFactory
)

func ConfigureORMFactory(factory ORMFactory) {
	if factory == nil {
		return
	}
	ormFactoryMu.Lock()
	defer ormFactoryMu.Unlock()
	ormFactory = factory
}

func OrmForConnectionWithContext(ctx context.Context, connection string) orm.Orm {
	ormFactoryMu.RLock()
	factory := ormFactory
	ormFactoryMu.RUnlock()
	return factory(contextOrBackground(ctx), connection)
}

func OrmWithContext(ctx context.Context) orm.Orm {
	return OrmForConnectionWithContext(ctx, "")
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

func randomRunToken() string {
	return token.RandomHex(16)
}

func stringAny(values []string) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return items
}
