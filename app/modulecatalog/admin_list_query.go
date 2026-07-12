package modulecatalog

import (
	"context"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/http/request"
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
	page, pageSize := normalizeAdminPage(requestValue.Page, requestValue.PageSize)
	dbQuery := facades.Orm().WithContext(contextOrBackground(ctx)).Query().Table(q.spec.table)
	dbQuery = q.applyFilters(dbQuery, requestValue.Filters)
	total, err := dbQuery.Count()
	if err != nil {
		return request.PageResult[TDTO]{}, err
	}
	records := make([]TRecord, 0)
	err = dbQuery.OrderByDesc(q.spec.orderBy).Offset((page - 1) * pageSize).Limit(pageSize).Get(&records)
	return request.PageResult[TDTO]{List: mapSlice(records, q.mapper), Total: total}, err
}

func (q adminListQuery[TRecord, TDTO]) applyFilters(
	dbQuery contractsorm.Query,
	filters map[string]string,
) contractsorm.Query {
	for _, item := range q.spec.filters {
		dbQuery = adminEqualFilter(dbQuery, item.column, filters[item.filter])
	}
	return dbQuery
}

func normalizeAdminPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 15
	}
	return page, pageSize
}

func adminEqualFilter(query contractsorm.Query, column string, value string) contractsorm.Query {
	if value == "" {
		return query
	}
	return query.Where(column, value)
}
