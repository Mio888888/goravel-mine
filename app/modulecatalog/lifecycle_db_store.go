package modulecatalog

import (
	"context"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
)

type DBLifecycleStore struct {
	connection string
	nowFunc    func() time.Time
}

func NewDBLifecycleStore(connection string) *DBLifecycleStore {
	return &DBLifecycleStore{connection: strings.TrimSpace(connection), nowFunc: time.Now}
}

func (s *DBLifecycleStore) now() time.Time {
	if s.nowFunc == nil {
		return time.Now()
	}
	return s.nowFunc()
}

func (s *DBLifecycleStore) orm(ctx context.Context) contractsorm.Orm {
	base := facades.Orm().WithContext(contextOrBackground(ctx))
	if s.connection == "" {
		return base
	}
	return base.Connection(s.connection)
}
