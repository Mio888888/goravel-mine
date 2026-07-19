package modulecatalog

import (
	"context"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/http/request"
	"goravel/app/scopes"
)

type adminPageRequest struct {
	Filters  map[string]string
	Page     int
	PageSize int
}

type adminEqualColumn struct {
	filter string
	column string
}

type adminListSpec struct {
	table   string
	orderBy string
	filters []adminEqualColumn
}

type adminListQuery[TRecord, TDTO any] struct {
	spec   adminListSpec
	mapper func(TRecord) TDTO
}

func (q adminListQuery[TRecord, TDTO]) page(
	ctx context.Context,
	requestValue adminPageRequest,
) (request.PageResult[TDTO], error) {
	dbQuery := facades.Orm().WithContext(contextOrBackground(ctx)).Query().Table(q.spec.table)
	dbQuery = q.applyFilters(dbQuery, requestValue.Filters)
	result, err := request.Paginate[TRecord](
		dbQuery.OrderByDesc(q.spec.orderBy),
		requestValue.Page,
		requestValue.PageSize,
	)
	if err != nil {
		return request.PageResult[TDTO]{}, err
	}
	return request.PageResult[TDTO]{List: mapSlice(result.List, q.mapper), Total: result.Total}, nil
}

func (q adminListQuery[TRecord, TDTO]) applyFilters(
	dbQuery contractsorm.Query,
	filters map[string]string,
) contractsorm.Query {
	for _, item := range q.spec.filters {
		dbQuery = dbQuery.Scopes(scopes.EqualIfPresent(item.column, filters[item.filter]))
	}
	return dbQuery
}
