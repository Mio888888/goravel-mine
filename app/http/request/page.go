package request

import (
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	contractshttp "github.com/goravel/framework/contracts/http"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 15
)

type PageResult[T any] struct {
	List  []T   `json:"list"`
	Total int64 `json:"total"`
}

func Paginate[T any](query contractsorm.Query, page, pageSize int) (PageResult[T], error) {
	if page < 1 {
		page = DefaultPage
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}

	list := make([]T, 0)
	var total int64
	err := query.Paginate(page, pageSize, &list, &total)
	if list == nil {
		list = make([]T, 0)
	}

	return PageResult[T]{List: list, Total: total}, err
}

func Page(request contractshttp.ContextRequest) int {
	page := request.QueryInt("page", DefaultPage)
	if page < 1 {
		return DefaultPage
	}
	return page
}

func PageSize(request contractshttp.ContextRequest) int {
	pageSize := request.QueryInt("per_page", DefaultPageSize)
	if request.Query("per_page") == "" {
		pageSize = request.QueryInt("page_size", pageSize)
	}
	if pageSize < 1 {
		return DefaultPageSize
	}
	return pageSize
}
