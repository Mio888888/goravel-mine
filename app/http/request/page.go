package request

import contractsorm "github.com/goravel/framework/contracts/database/orm"

type PageResult[T any] struct {
	List  []T   `json:"list"`
	Total int64 `json:"total"`
}

func Paginate[T any](query contractsorm.Query, page, pageSize int) (PageResult[T], error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 15
	}

	list := make([]T, 0)
	var total int64
	err := query.Paginate(page, pageSize, &list, &total)
	if list == nil {
		list = make([]T, 0)
	}

	return PageResult[T]{List: list, Total: total}, err
}
